package game

import (
	"errors"
	"testing"
)

func play(t *testing.T, g *Game, moves []struct {
	mark Cell
	cell int
}) {
	t.Helper()
	for i, m := range moves {
		if err := g.ApplyMove(m.mark, m.cell); err != nil {
			t.Fatalf("move %d (%v -> %d) failed: %v", i, m.mark, m.cell, err)
		}
	}
}

func TestGameXWins(t *testing.T) {
	g := New()
	// X takes the top row; O answers in the middle row and loses.
	//  X X X
	//  O O .
	//  . . .
	play(t, g, []struct {
		mark Cell
		cell int
	}{
		{X, 0}, {O, 3},
		{X, 1}, {O, 4},
		{X, 2}, // completes the top row
	})

	if g.Status() != Won {
		t.Fatalf("Status() = %v, want Won", g.Status())
	}
	if g.Winner() != X {
		t.Errorf("Winner() = %v, want X", g.Winner())
	}
	if !g.IsOver() {
		t.Error("IsOver() = false, want true")
	}
}

func TestGameDraw(t *testing.T) {
	g := New()
	// A filled board with no line:
	//  X O X
	//  X O O
	//  O X X
	play(t, g, []struct {
		mark Cell
		cell int
	}{
		{X, 0}, {O, 1},
		{X, 2}, {O, 4},
		{X, 3}, {O, 5},
		{X, 7}, {O, 6},
		{X, 8}, // last cell, no winner
	})

	if g.Status() != Draw {
		t.Fatalf("Status() = %v, want Draw", g.Status())
	}
	if g.Winner() != Empty {
		t.Errorf("Winner() = %v, want Empty on a draw", g.Winner())
	}
	if !g.IsOver() {
		t.Error("IsOver() = false, want true")
	}
}

func TestGameStrictAlternation(t *testing.T) {
	g := New()
	if err := g.ApplyMove(X, 0); err != nil {
		t.Fatalf("first move failed: %v", err)
	}
	if err := g.ApplyMove(X, 1); !errors.Is(err, ErrNotYourTurn) {
		t.Errorf("second X move: got %v, want ErrNotYourTurn", err)
	}
	if g.Turn() != O {
		t.Errorf("Turn() = %v after rejected move, want O", g.Turn())
	}
	if g.Board()[1] != Empty {
		t.Error("rejected move marked the board")
	}
}

func TestGameMoveRejections(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(g *Game)
		mark    Cell
		cell    int
		wantErr error
	}{
		{
			name:    "out of bounds high",
			mark:    X,
			cell:    9,
			wantErr: ErrOutOfBounds,
		},
		{
			name:    "out of bounds negative",
			mark:    X,
			cell:    -1,
			wantErr: ErrOutOfBounds,
		},
		{
			name:    "not a player",
			mark:    Empty,
			cell:    0,
			wantErr: ErrNotAPlayer,
		},
		{
			name:    "out of turn",
			mark:    O,
			cell:    0,
			wantErr: ErrNotYourTurn,
		},
		{
			name:    "occupied cell",
			setup:   func(g *Game) { _ = g.ApplyMove(X, 4) },
			mark:    O,
			cell:    4,
			wantErr: ErrCellOccupied,
		},
		{
			name: "move after game over",
			setup: func(g *Game) {
				g.ApplyMove(X, 0)
				g.ApplyMove(O, 3)
				g.ApplyMove(X, 1)
				g.ApplyMove(O, 4)
				g.ApplyMove(X, 2) // X wins
			},
			mark:    O,
			cell:    8,
			wantErr: ErrGameOver,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := New()
			if tt.setup != nil {
				tt.setup(g)
			}
			if err := g.ApplyMove(tt.mark, tt.cell); !errors.Is(err, tt.wantErr) {
				t.Errorf("ApplyMove(%v, %d) = %v, want %v", tt.mark, tt.cell, err, tt.wantErr)
			}
		})
	}
}

func TestCheckOrderGameOverBeatsBounds(t *testing.T) {
	g := New()
	g.ApplyMove(X, 0)
	g.ApplyMove(O, 3)
	g.ApplyMove(X, 1)
	g.ApplyMove(O, 4)
	g.ApplyMove(X, 2) // X wins

	if err := g.ApplyMove(O, 99); !errors.Is(err, ErrGameOver) {
		t.Errorf("post-game out-of-bounds move = %v, want ErrGameOver", err)
	}
}

func TestRejectedMoveLeavesStateUntouched(t *testing.T) {
	g := New()
	before := g.Board()
	if err := g.ApplyMove(O, 0); err == nil {
		t.Fatal("expected rejection")
	}
	if g.Board() != before {
		t.Error("board changed after a rejected move")
	}
	if g.Turn() != X {
		t.Errorf("Turn() = %v after rejected move, want X", g.Turn())
	}
}
