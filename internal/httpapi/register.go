package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/ozanem/go-xox/internal/auth"
)

const (
	minUsernameLen = 3
	maxUsernameLen = 50
	minPasswordLen = 8
	maxPasswordLen = 72
)

type registerRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (a *API) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "request body must be valid JSON with username and password")
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	if n := len(req.Username); n < minUsernameLen || n > maxUsernameLen {
		writeError(w, http.StatusBadRequest, "invalid_request", "username must be between 3 and 50 characters")
		return
	}
	if n := len(req.Password); n < minPasswordLen || n > maxPasswordLen {
		writeError(w, http.StatusBadRequest, "invalid_request", "password must be between 8 and 72 characters")
		return
	}

	err := a.auth.Register(r.Context(), req.Username, req.Password)
	switch {
	case errors.Is(err, auth.ErrUsernameTaken):
		writeError(w, http.StatusConflict, "username_taken", "that username is already registered")
		return
	case err != nil:
		a.logger.Error("register failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "something went wrong")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"username": req.Username})
}
