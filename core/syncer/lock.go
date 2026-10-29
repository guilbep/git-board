package syncer

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/guilbep/git-board/core/gitstore"
)

// errHeld: another open file holds the OS lock.
var errHeld = errors.New("held")

// lock takes the run's lock: an OS lock on board/sync.lock, in git's
// common directory, held while the file is open. The OS drops it when the
// process ends, however it ends, so a file a killed run left keeps no one
// out. The file is never removed: a run that opened it just before would
// lock a file no other run sees.
func lock(s *gitstore.Store) (func(), error) {
	dir := filepath.Join(s.CommonDir(), "board")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "sync.lock")
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}
	if err := tryLock(f); err != nil {
		f.Close()
		if errors.Is(err, errHeld) {
			return nil, fmt.Errorf("%w (process %s): %s", ErrLocked, holder(path), path)
		}
		return nil, fmt.Errorf("syncer: locking %s: %w", path, err)
	}
	if err := f.Truncate(0); err != nil {
		f.Close()
		return nil, err
	}
	if _, err := fmt.Fprintf(f, "%d\n", os.Getpid()); err != nil {
		f.Close()
		return nil, err
	}
	// unlocked first: a process forking as it closes shares the open file
	// until it execs, and with it the lock
	return func() { unlockFile(f); f.Close() }, nil
}

// holder is the process id the lock's holder wrote on the file's first
// line, or "?".
func holder(path string) string {
	data, _ := os.ReadFile(path)
	if line, _, _ := bytes.Cut(data, []byte("\n")); len(line) > 0 {
		return string(line)
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
	err = tryLock(f)
	if err == nil {
		unlockFile(f) // it was free: let it go before a fork can share it
	}
	return errors.Is(err, errHeld)
}
