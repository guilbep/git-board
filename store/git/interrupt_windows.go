package git

import "os"

// interrupt kills git: Windows has no SIGINT to send.
func interrupt(p *os.Process) error { return p.Kill() }
