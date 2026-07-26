package game

import "errors"

var (
	ErrGameOver     = errors.New("game is over")
	ErrOutOfBounds  = errors.New("cell out of bounds")
	ErrNotYourTurn  = errors.New("not your turn")
	ErrCellOccupied = errors.New("cell already occupied")
	ErrNotAPlayer   = errors.New("mark is not a player")
)

type Status uint8

const (
	InProgress Status = iota
	Won
	Draw
)

type Game struct {
	board  Board
	turn   Cell
	status Status
	winner Cell
}

func New() *Game {
	return &Game{turn: X}
}

func (g *Game) ApplyMove(mark Cell, cell int) error {
	if g.status != InProgress {
		return ErrGameOver
	}
	if mark != X && mark != O {
		return ErrNotAPlayer
	}
	if !g.board.InBounds(cell) {
		return ErrOutOfBounds
	}
	if mark != g.turn {
		return ErrNotYourTurn
	}
	if g.board[cell] != Empty {
		return ErrCellOccupied
	}

	g.board[cell] = mark

	switch {
	case g.board.Winner() != Empty:
		g.status = Won
		g.winner = mark
	case g.board.IsFull():
		g.status = Draw
	default:
		g.turn = g.opponent(mark)
	}
	return nil
}

func (g *Game) opponent(mark Cell) Cell {
	if mark == X {
		return O
	}
	return X
}

func (g *Game) Board() Board { return g.board }

func (g *Game) Turn() Cell { return g.turn }

func (g *Game) Status() Status { return g.status }

func (g *Game) Winner() Cell { return g.winner }

func (g *Game) IsOver() bool { return g.status != InProgress }
