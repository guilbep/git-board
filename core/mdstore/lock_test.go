package mdstore

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Writers waiting on a lock its dead writer left take it over one at a
// time: one never deletes the fresh lock another just took.
func TestStaleLockTakenOverOnce(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "x.jsonl.lock")
	old := time.Now().Add(-2 * staleLock)
	for round := range 100 {
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
		var in, overlaps atomic.Int32
		var wg sync.WaitGroup
		for range 16 {
			wg.Go(func() {
				unlock, err := lock(path)
				if err != nil {
					t.Error(err)
					return
				}
				if in.Add(1) > 1 {
					overlaps.Add(1)
				}
				time.Sleep(200 * time.Microsecond)
				in.Add(-1)
				unlock()
			})
		}
		wg.Wait()
		if n := overlaps.Load(); n > 0 {
			t.Fatalf("round %d: %d writers held the lock at once", round, n)
		}
	}
}
