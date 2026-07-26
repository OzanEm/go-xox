package httpapi

import (
	"errors"
	"net/http"

	"github.com/ozanem/go-xox/internal/auth"
)

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func (a *API) handleRefresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "request body must be valid JSON with refresh_token")
		return
	}
	if req.RefreshToken == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "refresh_token is required")
		return
	}

	pair, err := a.auth.Refresh(r.Context(), req.RefreshToken)
	switch {
	case errors.Is(err, auth.ErrInvalidToken):

		writeError(w, http.StatusUnauthorized, "invalid_token", "refresh token is invalid or expired")
		return
	case err != nil:
		a.logger.Error("refresh failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "something went wrong")
		return
	}

	writeJSON(w, http.StatusOK, toTokenPairResponse(pair))
}

func (a *API) handleLogout(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "request body must be valid JSON with refresh_token")
		return
	}
	if req.RefreshToken == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "refresh_token is required")
		return
	}

	if err := a.auth.Logout(r.Context(), req.RefreshToken); err != nil {
		a.logger.Error("logout failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "something went wrong")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
