package server

import (
	"io"
	"log/slog"
	"sync"
	"testing"
)

func TestGameEndsExactlyOnce_MoveVsForfeit(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	for i := 0; i < 1000; i++ {
		rec := &captureRecorder{}
		gm := NewGameManager(rec, logger)
		gm.Create(1, 2) // 1 = X, 2 = O

		// Bring X to one move from winning the top row.
		for _, m := range []struct {
			player int64
			cell   int
		}{{1, 0}, {2, 3}, {1, 1}, {2, 4}} {
			if _, err := gm.Apply(m.player, m.cell); err != nil {
				t.Fatalf("setup move %+v: %v", m, err)
			}
		}

		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			_, _ = gm.Apply(1, 2) // X's winning move
		}()
		go func() {
			defer wg.Done()
			<-start
			gm.Forfeit(2) // O disconnects at the same instant
		}()
		close(start)
		wg.Wait()

		rec.mu.Lock()
		decisive := append([][2]int64(nil), rec.decisive...)
		draws := len(rec.draws)
		rec.mu.Unlock()

		if draws != 0 {
			t.Fatalf("iteration %d: RecordDraw called %d times on a decisive game", i, draws)
		}
		if len(decisive) != 1 {
			t.Fatalf("iteration %d: RecordDecisive called %d times, want exactly 1 (got %v)", i, len(decisive), decisive)
		}
		if decisive[0] != [2]int64{1, 2} {
			t.Fatalf("iteration %d: recorded {winner,loser} = %v, want {1,2}", i, decisive[0])
		}
	}
}
