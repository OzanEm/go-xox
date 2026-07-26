package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/ozanem/go-xox/internal/user"
)

type API struct {
	auth   AuthService
	users  LeaderBoard
	logger *slog.Logger
}

type LeaderBoard interface {
	GetLeaderBoard(ctx context.Context) ([]user.User, error)
}

func New(auth AuthService, logger *slog.Logger, users LeaderBoard) *API {
	return &API{auth: auth, logger: logger, users: users}
}

func (a *API) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", a.handleHealth)
	mux.HandleFunc("POST /register", a.handleRegister)
	mux.HandleFunc("POST /login", a.handleLogin)
	mux.HandleFunc("POST /refresh", a.handleRefresh)

	mux.Handle("POST /logout", a.auth.WithAuth(http.HandlerFunc(a.handleLogout)))
	mux.Handle("GET /leaderboard", a.auth.WithAuth(http.HandlerFunc(a.getLeaderboard)))
	return mux
}

func (a *API) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type leaderboardEntry struct {
	Username string `json:"username"`
	Wins     int64  `json:"wins"`
	Losses   int64  `json:"losses"`
	Draws    int64  `json:"draws"`
}

func (a *API) getLeaderboard(w http.ResponseWriter, r *http.Request) {
	res, err := a.users.GetLeaderBoard(r.Context())
	if err != nil {
		a.logger.Error("leaderboard query failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "something went wrong")
		return
	}

	entries := make([]leaderboardEntry, 0, len(res))
	for _, u := range res {
		entries = append(entries, leaderboardEntry{
			Username: u.Username,
			Wins:     u.Wins,
			Losses:   u.Losses,
			Draws:    u.Draws,
		})
	}
	writeJSON(w, http.StatusOK, entries)
}

type errorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	var body errorResponse
	body.Error.Code = code
	body.Error.Message = message
	writeJSON(w, status, body)
}
