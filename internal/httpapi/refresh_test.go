package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/ozanem/go-xox/internal/auth"
)

func TestHandleRefresh_Success(t *testing.T) {
	pair := testPair()
	svc := &fakeAuth{pair: pair}

	rec := post(t, newTestAPI(svc), "/refresh", `{"refresh_token":"old-token"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body)
	}
	if svc.gotRefreshToken != "old-token" {
		t.Errorf("service got token %q, want %q", svc.gotRefreshToken, "old-token")
	}

	var got tokenPairResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.AccessToken != pair.AccessToken || got.RefreshToken != pair.RefreshToken {
		t.Errorf("got pair (%q, %q), want (%q, %q)",
			got.AccessToken, got.RefreshToken, pair.AccessToken, pair.RefreshToken)
	}
}

func TestHandleRefresh_BadRequests(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"malformed json", `{"refresh_token":`},
		{"empty body", ``},
		{"missing token", `{}`},
		{"blank token", `{"refresh_token":""}`},
		{"unknown field", `{"token":"abc"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeAuth{pair: testPair()}
			rec := post(t, newTestAPI(svc), "/refresh", tt.body)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400 (body: %s)", rec.Code, rec.Body)
			}
			if svc.gotRefreshToken != "" {
				t.Error("handler called the auth service on a malformed request")
			}
		})
	}
}

func TestHandleRefresh_InvalidToken(t *testing.T) {
	svc := &fakeAuth{err: auth.ErrInvalidToken}

	rec := post(t, newTestAPI(svc), "/refresh", `{"refresh_token":"revoked-or-unknown"}`)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}

	var got errorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Error.Code != "invalid_token" {
		t.Errorf("error code = %q, want invalid_token", got.Error.Code)
	}
	for _, leak := range []string{"revoked", "expired only", "not found"} {
		if strings.Contains(strings.ToLower(got.Error.Message), leak) {
			t.Errorf("error message leaks rejection reason: %q", got.Error.Message)
		}
	}
}

func TestHandleRefresh_InternalError(t *testing.T) {
	svc := &fakeAuth{err: errors.New("postgres: connection refused")}

	rec := post(t, newTestAPI(svc), "/refresh", `{"refresh_token":"abc"}`)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "postgres") {
		t.Errorf("response leaks internal error detail: %s", rec.Body)
	}
}

func TestHandleLogout_Success(t *testing.T) {
	svc := &fakeAuth{}

	rec := post(t, newTestAPI(svc), "/logout", `{"refresh_token":"abc"}`)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 (body: %s)", rec.Code, rec.Body)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("204 response has a body: %s", rec.Body)
	}
	if svc.gotRefreshToken != "abc" {
		t.Errorf("service got token %q, want %q", svc.gotRefreshToken, "abc")
	}
}

func TestHandleLogout_BadRequest(t *testing.T) {
	svc := &fakeAuth{}

	rec := post(t, newTestAPI(svc), "/logout", `{}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandleLogout_InternalError(t *testing.T) {
	svc := &fakeAuth{err: errors.New("postgres: connection refused")}

	rec := post(t, newTestAPI(svc), "/logout", `{"refresh_token":"abc"}`)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "postgres") {
		t.Errorf("response leaks internal error detail: %s", rec.Body)
	}
}
