package game

import "testing"

func TestBoardWinner(t *testing.T) {
	tests := []struct {
		name  string
		cells [3]int
		want  Cell
	}{
		{"row top", [3]int{0, 1, 2}, X},
		{"row middle", [3]int{3, 4, 5}, X},
		{"row bottom", [3]int{6, 7, 8}, X},
		{"col left", [3]int{0, 3, 6}, X},
		{"col middle", [3]int{1, 4, 7}, X},
		{"col right", [3]int{2, 5, 8}, X},
		{"diag main", [3]int{0, 4, 8}, X},
		{"diag anti", [3]int{2, 4, 6}, X},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b Board
			for _, i := range tt.cells {
				b[i] = X
			}
			if got := b.Winner(); got != tt.want {
				t.Errorf("Winner() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBoardWinnerNegative(t *testing.T) {
	tests := []struct {
		name  string
		board Board
	}{
		{"empty board", Board{}},
		{
			// Two of X's marks on a line but the third is O — not a win.
			name:  "blocked line",
			board: Board{X, X, O, Empty, Empty, Empty, Empty, Empty, Empty},
		},
		{
			// A full board with no three-in-a-row: the draw layout.
			name:  "full no winner",
			board: Board{X, O, X, X, O, O, O, X, X},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.board.Winner(); got != Empty {
				t.Errorf("Winner() = %v, want Empty", got)
			}
		})
	}
}

func TestBoardIsFull(t *testing.T) {
	full := Board{X, O, X, X, O, O, O, X, X}
	if !full.IsFull() {
		t.Error("IsFull() = false on a fully marked board, want true")
	}
	var empty Board
	if empty.IsFull() {
		t.Error("IsFull() = true on an empty board, want false")
	}
	oneLeft := full
	oneLeft[4] = Empty
	if oneLeft.IsFull() {
		t.Error("IsFull() = true with one empty cell, want false")
	}
}

func TestBoardInBounds(t *testing.T) {
	var b Board
	for _, cell := range []int{0, 4, 8} {
		if !b.InBounds(cell) {
			t.Errorf("InBounds(%d) = false, want true", cell)
		}
	}
	for _, cell := range []int{-1, 9, 100} {
		if b.InBounds(cell) {
			t.Errorf("InBounds(%d) = true, want false", cell)
		}
	}
}

func TestCellString(t *testing.T) {
	tests := map[Cell]string{X: "X", O: "O", Empty: " "}
	for c, want := range tests {
		if got := c.String(); got != want {
			t.Errorf("Cell(%d).String() = %q, want %q", c, got, want)
		}
	}
}
