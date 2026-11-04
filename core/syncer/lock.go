package syncer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/guilbep/git-board/core/datastore"
	"github.com/guilbep/git-board/core/internal/filelock"
	"github.com/guilbep/git-board/core/oplog"
)

// errHeld: another open file holds the OS lock.
var errHeld = filelock.ErrHeld

// tryLock takes f's OS lock without waiting: errHeld when another holds it.
func tryLock(f *os.File) error { return filelock.TryLock(f) }

// unlockFile drops f's OS lock, for every process sharing the open file.
func unlockFile(f *os.File) { filelock.Unlock(f) }

// lock takes the run's lock: an OS lock on board/sync.lock, in git's
// common directory, held while the file is open. The OS drops it when the
// process ends, however it ends, so a file a killed run left keeps no one
// out. The file is never removed: a run that opened it just before would
// lock a file no other run sees. It holds the journal (journal.go), which
// the run started at started goes on from.
//
// A datastore every machine shares (a datastore.Locker: PostgreSQL) holds
// the board's lock first, for every machine: two runs on one board would
// each create the same forge issue. The journal stays the machine's.
func lock(s oplog.Datastore, started int64) (*journal, error) {
	release := func() {}
	if l, ok := s.(datastore.Locker); ok {
		var err error
		if release, err = takeShared(l); err != nil {
			return nil, err
		}
	}
	j, err := lockFile(s, started)
	if err != nil {
		release()
		return nil, err
	}
	j.release = release
	return j, nil
}

// lockName is the board's lock a run takes in a datastore.Locker.
const lockName = "sync"

// takeShared takes the board's lock in l, waiting out a look at it
// (Running's) as takeLock does.
func takeShared(l datastore.Locker) (func(), error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	unlock, err := l.Lock(ctx, lockName)
	for i := 0; i < 10 && errors.Is(err, datastore.ErrHeld); i++ {
		time.Sleep(10 * time.Millisecond)
		unlock, err = l.Lock(ctx, lockName)
	}
	if errors.Is(err, datastore.ErrHeld) {
		return nil, fmt.Errorf("%w (on another machine, or in another process): %w", ErrLocked, err)
	}
	if err != nil {
		return nil, fmt.Errorf("syncer: locking the board: %w", err)
	}
	return unlock, nil
}

// lockFile takes the machine's lock, and opens its journal.
func lockFile(s oplog.Datastore, started int64) (*journal, error) {
	dir := s.Folder()
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

// Running says a run holds the lock now, in this process or another; on a
// datastore every machine shares, on any machine.
func Running(s oplog.Datastore) bool {
	if l, ok := s.(datastore.Locker); ok {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		unlock, err := l.Lock(ctx, lockName)
		if err == nil {
			unlock() // it was free: a look isn't a run
		}
		if errors.Is(err, datastore.ErrHeld) {
			return true
		}
	}
	f, err := os.OpenFile(filepath.Join(s.Folder(), "sync.lock"), os.O_RDWR, 0)
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
