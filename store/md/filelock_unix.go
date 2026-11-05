//go:build !windows

package md

import (
	"errors"
	"os"
	"syscall"
)

// errHeld: another open file holds the lock.
var errHeld = errors.New("md: lock held")

// tryLock takes f's flock without waiting: errHeld when another holds
// it. The OS drops it when the process ends, however it ends.
func tryLock(f *os.File) error {
	err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return errHeld
	}
	return err
}

// unlockFile drops f's flock.
func unlockFile(f *os.File) { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }
