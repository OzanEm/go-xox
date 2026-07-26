package auth

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var testSecret = []byte("test-secret")

func TestTokenIssuer_IssueThenVerify(t *testing.T) {
	issuer := NewTokenIssuer(testSecret, 30*time.Minute)

	token, expiresAt, err := issuer.Issue(42, "alice")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	claims, err := issuer.Verify(token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	userID, err := claims.UserID()
	if err != nil {
		t.Fatalf("UserID: %v", err)
	}
	if userID != 42 {
		t.Errorf("user id = %d, want 42", userID)
	}
	if claims.Username != "alice" {
		t.Errorf("username = %q, want %q", claims.Username, "alice")
	}
	if got := claims.ExpiresAt.Time; !got.Equal(expiresAt.Truncate(time.Second)) {
		t.Errorf("exp claim = %v, want %v", got, expiresAt.Truncate(time.Second))
	}
}

func TestTokenIssuer_VerifyRejects(t *testing.T) {
	issuer := NewTokenIssuer(testSecret, 30*time.Minute)
	valid, _, err := issuer.Issue(1, "alice")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	foreign, _, err := NewTokenIssuer([]byte("other-secret"), 30*time.Minute).Issue(1, "alice")
	if err != nil {
		t.Fatalf("Issue foreign: %v", err)
	}

	unsigned, err := jwt.NewWithClaims(jwt.SigningMethodNone, Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "1",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		Username: "alice",
	}).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("Issue unsigned: %v", err)
	}

	parts := strings.Split(valid, ".")
	tampered := parts[0] + "." + parts[1] + "X." + parts[2]

	tests := []struct {
		name  string
		token string
	}{
		{"empty", ""},
		{"not a jwt", "definitely-not-a-token"},
		{"signed with a different secret", foreign},
		{"alg none", unsigned},
		{"tampered payload", tampered},
		{"missing signature", parts[0] + "." + parts[1]},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := issuer.Verify(tt.token); !errors.Is(err, ErrInvalidToken) {
				t.Errorf("Verify(%q) error = %v, want ErrInvalidToken", tt.name, err)
			}
		})
	}
}

func TestTokenIssuer_VerifyRejectsExpired(t *testing.T) {
	issuer := NewTokenIssuer(testSecret, 30*time.Minute)

	base := time.Now()
	issuer.now = func() time.Time { return base.Add(-time.Hour) }

	token, _, err := issuer.Issue(1, "alice")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	issuer.now = func() time.Time { return base }

	if _, err := issuer.Verify(token); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("Verify(expired) error = %v, want ErrInvalidToken", err)
	}
}
