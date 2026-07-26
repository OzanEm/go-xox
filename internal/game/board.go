package game

type Cell uint8

const (
	Empty Cell = iota
	X
	O
)

func (c Cell) String() string {
	switch c {
	case X:
		return "X"
	case O:
		return "O"
	default:
		return " "
	}
}

type Board [9]Cell

var winLines = [8][3]int{
	{0, 1, 2}, {3, 4, 5}, {6, 7, 8}, // rows
	{0, 3, 6}, {1, 4, 7}, {2, 5, 8}, // columns
	{0, 4, 8}, {2, 4, 6}, // diagonals
}

func (b Board) InBounds(cell int) bool {
	return cell >= 0 && cell < len(b)
}

func (b Board) Winner() Cell {
	for _, line := range winLines {
		first := b[line[0]]
		if first != Empty && first == b[line[1]] && first == b[line[2]] {
			return first
		}
	}
	return Empty
}

func (b Board) IsFull() bool {
	for _, c := range b {
		if c == Empty {
			return false
		}
	}
	return true
}
