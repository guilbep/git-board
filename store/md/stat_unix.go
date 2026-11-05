//go:build linux || darwin || freebsd || netbsd || openbsd

package md

import (
	"io/fs"
	"syscall"
)

func statOf(fi fs.FileInfo) stat {
	st := stat{size: fi.Size(), mtime: fi.ModTime().UnixNano()}
	if sys, ok := fi.Sys().(*syscall.Stat_t); ok {
		st.ctime = ctimeOf(sys)
		st.ino = sys.Ino
	}
	return st
}
