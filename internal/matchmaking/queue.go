package matchmaking

import (
	"errors"
	"sync"
)

type PlayerID = int64

type Match struct {
	A PlayerID
	B PlayerID
}

var ErrAlreadyQueued = errors.New("player already in queue")

type Queue interface {
	Join(p PlayerID) (m Match, paired bool, err error)

	Leave(p PlayerID) bool

	Len() int
}

type FIFOQueue struct {
	waiting []PlayerID
	mu      sync.Mutex
}

var _ Queue = (*FIFOQueue)(nil)

func NewFIFOQueue() *FIFOQueue { return &FIFOQueue{} }

func (q *FIFOQueue) Join(p PlayerID) (Match, bool, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for _, w := range q.waiting {
		if w == p {
			return Match{}, false, ErrAlreadyQueued
		}
	}

	q.waiting = append(q.waiting, p)
	if len(q.waiting) < 2 {
		return Match{}, false, nil
	}

	a, b := q.waiting[0], q.waiting[1]
	q.waiting = q.waiting[2:]
	return Match{A: a, B: b}, true, nil
}

func (q *FIFOQueue) Leave(p PlayerID) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i, w := range q.waiting {
		if w == p {
			q.waiting = append(q.waiting[:i], q.waiting[i+1:]...)
			return true
		}
	}
	return false
}

func (q *FIFOQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.waiting)
}
