package syncer

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// tryLock locks one byte of f, far past its end, without waiting: errHeld
// when another holds it. The text stays readable, as Windows keeps other
// processes from reading a locked range.
func tryLock(f *os.File) error {
	err := windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0,
		&windows.Overlapped{Offset: 0, OffsetHigh: 0x7fffffff})
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return errHeld
	}
	return err
}

// unlockFile drops tryLock's lock.
func unlockFile(f *os.File) {
	_ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, &windows.Overlapped{Offset: 0, OffsetHigh: 0x7fffffff})
}
