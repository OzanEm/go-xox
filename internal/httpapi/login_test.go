package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ozanem/go-xox/internal/auth"
	"github.com/ozanem/go-xox/internal/user"
)

type fakeAuth struct {
	pair auth.TokenPair
	err  error

	gotUsername     string
	gotPassword     string
	gotRefreshToken string
}

func (f *fakeAuth) Register(_ context.Context, username, password string) error {
	f.gotUsername, f.gotPassword = username, password
	return f.err
}

func (f *fakeAuth) Login(_ context.Context, username, password string) (auth.TokenPair, error) {
	f.gotUsername, f.gotPassword = username, password
	if f.err != nil {
		return auth.TokenPair{}, f.err
	}
	return f.pair, nil
}

func (f *fakeAuth) Refresh(_ context.Context, refreshToken string) (auth.TokenPair, error) {
	f.gotRefreshToken = refreshToken
	if f.err != nil {
		return auth.TokenPair{}, f.err
	}
	return f.pair, nil
}

func (f *fakeAuth) Logout(_ context.Context, refreshToken string) error {
	f.gotRefreshToken = refreshToken
	return f.err
}

func (f *fakeAuth) WithAuth(next http.Handler) http.Handler { return next }

type fakeLeaderboard struct {
	entries []user.User
	err     error
}

func (f *fakeLeaderboard) GetLeaderBoard(_ context.Context) ([]user.User, error) {
	return f.entries, f.err
}

func testPair() auth.TokenPair {
	now := time.Now().UTC().Truncate(time.Second)
	return auth.TokenPair{
		AccessToken:      "a.b.c",
		AccessExpiresAt:  now.Add(30 * time.Minute),
		RefreshToken:     "raw-refresh-token",
		RefreshExpiresAt: now.Add(24 * time.Hour),
	}
}

func newTestAPI(svc AuthService) http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(svc, logger, &fakeLeaderboard{}).Routes()
}

func post(t *testing.T, h http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestHandleLogin_Success(t *testing.T) {
	pair := testPair()
	svc := &fakeAuth{pair: pair}

	rec := post(t, newTestAPI(svc), "/login", `{"username":"alice","password":"password123"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var got tokenPairResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.AccessToken != pair.AccessToken {
		t.Errorf("access_token = %q, want %q", got.AccessToken, pair.AccessToken)
	}
	if got.RefreshToken != pair.RefreshToken {
		t.Errorf("refresh_token = %q, want %q", got.RefreshToken, pair.RefreshToken)
	}
	if !got.AccessExpiresAt.Equal(pair.AccessExpiresAt) {
		t.Errorf("access_expires_at = %v, want %v", got.AccessExpiresAt, pair.AccessExpiresAt)
	}
	if !got.RefreshExpiresAt.Equal(pair.RefreshExpiresAt) {
		t.Errorf("refresh_expires_at = %v, want %v", got.RefreshExpiresAt, pair.RefreshExpiresAt)
	}

	if svc.gotUsername != "alice" || svc.gotPassword != "password123" {
		t.Errorf("service got (%q, %q), want (alice, password123)", svc.gotUsername, svc.gotPassword)
	}
}

func TestHandleLogin_TrimsUsername(t *testing.T) {
	svc := &fakeAuth{pair: testPair()}

	post(t, newTestAPI(svc), "/login", `{"username":"  alice  ","password":"password123"}`)

	if svc.gotUsername != "alice" {
		t.Errorf("username = %q, want it trimmed to %q", svc.gotUsername, "alice")
	}
}

func TestHandleLogin_BadRequests(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"malformed json", `{"username":`},
		{"empty body", ``},
		{"missing password", `{"username":"alice"}`},
		{"missing username", `{"password":"password123"}`},
		{"blank username", `{"username":"   ","password":"password123"}`},
		{"unknown field", `{"user":"alice","password":"password123"}`},
		{"json array", `["alice","password123"]`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeAuth{pair: testPair()}
			rec := post(t, newTestAPI(svc), "/login", tt.body)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400 (body: %s)", rec.Code, rec.Body)
			}
			if svc.gotUsername != "" {
				t.Error("handler called the auth service on a malformed request")
			}
		})
	}
}

func TestHandleLogin_InvalidCredentials(t *testing.T) {
	svc := &fakeAuth{err: auth.ErrInvalidCredentials}

	rec := post(t, newTestAPI(svc), "/login", `{"username":"alice","password":"nope"}`)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}

	var got errorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Error.Code != "invalid_credentials" {
		t.Errorf("error code = %q, want invalid_credentials", got.Error.Code)
	}
	if strings.Contains(strings.ToLower(got.Error.Message), "no such user") {
		t.Errorf("error message leaks user existence: %q", got.Error.Message)
	}
}

func TestHandleLogin_InternalError(t *testing.T) {
	svc := &fakeAuth{err: errors.New("postgres: connection refused")}

	rec := post(t, newTestAPI(svc), "/login", `{"username":"alice","password":"password123"}`)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "postgres") {
		t.Errorf("response leaks internal error detail: %s", rec.Body)
	}
}

func TestRoutes_MethodAndPath(t *testing.T) {
	h := newTestAPI(&fakeAuth{pair: testPair()})

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
	}{
		{"health", http.MethodGet, "/healthz", http.StatusOK},
		{"login with GET", http.MethodGet, "/login", http.StatusMethodNotAllowed},
		{"refresh with GET", http.MethodGet, "/refresh", http.StatusMethodNotAllowed},
		{"logout with GET", http.MethodGet, "/logout", http.StatusMethodNotAllowed},
		{"unknown path", http.MethodGet, "/nope", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}
