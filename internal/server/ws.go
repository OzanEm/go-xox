package server

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"

	"github.com/ozanem/go-xox/internal/auth"
)

type WSHandler struct {
	hub      *Hub
	tokens   *auth.TokenIssuer
	upgrader websocket.Upgrader
	logger   *slog.Logger
}

func NewWSHandler(hub *Hub, tokens *auth.TokenIssuer, logger *slog.Logger) *WSHandler {
	return &WSHandler{
		hub:    hub,
		tokens: tokens,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin:     func(*http.Request) bool { return true },
		},
		logger: logger,
	}
}

func (h *WSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	token := bearerToken(r)
	if token == "" {
		http.Error(w, "missing bearer token", http.StatusUnauthorized)
		return
	}
	claims, err := h.tokens.Verify(token)
	if err != nil {
		http.Error(w, "invalid or expired token", http.StatusUnauthorized)
		return
	}
	userID, err := claims.UserID()
	if err != nil {
		http.Error(w, "invalid token subject", http.StatusUnauthorized)
		return
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Info("ws upgrade failed", "user", userID, "error", err)
		return
	}

	client := newClient(h.hub, conn, userID, claims.Username, h.tokens, h.logger)
	h.hub.Register(client)
	go client.writePump()
	go client.readPump()
}

func bearerToken(r *http.Request) string {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if len(h) > len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
		return h[len(prefix):]
	}
	return ""
}
