//go:build !(linux || darwin || freebsd || netbsd || openbsd)

package md

import "io/fs"

func statOf(fi fs.FileInfo) stat { return stat{size: fi.Size(), mtime: fi.ModTime().UnixNano()} }
