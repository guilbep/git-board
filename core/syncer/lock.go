package syncer

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/guilbep/git-board/core/gitstore"
)

// errHeld: another open file holds the OS lock.
var errHeld = errors.New("held")

// lock takes the run's lock: an OS lock on board/sync.lock, in git's
// common directory, held while the file is open. The OS drops it when the
// process ends, however it ends, so a file a killed run left keeps no one
// out. The file is never removed: a run that opened it just before would
// lock a file no other run sees. It holds the journal (journal.go), which
// the run started at started goes on from.
func lock(s *gitstore.Store, started int64) (*journal, error) {
	dir := filepath.Join(s.CommonDir(), "board")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "sync.lock")
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}
	if err := takeLock(f); err != nil {
		f.Close()
		if errors.Is(err, errHeld) {
			return nil, fmt.Errorf("%w (process %s): %s", ErrLocked, holder(path), path)
		}
		return nil, fmt.Errorf("syncer: locking %s: %w", path, err)
	}
	j, err := openJournal(f, started)
	if err != nil {
		unlockFile(f)
		f.Close()
		return nil, err
	}
	return j, nil
}

// takeLock is tryLock, waiting out a look at the lock (Running's, Cut's),
// which holds it an instant: errHeld once it was held a tenth of a second.
func takeLock(f *os.File) error {
	err := tryLock(f)
	for i := 0; i < 10 && errors.Is(err, errHeld); i++ {
		time.Sleep(10 * time.Millisecond)
		err = tryLock(f)
	}
	return err
}

// holder is the process id the lock's holder wrote on the file's first
// line, or "?".
func holder(path string) string {
	data, _ := os.ReadFile(path)
	if head, _, _ := readJournal(data); head.PID > 0 {
		return strconv.Itoa(head.PID)
	}
	return "?"
}

// Running says a run holds the lock now, in this process or another.
func Running(s *gitstore.Store) bool {
	f, err := os.OpenFile(filepath.Join(s.CommonDir(), "board", "sync.lock"), os.O_RDWR, 0)
	if err != nil {
		return false // no run ever took it
	}
	defer f.Close()
	err = takeLock(f) // Cut's look isn't a run
	if err == nil {
		unlockFile(f) // it was free: let it go before a fork can share it
	}
	return errors.Is(err, errHeld)
}
