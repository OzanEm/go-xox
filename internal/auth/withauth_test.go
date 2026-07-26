package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newAuthMiddleware(t *testing.T) (*Service, *TokenIssuer) {
	t.Helper()
	issuer := NewTokenIssuer(testSecret, 30*time.Minute)
	svc := NewService(nil, issuer, nil, 0)
	return svc, issuer
}

func TestWithAuth_ValidTokenPassesWithClaims(t *testing.T) {
	svc, issuer := newAuthMiddleware(t)
	token, _, err := issuer.Issue(42, "alice")
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	var gotUsername string
	var hadClaims bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := ClaimsFromContext(r.Context())
		hadClaims = ok
		if ok {
			gotUsername = claims.Username
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/leaderboard", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	svc.WithAuth(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !hadClaims {
		t.Fatal("handler ran without claims in context")
	}
	if gotUsername != "alice" {
		t.Errorf("claims username = %q, want alice", gotUsername)
	}
}

func TestWithAuth_Rejections(t *testing.T) {
	svc, issuer := newAuthMiddleware(t)

	foreign, _, err := NewTokenIssuer([]byte("some-other-secret"), time.Hour).Issue(1, "mallory")
	if err != nil {
		t.Fatalf("issue foreign token: %v", err)
	}
	_ = issuer

	tests := []struct {
		name   string
		header string
	}{
		{"no header", ""},
		{"not bearer", "Basic abc123"},
		{"garbage token", "Bearer not-a-jwt"},
		{"foreign signature", "Bearer " + foreign},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nextRan := false
			next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { nextRan = true })

			req := httptest.NewRequest(http.MethodGet, "/leaderboard", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			rec := httptest.NewRecorder()
			svc.WithAuth(next).ServeHTTP(rec, req)

			if nextRan {
				t.Fatal("handler ran despite auth failure")
			}
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401", rec.Code)
			}

			var got struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("401 body is not the JSON error shape: %v (body: %s)", err, rec.Body)
			}
			if got.Error.Code != "invalid_token" {
				t.Errorf("error code = %q, want invalid_token", got.Error.Code)
			}
		})
	}
}
