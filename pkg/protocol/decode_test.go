package protocol

import "testing"

func TestDecodePayload_RoundTrip(t *testing.T) {
	original := MatchFoundPayload{
		GameID: "game-123",
		Symbol: X,
		Board:  Board{X, O, Empty, Empty, Empty, Empty, Empty, Empty, Empty},
		Turn:   O,
	}

	env, err := NewMessage(TypeMatchFound, original)
	if err != nil {
		t.Fatalf("NewMessage: %v", err)
	}

	got, err := DecodePayload[MatchFoundPayload](env)
	if err != nil {
		t.Fatalf("DecodePayload: %v", err)
	}
	if got != original {
		t.Errorf("round-trip = %+v, want %+v", got, original)
	}
}

func TestDecodePayload_Move(t *testing.T) {
	env, err := NewMessage(TypeMove, MovePayload{Cell: 7})
	if err != nil {
		t.Fatalf("NewMessage: %v", err)
	}
	got, err := DecodePayload[MovePayload](env)
	if err != nil {
		t.Fatalf("DecodePayload: %v", err)
	}
	if got.Cell != 7 {
		t.Errorf("cell = %d, want 7", got.Cell)
	}
}

func TestDecodePayload_EmptyPayload(t *testing.T) {
	env, err := NewMessage(TypeJoinQueue, nil)
	if err != nil {
		t.Fatalf("NewMessage: %v", err)
	}
	got, err := DecodePayload[MovePayload](env)
	if err != nil {
		t.Fatalf("DecodePayload on empty payload should not error, got: %v", err)
	}
	if got != (MovePayload{}) {
		t.Errorf("empty payload = %+v, want zero value", got)
	}
}
