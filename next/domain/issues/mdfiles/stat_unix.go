//go:build !windows

package mdfiles

import (
	"io/fs"
	"syscall"
)

func statOf(fi fs.FileInfo) stat {
	st := stat{Size: fi.Size(), Mtime: fi.ModTime().UnixNano()}
	if sys, ok := fi.Sys().(*syscall.Stat_t); ok {
		st.Ctime = sys.Ctim.Sec*1e9 + sys.Ctim.Nsec
		st.Ino = sys.Ino
	}
	return st
}
