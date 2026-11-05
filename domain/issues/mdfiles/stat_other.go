//go:build !(linux || darwin || freebsd || netbsd || openbsd)

package mdfiles

import "io/fs"

func statOf(fi fs.FileInfo) stat { return stat{Size: fi.Size(), Mtime: fi.ModTime().UnixNano()} }
