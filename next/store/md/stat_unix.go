//go:build !windows

package md

import (
	"io/fs"
	"syscall"
)

func statOf(fi fs.FileInfo) stat {
	st := stat{size: fi.Size(), mtime: fi.ModTime().UnixNano()}
	if sys, ok := fi.Sys().(*syscall.Stat_t); ok {
		st.ctime = sys.Ctim.Sec*1e9 + sys.Ctim.Nsec
		st.ino = sys.Ino
	}
	return st
}
