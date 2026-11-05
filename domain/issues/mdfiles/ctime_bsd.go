//go:build darwin || freebsd || netbsd || openbsd

package mdfiles

import "syscall"

func ctimeOf(s *syscall.Stat_t) int64 { return s.Ctimespec.Sec*1e9 + s.Ctimespec.Nsec }
