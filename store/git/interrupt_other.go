//go:build !windows

package git

import "os"

// interrupt asks git to stop: on SIGINT it removes its lock files.
func interrupt(p *os.Process) error { return p.Signal(os.Interrupt) }
