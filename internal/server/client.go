package server

import (
	"encoding/json"
	"log/slog"
	"time"

	"github.com/gorilla/websocket"

	"github.com/ozanem/go-xox/internal/auth"
	"github.com/ozanem/go-xox/pkg/protocol"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 4 << 10
	sendBuffer     = 32
)

type Client struct {
	hub      *Hub
	conn     *websocket.Conn
	userID   int64
	username string
	send     chan []byte
	logger   *slog.Logger
	tokens   *auth.TokenIssuer
}

func newClient(hub *Hub, conn *websocket.Conn, userID int64, username string, tokens *auth.TokenIssuer, logger *slog.Logger) *Client {
	return &Client{
		hub:      hub,
		conn:     conn,
		userID:   userID,
		username: username,
		send:     make(chan []byte, sendBuffer),
		tokens:   tokens,
		logger:   logger,
	}
}

func (c *Client) verifyOperationToken(token string) bool {
	claims, err := c.tokens.Verify(token)
	if err != nil {
		return false
	}
	userID, err := claims.UserID()
	if err != nil || userID != c.userID {
		return false
	}
	return true
}

func (c *Client) readPump() {
	defer c.hub.Unregister(c)

	c.conn.SetReadLimit(maxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				c.logger.Info("unexpected ws close", "user", c.userID, "error", err)
			}
			return
		}

		var envelope protocol.Envelope
		if err := json.Unmarshal(data, &envelope); err != nil {
			c.hub.sendError(c, "bad_message", "message was not valid JSON")
			continue
		}

		switch envelope.Type {
		case protocol.TypeJoinQueue:
			payload, err := protocol.DecodePayload[protocol.JoinQueuePayload](envelope)
			if err != nil {
				c.hub.sendError(c, "bad_message", "join_queue payload was invalid")
				continue
			}

			if !c.verifyOperationToken(payload.Token) {
				c.hub.sendError(c, "invalid_token", "token is invalid or expired")
				return
			}
			c.hub.JoinQueue(c)
		case protocol.TypeMove:

			if len(envelope.Payload) == 0 {
				c.hub.sendError(c, "bad_message", "move requires a cell payload")
				continue
			}
			move, err := protocol.DecodePayload[protocol.MovePayload](envelope)
			if err != nil {
				c.hub.sendError(c, "bad_message", "move payload was invalid")
				continue
			}
			if !c.verifyOperationToken(move.Token) {
				c.hub.sendError(c, "invalid_token", "token is invalid or expired")
				return
			}
			c.hub.HandleMove(c, move.Cell)
		default:
			c.hub.sendError(c, "unknown_type", "unknown message type: "+string(envelope.Type))
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()

	for {
		select {
		case data, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
