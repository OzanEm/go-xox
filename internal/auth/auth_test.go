package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/ozanem/go-xox/internal/refreshtoken"
	"github.com/ozanem/go-xox/internal/user"
)

type fakeRepo struct {
	users map[string]*user.User
	err   error
}

func (f *fakeRepo) ByUsername(_ context.Context, username string) (*user.User, error) {
	if f.err != nil {
		return nil, f.err
	}
	u, ok := f.users[username]
	if !ok {
		return nil, user.ErrNotFound
	}
	return u, nil
}

func (f *fakeRepo) ByUserID(_ context.Context, userID int64) (*user.User, error) {
	if f.err != nil {
		return nil, f.err
	}
	for _, u := range f.users {
		if u.ID == userID {
			return u, nil
		}
	}
	return nil, user.ErrNotFound
}

func (f *fakeRepo) Create(_ context.Context, username, passwordHash string) (*user.User, error) {
	if f.err != nil {
		return nil, f.err
	}
	if _, exists := f.users[username]; exists {
		return nil, user.ErrUsernameTaken
	}
	u := &user.User{
		ID:           int64(len(f.users) + 1),
		Username:     username,
		PasswordHash: passwordHash,
		CreatedAt:    time.Now(),
	}
	if f.users == nil {
		f.users = map[string]*user.User{}
	}
	f.users[username] = u
	return u, nil
}

type fakeRefreshRepo struct {
	tokens map[string]*refreshtoken.RefreshToken
	err    error
}

func newFakeRefreshRepo() *fakeRefreshRepo {
	return &fakeRefreshRepo{tokens: map[string]*refreshtoken.RefreshToken{}}
}

func (f *fakeRefreshRepo) Store(_ context.Context, userID int64, tokenHash string, expiresAt time.Time) (*refreshtoken.RefreshToken, error) {
	if f.err != nil {
		return nil, f.err
	}
	t := &refreshtoken.RefreshToken{
		ID:        int64(len(f.tokens) + 1),
		UserID:    userID,
		Hash:      tokenHash,
		Expires:   expiresAt,
		CreatedAt: time.Now(),
	}
	f.tokens[tokenHash] = t
	return t, nil
}

func (f *fakeRefreshRepo) ByHash(_ context.Context, tokenHash string) (*refreshtoken.RefreshToken, error) {
	if f.err != nil {
		return nil, f.err
	}
	t, ok := f.tokens[tokenHash]
	if !ok {
		return nil, refreshtoken.ErrNotFound
	}
	return t, nil
}

func (f *fakeRefreshRepo) Revoke(_ context.Context, tokenHash string) error {
	if f.err != nil {
		return f.err
	}
	if t, ok := f.tokens[tokenHash]; ok {
		now := time.Now()
		t.RevokedAt = &now
	}
	return nil
}

func newTestService(t *testing.T) *Service {
	t.Helper()

	hash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	repo := &fakeRepo{users: map[string]*user.User{
		"alice": {ID: 1, Username: "alice", PasswordHash: string(hash)},
	}}

	return NewService(
		repo,
		NewTokenIssuer(testSecret, 30*time.Minute),
		NewRefreshTokenIssuer(24*time.Hour, newFakeRefreshRepo()),
		bcrypt.MinCost,
	)
}

func TestService_Login(t *testing.T) {
	svc := newTestService(t)

	pair, err := svc.Login(context.Background(), "alice", "password123")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if pair.AccessToken == "" {
		t.Fatal("Login returned an empty access token")
	}
	if pair.RefreshToken == "" {
		t.Fatal("Login returned an empty refresh token")
	}
	if !pair.AccessExpiresAt.After(time.Now()) {
		t.Errorf("AccessExpiresAt = %v, want a future time", pair.AccessExpiresAt)
	}
	if !pair.RefreshExpiresAt.After(pair.AccessExpiresAt) {
		t.Errorf("RefreshExpiresAt = %v, want later than the access expiry %v",
			pair.RefreshExpiresAt, pair.AccessExpiresAt)
	}

	claims, err := svc.tokens.Verify(pair.AccessToken)
	if err != nil {
		t.Fatalf("issued token failed verification: %v", err)
	}
	if claims.Username != "alice" {
		t.Errorf("username = %q, want %q", claims.Username, "alice")
	}
}

func TestService_LoginRejectsBadCredentials(t *testing.T) {
	svc := newTestService(t)

	tests := []struct {
		name     string
		username string
		password string
	}{
		{"wrong password", "alice", "wrong-password"},
		{"unknown user", "mallory", "password123"},
		{"empty password", "alice", ""},
		{"empty username", "", "password123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.Login(context.Background(), tt.username, tt.password)
			if !errors.Is(err, ErrInvalidCredentials) {
				t.Errorf("error = %v, want ErrInvalidCredentials", err)
			}
		})
	}
}

func TestService_LoginPropagatesRepositoryFailure(t *testing.T) {
	repoErr := errors.New("connection refused")
	svc := NewService(
		&fakeRepo{err: repoErr},
		NewTokenIssuer(testSecret, 30*time.Minute),
		NewRefreshTokenIssuer(24*time.Hour, newFakeRefreshRepo()),
		bcrypt.MinCost,
	)

	_, err := svc.Login(context.Background(), "alice", "password123")
	if errors.Is(err, ErrInvalidCredentials) {
		t.Fatal("repository failure was reported as invalid credentials")
	}
	if !errors.Is(err, repoErr) {
		t.Errorf("error = %v, want it to wrap %v", err, repoErr)
	}
}

func TestService_RefreshRotates(t *testing.T) {
	svc := newTestService(t)

	first, err := svc.Login(context.Background(), "alice", "password123")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	second, err := svc.Refresh(context.Background(), first.RefreshToken)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if second.AccessToken == "" || second.RefreshToken == "" {
		t.Fatal("Refresh returned an incomplete pair")
	}
	if second.RefreshToken == first.RefreshToken {
		t.Error("Refresh did not rotate: same refresh token returned")
	}
	if _, err := svc.Refresh(context.Background(), first.RefreshToken); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("second use of rotated token: error = %v, want ErrInvalidToken", err)
	}
	if _, err := svc.Refresh(context.Background(), second.RefreshToken); err != nil {
		t.Errorf("rotated token failed to refresh: %v", err)
	}
}

func TestService_RefreshRejectsUnknownToken(t *testing.T) {
	svc := newTestService(t)

	_, err := svc.Refresh(context.Background(), "definitely-not-issued")
	if !errors.Is(err, ErrInvalidToken) {
		t.Errorf("error = %v, want ErrInvalidToken", err)
	}
}

func TestService_LogoutRevokes(t *testing.T) {
	svc := newTestService(t)

	pair, err := svc.Login(context.Background(), "alice", "password123")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	if err := svc.Logout(context.Background(), pair.RefreshToken); err != nil {
		t.Fatalf("Logout: %v", err)
	}

	if _, err := svc.Refresh(context.Background(), pair.RefreshToken); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("refresh after logout: error = %v, want ErrInvalidToken", err)
	}

}

func TestService_HashPasswordRoundTrips(t *testing.T) {
	svc := newTestService(t)

	hash, err := svc.HashPassword("hunter2")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if hash == "hunter2" {
		t.Fatal("HashPassword returned the plaintext")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte("hunter2")); err != nil {
		t.Errorf("hash does not verify against its own input: %v", err)
	}
}
