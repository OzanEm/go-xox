package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/ozanem/go-xox/internal/refreshtoken"
	"github.com/ozanem/go-xox/internal/user"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInvalidToken       = errors.New("invalid token")
	ErrUsernameTaken      = errors.New("username already taken")
)

// If a user does not exist, we still do the dummy hash so the process stays hidden
const dummyHash = "$2a$12$H4eoKG6QwDLBHUvFIVxO1eQlNCgpfXqi/o8B6t07XqXRnMb.Vjoye"

type Service struct {
	users      user.Repository
	tokens     *TokenIssuer
	refresh    *RefreshTokenIssuer
	bcryptCost int
}

type TokenPair struct {
	AccessToken      string
	AccessExpiresAt  time.Time
	RefreshToken     string
	RefreshExpiresAt time.Time
}

func NewService(users user.Repository, tokens *TokenIssuer, refresh *RefreshTokenIssuer, bcryptCost int) *Service {
	return &Service{users: users, tokens: tokens, refresh: refresh, bcryptCost: bcryptCost}
}

func (s *Service) Login(ctx context.Context, username, password string) (TokenPair, error) {
	u, err := s.verifyCredentials(ctx, username, password)
	if err != nil {
		return TokenPair{}, err
	}
	return s.issuePair(ctx, u)
}

func (s *Service) Refresh(ctx context.Context, rawToken string) (TokenPair, error) {
	stored, err := s.refresh.Validate(ctx, rawToken)
	switch {
	case errors.Is(err, refreshtoken.ErrNotFound),
		errors.Is(err, refreshtoken.ErrInvalidRefreshToken):
		return TokenPair{}, ErrInvalidToken
	case err != nil:
		return TokenPair{}, fmt.Errorf("validate refresh token: %w", err)
	}

	u, err := s.users.ByUserID(ctx, stored.UserID)
	switch {
	case errors.Is(err, user.ErrNotFound):
		// The account behind the token is gone; the token dies with it.
		return TokenPair{}, ErrInvalidToken
	case err != nil:
		return TokenPair{}, fmt.Errorf("look up user: %w", err)
	}

	pair, err := s.issuePair(ctx, u)
	if err != nil {
		return TokenPair{}, err
	}
	if err := s.refresh.Revoke(ctx, rawToken); err != nil {
		return TokenPair{}, fmt.Errorf("revoke rotated refresh token: %w", err)
	}
	return pair, nil
}

func (s *Service) Logout(ctx context.Context, rawToken string) error {
	if err := s.refresh.Revoke(ctx, rawToken); err != nil {
		return fmt.Errorf("revoke refresh token: %w", err)
	}
	return nil
}

func (s *Service) issuePair(ctx context.Context, u *user.User) (TokenPair, error) {
	access, accessExp, err := s.tokens.Issue(u.ID, u.Username)
	if err != nil {
		return TokenPair{}, fmt.Errorf("issue access token: %w", err)
	}

	refresh, refreshExp, err := s.refresh.Issue(ctx, u.ID)
	if err != nil {
		return TokenPair{}, fmt.Errorf("issue refresh token: %w", err)
	}

	return TokenPair{
		AccessToken:      access,
		AccessExpiresAt:  accessExp,
		RefreshToken:     refresh,
		RefreshExpiresAt: refreshExp,
	}, nil
}

func (s *Service) verifyCredentials(ctx context.Context, username, password string) (*user.User, error) {
	u, err := s.users.ByUsername(ctx, username)
	switch {
	case errors.Is(err, user.ErrNotFound):
		// Burn the same work an existing user would have cost us, then fail.
		_ = bcrypt.CompareHashAndPassword([]byte(dummyHash), []byte(password))
		return nil, ErrInvalidCredentials
	case err != nil:
		return nil, fmt.Errorf("look up user: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}
	return u, nil
}

func (s *Service) HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), s.bcryptCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}

type claimsKey struct{}

func ClaimsFromContext(ctx context.Context) (*Claims, bool) {
	c, ok := ctx.Value(claimsKey{}).(*Claims)
	return c, ok
}

func (s *Service) WithAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok {
			writeAuthError(w)
			return
		}
		claims, err := s.tokens.Verify(token)
		if err != nil {
			writeAuthError(w)
			return
		}
		r = r.WithContext(context.WithValue(r.Context(), claimsKey{}, claims))
		next.ServeHTTP(w, r)
	})
}

func writeAuthError(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":{"code":"invalid_token","message":"missing or invalid bearer token"}}`))
}
