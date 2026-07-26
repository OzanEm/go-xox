package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/ozanem/go-xox/internal/auth"
)

func TestHandleRegister_Success(t *testing.T) {
	svc := &fakeAuth{}

	rec := post(t, newTestAPI(svc), "/register", `{"username":"alice","password":"password123"}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body: %s)", rec.Code, rec.Body)
	}
	if svc.gotUsername != "alice" || svc.gotPassword != "password123" {
		t.Errorf("service got (%q, %q), want (alice, password123)", svc.gotUsername, svc.gotPassword)
	}

	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got["username"] != "alice" {
		t.Errorf("response username = %q, want alice", got["username"])
	}
}

func TestHandleRegister_TrimsUsername(t *testing.T) {
	svc := &fakeAuth{}

	post(t, newTestAPI(svc), "/register", `{"username":"  alice  ","password":"password123"}`)

	if svc.gotUsername != "alice" {
		t.Errorf("username = %q, want it trimmed to %q", svc.gotUsername, "alice")
	}
}

func TestHandleRegister_UsernameTaken(t *testing.T) {
	svc := &fakeAuth{err: auth.ErrUsernameTaken}

	rec := post(t, newTestAPI(svc), "/register", `{"username":"alice","password":"password123"}`)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
	var got errorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Error.Code != "username_taken" {
		t.Errorf("error code = %q, want username_taken", got.Error.Code)
	}
}

func TestHandleRegister_BadRequests(t *testing.T) {
	tooLong := ""
	for i := 0; i < 73; i++ {
		tooLong += "x"
	}

	tests := []struct {
		name string
		body string
	}{
		{"malformed json", `{"username":`},
		{"empty body", ``},
		{"missing password", `{"username":"alice"}`},
		{"missing username", `{"password":"password123"}`},
		{"blank username", `{"username":"   ","password":"password123"}`},
		{"username too short", `{"username":"ab","password":"password123"}`},
		{"password too short", `{"username":"alice","password":"short"}`},
		{"password too long", `{"username":"alice","password":"` + tooLong + `"}`},
		{"unknown field", `{"user":"alice","password":"password123"}`},
		{"json array", `["alice","password123"]`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeAuth{}
			rec := post(t, newTestAPI(svc), "/register", tt.body)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400 (body: %s)", rec.Code, rec.Body)
			}
			if svc.gotUsername != "" {
				t.Error("handler called the auth service on a malformed request")
			}
		})
	}
}

func TestHandleRegister_InternalError(t *testing.T) {
	svc := &fakeAuth{err: errors.New("postgres: connection refused")}

	rec := post(t, newTestAPI(svc), "/register", `{"username":"alice","password":"password123"}`)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "postgres") {
		t.Errorf("response leaks internal error detail: %s", rec.Body)
	}
}
