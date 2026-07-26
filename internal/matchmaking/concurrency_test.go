package matchmaking

import (
	"sync"
	"testing"
)

func TestFIFOQueueConcurrentJoin(t *testing.T) {
	const n = 1000

	q := NewFIFOQueue()
	matches := make(chan Match, n)

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(id PlayerID) {
			defer wg.Done()
			m, paired, err := q.Join(id)
			if err != nil {
				t.Errorf("Join(%d) unexpected error: %v", id, err)
				return
			}
			if paired {
				matches <- m
			}
		}(PlayerID(i + 1))
	}
	wg.Wait()
	close(matches)

	seen := make(map[PlayerID]int)
	pairings := 0
	for m := range matches {
		pairings++
		seen[m.A]++
		seen[m.B]++
	}

	if pairings != n/2 {
		t.Errorf("got %d pairings, want %d", pairings, n/2)
	}
	for id := PlayerID(1); id <= n; id++ {
		switch seen[id] {
		case 1: // correct: paired exactly once
		case 0:
			t.Errorf("player %d was never paired (lost)", id)
		default:
			t.Errorf("player %d appeared in %d matches (paired more than once)", id, seen[id])
		}
	}
	if q.Len() != 0 {
		t.Errorf("queue not empty after all pairings: Len() = %d", q.Len())
	}
}

func TestFIFOQueueConcurrentJoinLeave(t *testing.T) {
	const n = 1000

	q := NewFIFOQueue()
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		id := PlayerID(i + 1)
		wg.Add(2)
		go func() { defer wg.Done(); q.Join(id) }()
		go func() { defer wg.Done(); q.Leave(id) }()
	}
	wg.Wait()

	if l := q.Len(); l < 0 || l > n {
		t.Errorf("Len() = %d, outside the sane range [0,%d]", l, n)
	}
}
