package server

import (
	"errors"
	"io"
	"log/slog"
	"testing"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestInGameTracksSessionLifetime(t *testing.T) {
	gm := NewGameManager(noopRecorder{}, testLogger())

	if gm.InGame(1) {
		t.Error("player is in a game before one was created")
	}

	if _, err := gm.Create(1, 2); err != nil {
		t.Fatalf("create game: %v", err)
	}
	if !gm.InGame(1) || !gm.InGame(2) {
		t.Error("both players should be in a game after Create")
	}

	gm.Forfeit(2)
	if gm.InGame(1) || gm.InGame(2) {
		t.Error("neither player should be in a game after it ends")
	}
}

func TestCreateRejectsPlayerAlreadyInGame(t *testing.T) {
	gm := NewGameManager(noopRecorder{}, testLogger())

	if _, err := gm.Create(1, 2); err != nil {
		t.Fatalf("first game: %v", err)
	}

	for _, tc := range []struct {
		name string
		a, b int64
	}{
		{"first player busy", 1, 3},
		{"second player busy", 3, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := gm.Create(tc.a, tc.b); !errors.Is(err, errAlreadyInGame) {
				t.Fatalf("err = %v, want errAlreadyInGame", err)
			}
			if gm.InGame(3) {
				t.Error("the free player was registered even though Create failed")
			}
		})
	}

	// The original game is untouched: player 1 is still X and still to move.
	if _, err := gm.Apply(1, 0); err != nil {
		t.Errorf("player 1 cannot move in their original game: %v", err)
	}
}
