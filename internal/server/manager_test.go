package server

import (
	"io"
	"log/slog"
	"testing"
)

func TestInGameTracksSessionLifetime(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	gm := NewGameManager(noopRecorder{}, logger)

	if gm.InGame(1) {
		t.Error("player is in a game before one was created")
	}

	gm.Create(1, 2)
	if !gm.InGame(1) || !gm.InGame(2) {
		t.Error("both players should be in a game after Create")
	}

	gm.Forfeit(2)
	if gm.InGame(1) || gm.InGame(2) {
		t.Error("neither player should be in a game after it ends")
	}
}

// Second line of defence. The hub refuses to queue a player who is already
// playing, so a player should never hold two sessions at once — but if that
// guard is ever bypassed, tearing one game down must not evict a player from
// the *other*, still-live game.
func TestRemoveOnlyClearsMappingsPointingAtItself(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	gm := NewGameManager(noopRecorder{}, logger)

	gm.Create(1, 2) // g1: player 1 vs 2
	gm.Create(1, 3) // g2: player 1 vs 3 — player 1's mapping now points at g2

	gm.Forfeit(2) // player 2 quits g1

	if !gm.InGame(1) {
		t.Error("player 1 was evicted from g2 by g1's teardown")
	}
	if !gm.InGame(3) {
		t.Error("player 3 was evicted from g2 by g1's teardown")
	}
	if _, err := gm.Apply(1, 0); err != nil {
		t.Errorf("player 1 cannot move in g2 after g1 was torn down: %v", err)
	}
}
