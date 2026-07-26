package matchmaking

import (
	"errors"
	"testing"
)

func conformance(t *testing.T, newQueue func() Queue) {
	t.Helper()

	t.Run("empty queue has length zero", func(t *testing.T) {
		if got := newQueue().Len(); got != 0 {
			t.Errorf("Len() = %d, want 0", got)
		}
	})

	t.Run("first join parks the player", func(t *testing.T) {
		q := newQueue()
		m, paired, err := q.Join(1)
		if err != nil {
			t.Fatalf("Join returned error: %v", err)
		}
		if paired {
			t.Errorf("paired = true on first join, want false (got match %+v)", m)
		}
		if q.Len() != 1 {
			t.Errorf("Len() = %d after one join, want 1", q.Len())
		}
	})

	t.Run("second distinct join pairs both, earlier joiner is A", func(t *testing.T) {
		q := newQueue()
		q.Join(1)
		m, paired, err := q.Join(2)
		if err != nil {
			t.Fatalf("Join returned error: %v", err)
		}
		if !paired {
			t.Fatal("paired = false on second join, want true")
		}
		if m.A != 1 || m.B != 2 {
			t.Errorf("Match = %+v, want {A:1 B:2} (earlier joiner is A)", m)
		}
		if q.Len() != 0 {
			t.Errorf("Len() = %d after a pairing, want 0", q.Len())
		}
	})

	t.Run("duplicate join is rejected and leaves state untouched", func(t *testing.T) {
		q := newQueue()
		q.Join(1)
		m, paired, err := q.Join(1) // same player again
		if !errors.Is(err, ErrAlreadyQueued) {
			t.Errorf("err = %v, want ErrAlreadyQueued", err)
		}
		if paired {
			t.Errorf("paired = true on duplicate join, want false — this is a self-match bug (got %+v)", m)
		}
		if q.Len() != 1 {
			t.Errorf("Len() = %d after rejected duplicate, want 1", q.Len())
		}
	})

	t.Run("leave removes a waiting player", func(t *testing.T) {
		q := newQueue()
		q.Join(1)
		if !q.Leave(1) {
			t.Error("Leave(1) = false, want true for a queued player")
		}
		if q.Len() != 0 {
			t.Errorf("Len() = %d after leave, want 0", q.Len())
		}
	})

	t.Run("leave of an absent player reports false", func(t *testing.T) {
		q := newQueue()
		if q.Leave(99) {
			t.Error("Leave(99) = true on empty queue, want false")
		}
	})

	t.Run("a departed player is not paired later", func(t *testing.T) {
		q := newQueue()
		q.Join(1)
		q.Leave(1)
		_, paired, _ := q.Join(2)
		if paired {
			t.Error("paired = true, but player 1 had left — stale pairing")
		}
		if q.Len() != 1 {
			t.Errorf("Len() = %d, want 1 (only player 2 waiting)", q.Len())
		}
	})

	t.Run("queue is reusable after a pairing", func(t *testing.T) {
		q := newQueue()
		q.Join(1)
		q.Join(2) // pair, queue now empty

		if _, paired, _ := q.Join(3); paired {
			t.Error("player 3 paired immediately, want parked")
		}
		m, paired, _ := q.Join(4)
		if !paired {
			t.Fatal("player 4 did not pair with waiting player 3")
		}
		if m.A != 3 || m.B != 4 {
			t.Errorf("Match = %+v, want {A:3 B:4}", m)
		}
	})
}

func TestFIFOQueue(t *testing.T) {
	conformance(t, func() Queue { return NewFIFOQueue() })
}
