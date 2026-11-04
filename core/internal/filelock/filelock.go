// Package filelock is the OS's lock on an open file: flock, or
// LockFileEx on Windows. The OS drops it when the process ends, however it
// ends, so a crashed holder keeps no one out.
//
// A lock is the open file's: two opens of one file, in one process too,
// are two holders. A lock file is never removed while in use: one opened
// just before would lock a file no other holder sees.
package filelock

import "errors"

// ErrHeld: another open file holds the lock.
var ErrHeld = errors.New("filelock: held")
