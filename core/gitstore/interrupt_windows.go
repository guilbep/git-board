package gitstore

import "os"

// interrupt stops git: Windows can't send a process os.Interrupt, so it is
// killed, and may leave a <ref>.lock behind.
func interrupt(p *os.Process) error { return p.Kill() }
