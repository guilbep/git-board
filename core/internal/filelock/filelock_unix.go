//go:build !windows

package filelock

import (
	"errors"
	"os"
	"syscall"
)

// TryLock takes f's flock without waiting: ErrHeld when another holds it.
func TryLock(f *os.File) error {
	err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return ErrHeld
	}
	return err
}

// Unlock drops f's flock, for every process sharing the open file.
func Unlock(f *os.File) { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }
