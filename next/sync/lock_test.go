package sync

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/guilbep/git-board/next/store/mem"
)

// A run waits out a look at the lock (Running's, Cut's), which holds it an
// instant.
func TestALookDoesntStopARun(t *testing.T) {
	t.Parallel()
	s := mem.New(t.TempDir())
	j, err := Lock(s, 1)
	if err != nil {
		t.Fatal(err)
	}
	j.Unlock()
	f, err := os.OpenFile(filepath.Join(s.Folder(), "sync.lock"), os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := tryLock(f); err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(20 * time.Millisecond)
		unlockFile(f)
		f.Close()
	}()
	if j, err = Lock(s, 2); err != nil {
		t.Fatalf("a run during a look: %v", err)
	}
	j.Unlock()
}
