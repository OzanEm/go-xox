package protocol

import (
	"encoding/json"
	"fmt"
)

type MessageType string

const (
	TypeJoinQueue MessageType = "join_queue" // JoinQueuePayload
	TypeMove      MessageType = "move"       // MovePayload

	TypeMatchFound MessageType = "match_found" // MatchFoundPayload
	TypeState      MessageType = "state"       // StatePayload
	TypeGameOver   MessageType = "game_over"   // GameOverPayload
	TypeError      MessageType = "error"       // ErrorPayload
)

type Envelope struct {
	Type    MessageType     `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type Mark string

const (
	Empty Mark = ""
	X     Mark = "X"
	O     Mark = "O"
)

type Board [9]Mark

type Result string

const (
	ResultWin  Result = "win"
	ResultLoss Result = "loss"
	ResultDraw Result = "draw"
)

type MovePayload struct {
	Cell  int    `json:"cell"`
	Token string `json:"token"`
}

type JoinQueuePayload struct {
	Token string `json:"token"`
}

type MatchFoundPayload struct {
	GameID string `json:"game_id"`
	Symbol Mark   `json:"symbol"`
	Board  Board  `json:"board"`
	Turn   Mark   `json:"turn"`
}

type StatePayload struct {
	Board Board `json:"board"`
	Turn  Mark  `json:"turn"`
}

type GameOverPayload struct {
	Result Result `json:"result"`
	Winner Mark   `json:"winner,omitempty"`
	Board  Board  `json:"board"`
}

type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func NewMessage(t MessageType, payload any) (Envelope, error) {
	if payload == nil {
		return Envelope{Type: t}, nil
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, fmt.Errorf("marshal %s payload: %w", t, err)
	}
	return Envelope{Type: t, Payload: raw}, nil
}
