package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func TestService_Register(t *testing.T) {
	svc := newTestService(t)

	if err := svc.Register(context.Background(), "bob", "hunter2password"); err != nil {
		t.Fatalf("Register: %v", err)
	}

	pair, err := svc.Login(context.Background(), "bob", "hunter2password")
	if err != nil {
		t.Fatalf("Login after Register failed: %v", err)
	}
	if pair.AccessToken == "" {
		t.Error("registered user could log in but got an empty access token")
	}
}

func TestService_RegisterStoresAHash(t *testing.T) {
	svc := newTestService(t)
	repo := svc.users.(*fakeRepo)

	const pw = "carol-secret-pw"
	if err := svc.Register(context.Background(), "carol", pw); err != nil {
		t.Fatalf("Register: %v", err)
	}

	stored := repo.users["carol"]
	if stored == nil {
		t.Fatal("Register did not persist the user")
	}
	if stored.PasswordHash == pw {
		t.Fatal("Register stored the plaintext password")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(stored.PasswordHash), []byte(pw)); err != nil {
		t.Errorf("stored hash does not verify against the password: %v", err)
	}
}

func TestService_RegisterDuplicate(t *testing.T) {
	svc := newTestService(t) // alice already exists

	err := svc.Register(context.Background(), "alice", "password123")
	if !errors.Is(err, ErrUsernameTaken) {
		t.Errorf("error = %v, want ErrUsernameTaken", err)
	}
}

func TestService_RegisterPropagatesRepositoryFailure(t *testing.T) {
	repoErr := errors.New("connection refused")
	svc := NewService(
		&fakeRepo{err: repoErr},
		NewTokenIssuer(testSecret, 30*time.Minute),
		NewRefreshTokenIssuer(24*time.Hour, newFakeRefreshRepo()),
		bcrypt.MinCost,
	)

	err := svc.Register(context.Background(), "dave", "password123")
	if errors.Is(err, ErrUsernameTaken) {
		t.Fatal("a repository failure was reported as ErrUsernameTaken")
	}
	if !errors.Is(err, repoErr) {
		t.Errorf("error = %v, want it to wrap %v", err, repoErr)
	}
}
