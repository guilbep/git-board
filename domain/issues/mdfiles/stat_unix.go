//go:build linux || darwin || freebsd || netbsd || openbsd

package mdfiles

import (
	"io/fs"
	"syscall"
)

func statOf(fi fs.FileInfo) stat {
	st := stat{Size: fi.Size(), Mtime: fi.ModTime().UnixNano()}
	if sys, ok := fi.Sys().(*syscall.Stat_t); ok {
		st.Ctime = ctimeOf(sys)
		st.Ino = sys.Ino
	}
	return st
}
