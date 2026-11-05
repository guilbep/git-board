package md

import "io/fs"

func statOf(fi fs.FileInfo) stat { return stat{size: fi.Size(), mtime: fi.ModTime().UnixNano()} }
