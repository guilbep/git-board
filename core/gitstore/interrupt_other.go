//go:build !windows

package gitstore

import "os"

// interrupt asks git to stop: SIGINT, on which it removes its lock files.
func interrupt(p *os.Process) error { return p.Signal(os.Interrupt) }
