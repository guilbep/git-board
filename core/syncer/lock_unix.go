//go:build !windows

package syncer

import (
	"errors"
	"os"
	"syscall"
)

// tryLock takes f's flock without waiting: errHeld when another holds it.
func tryLock(f *os.File) error {
	err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return errHeld
	}
	return err
}

// unlockFile drops f's flock, for every process sharing the open file.
func unlockFile(f *os.File) { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }
