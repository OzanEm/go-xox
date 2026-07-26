package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ozanem/go-xox/internal/user"
)

func getLeaderboard(t *testing.T, lb LeaderBoard) *httptest.ResponseRecorder {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := New(&fakeAuth{}, logger, lb).Routes()
	req := httptest.NewRequest(http.MethodGet, "/leaderboard", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestLeaderboard_ReturnsEntries(t *testing.T) {
	rec := getLeaderboard(t, &fakeLeaderboard{entries: []user.User{
		{ID: 1, Username: "alice", PasswordHash: "$2a$12$secret", Wins: 3, Losses: 1, Draws: 2},
		{ID: 2, Username: "bob", Wins: 1, Losses: 3, Draws: 2},
	}})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body)
	}

	var got []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0]["username"] != "alice" || got[0]["wins"] != float64(3) {
		t.Errorf("first entry = %v, want alice with 3 wins", got[0])
	}

	if strings.Contains(rec.Body.String(), "secret") {
		t.Fatalf("response leaks the password hash: %s", rec.Body)
	}
	for _, forbidden := range []string{"PasswordHash", "CreatedAt", "ID"} {
		if strings.Contains(rec.Body.String(), forbidden) {
			t.Errorf("response contains entity field %q: %s", forbidden, rec.Body)
		}
	}
}

func TestLeaderboard_EmptyIsArray(t *testing.T) {
	rec := getLeaderboard(t, &fakeLeaderboard{})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if body := strings.TrimSpace(rec.Body.String()); body != "[]" {
		t.Errorf("body = %s, want []", body)
	}
}

func TestLeaderboard_InternalError(t *testing.T) {
	rec := getLeaderboard(t, &fakeLeaderboard{err: errors.New("postgres: connection refused")})

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "postgres") {
		t.Errorf("response leaks internal error detail: %s", rec.Body)
	}
	var got errorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Error.Code != "internal_error" {
		t.Errorf("error code = %q, want internal_error", got.Error.Code)
	}
}
