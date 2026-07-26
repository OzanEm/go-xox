package server

import (
	"encoding/json"
	"errors"
	"log/slog"
	"sync"

	"github.com/ozanem/go-xox/internal/game"
	"github.com/ozanem/go-xox/internal/matchmaking"
	"github.com/ozanem/go-xox/pkg/protocol"
)

type Hub struct {
	mu      sync.Mutex
	clients map[int64]*Client
	queue   *matchmaking.FIFOQueue
	games   *GameManager
	logger  *slog.Logger
}

func NewHub(games *GameManager, logger *slog.Logger) *Hub {
	return &Hub{
		clients: make(map[int64]*Client),
		queue:   matchmaking.NewFIFOQueue(),
		games:   games,
		logger:  logger,
	}
}

func (h *Hub) Register(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if existing, ok := h.clients[c.userID]; ok {
		h.removeLocked(existing)
	}
	h.clients[c.userID] = c
}

func (h *Hub) Unregister(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[c.userID] != c {
		return
	}
	h.removeLocked(c)
}

func (h *Hub) removeLocked(c *Client) {
	delete(h.clients, c.userID)
	close(c.send)
	h.queue.Leave(c.userID)
	for _, o := range h.games.Forfeit(c.userID) {
		h.sendLocked(o.userID, o.env)
	}
}

func (h *Hub) JoinQueue(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	match, paired, err := h.queue.Join(c.userID)
	if errors.Is(err, matchmaking.ErrAlreadyQueued) {
		h.sendLocked(c.userID, env(protocol.TypeError, protocol.ErrorPayload{
			Code: "already_queued", Message: "you are already waiting in the queue",
		}))
		return
	}
	if !paired {
		return // parked, waiting for an opponent
	}

	a, aok := h.clients[match.A]
	b, bok := h.clients[match.B]
	if !aok || !bok {

		h.logger.Warn("pairing referenced a missing client", "a", match.A, "b", match.B)
		if aok {
			h.queue.Join(a.userID)
		}
		if bok {
			h.queue.Join(b.userID)
		}
		return
	}

	for _, o := range h.games.Create(match.A, match.B) {
		h.sendLocked(o.userID, o.env)
	}
}

func (h *Hub) HandleMove(c *Client, cell int) {
	outs, err := h.games.Apply(c.userID, cell)
	if err != nil {
		h.send(c.userID, moveError(err))
		return
	}
	for _, o := range outs {
		h.send(o.userID, o.env)
	}
}

func (h *Hub) sendError(c *Client, code, message string) {
	h.send(c.userID, env(protocol.TypeError, protocol.ErrorPayload{Code: code, Message: message}))
}

func (h *Hub) send(userID int64, e protocol.Envelope) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sendLocked(userID, e)
}

func (h *Hub) sendLocked(userID int64, e protocol.Envelope) {
	c, ok := h.clients[userID]
	if !ok {
		return
	}
	data, err := json.Marshal(e)
	if err != nil {
		h.logger.Error("marshal outbound message", "error", err)
		return
	}
	select {
	case c.send <- data:
	default:
		h.logger.Warn("client send buffer full; dropping connection", "user", userID)
		h.removeLocked(c)
	}
}

func moveError(err error) protocol.Envelope {
	code := "invalid_move"
	switch {
	case errors.Is(err, game.ErrNotYourTurn):
		code = "not_your_turn"
	case errors.Is(err, game.ErrCellOccupied):
		code = "cell_occupied"
	case errors.Is(err, game.ErrOutOfBounds):
		code = "out_of_bounds"
	case errors.Is(err, game.ErrGameOver):
		code = "game_over"
	case errors.Is(err, errNoActiveGame):
		code = "no_active_game"
	}
	return env(protocol.TypeError, protocol.ErrorPayload{Code: code, Message: err.Error()})
}
