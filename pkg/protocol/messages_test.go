package protocol

import (
	"encoding/json"
	"testing"
)

func TestNewMessage_MarshalsPayload(t *testing.T) {
	env, err := NewMessage(TypeMove, MovePayload{Cell: 4})
	if err != nil {
		t.Fatalf("NewMessage: %v", err)
	}
	if env.Type != TypeMove {
		t.Errorf("Type = %q, want %q", env.Type, TypeMove)
	}

	var got MovePayload
	if err := json.Unmarshal(env.Payload, &got); err != nil {
		t.Fatalf("payload is not valid MovePayload JSON: %v", err)
	}
	if got.Cell != 4 {
		t.Errorf("payload cell = %d, want 4", got.Cell)
	}
}

func TestNewMessage_NilPayload(t *testing.T) {
	env, err := NewMessage(TypeJoinQueue, nil)
	if err != nil {
		t.Fatalf("NewMessage: %v", err)
	}

	raw, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	got := string(raw)
	want := `{"type":"join_queue"}`
	if got != want {
		t.Errorf("frame = %s, want %s", got, want)
	}
}

func TestBoard_MarshalsAsStrings(t *testing.T) {
	b := Board{X, O, Empty, Empty, X, Empty, Empty, Empty, O}
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("marshal board: %v", err)
	}
	want := `["X","O","","","X","","","","O"]`
	if string(raw) != want {
		t.Errorf("board = %s, want %s", raw, want)
	}
}
