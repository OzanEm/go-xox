package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/ozanem/go-xox/internal/refreshtoken"
)

type RefreshTokenIssuer struct {
	repo refreshtoken.Repository
	ttl  time.Duration
	now  func() time.Time
}

func NewRefreshTokenIssuer(ttl time.Duration, repo refreshtoken.Repository) *RefreshTokenIssuer {
	return &RefreshTokenIssuer{repo: repo, ttl: ttl, now: time.Now}
}

func (rt *RefreshTokenIssuer) Issue(ctx context.Context, userID int64) (string, time.Time, error) {
	token, err := createRefreshToken()
	if err != nil {
		return "", time.Time{}, fmt.Errorf("generate refresh token: %w", err)
	}

	expiresAt := rt.now().Add(rt.ttl)

	if _, err := rt.repo.Store(ctx, userID, hashToken(token), expiresAt); err != nil {
		return "", time.Time{}, fmt.Errorf("store refresh token: %w", err)
	}

	return token, expiresAt, nil
}

func (rt *RefreshTokenIssuer) Validate(ctx context.Context, token string) (*refreshtoken.RefreshToken, error) {
	stored, err := rt.repo.ByHash(ctx, hashToken(token))
	if err != nil {
		return nil, err
	}

	if stored.RevokedAt != nil {
		return nil, refreshtoken.ErrInvalidRefreshToken
	}
	if rt.now().After(stored.Expires) {
		return nil, refreshtoken.ErrInvalidRefreshToken
	}
	return stored, nil
}

func (rt *RefreshTokenIssuer) Revoke(ctx context.Context, token string) error {
	return rt.repo.Revoke(ctx, hashToken(token))
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func createRefreshToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
