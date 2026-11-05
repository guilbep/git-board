package md

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// A listing hashes every pack file (datastores.md: a version is the
// file's hash): 34 MB at 5,000 issues, most of a list's time. So the
// versions are kept in this machine's folder, each with its file's stat,
// as git's index keeps a file's: a file whose stat is the one kept isn't
// read again. A version is kept only when its file was last written a
// second or more before it was read, and its stat was the same before
// and after: a file written again since has another stat, however coarse
// its clock.

// stat is what tells a file changed: a write changes its size, or its
// times.
type stat struct {
	size, mtime, ctime int64
	ino                uint64
}

// known is a file's version at a stat.
type known struct {
	stat    stat
	version string
}

// versions are what a Store knows of its files' versions, by
// <kind>/<id>, read from the folder's file once, written back when a
// listing learned more.
type versions struct {
	mu    sync.Mutex
	path  string
	read  bool
	by    map[string]known
	dirty bool
}

func (s *Store) versionsPath() string {
	h := sha256.Sum256([]byte(s.dir))
	return filepath.Join(s.folder, "v2", "md-"+hex.EncodeToString(h[:8])+".versions")
}

// cached is the version of the file named key (<kind>/<id>) at path,
// read and hashed when its stat isn't the one kept; "" for a file with
// no pack, and gone for one that isn't there.
func (s *Store) cached(key, path string, info fs.FileInfo) (v string, gone bool, err error) {
	vs := &s.vers
	st := statOf(info)
	vs.mu.Lock()
	if !vs.read {
		vs.read, vs.by, vs.path = true, readVersions(s.versionsPath()), s.versionsPath()
	}
	k, ok := vs.by[key]
	vs.mu.Unlock()
	if ok && k.stat == st {
		return k.version, false, nil
	}
	start := time.Now()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", true, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("md: %w", err)
	}
	v = version(whole(data))
	if after, err := os.Stat(path); err == nil && statOf(after) == st && start.Sub(after.ModTime()) > time.Second {
		vs.mu.Lock()
		vs.by[key], vs.dirty = known{st, v}, true
		vs.mu.Unlock()
	}
	return v, false, nil
}

// saveVersions writes what a listing learned, for the next process.
func (s *Store) saveVersions() {
	vs := &s.vers
	vs.mu.Lock()
	defer vs.mu.Unlock()
	if !vs.dirty {
		return
	}
	vs.dirty = false
	var b bytes.Buffer
	b.WriteString("git-board md versions 1\n")
	for key, k := range vs.by {
		fmt.Fprintf(&b, "%s %d %d %d %d %s\n", key, k.stat.size, k.stat.mtime, k.stat.ctime, k.stat.ino, k.version)
	}
	if err := os.MkdirAll(filepath.Dir(vs.path), 0o755); err != nil {
		return
	}
	f, err := os.CreateTemp(filepath.Dir(vs.path), filepath.Base(vs.path)+".tmp*")
	if err != nil {
		return
	}
	_, err = f.Write(b.Bytes())
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(f.Name(), vs.path) // a reader sees the old file or the new one
	}
	if err != nil {
		os.Remove(f.Name())
	}
}

// readVersions reads the versions file at path; a missing or foreign
// one is none known.
func readVersions(path string) map[string]known {
	by := map[string]known{}
	f, err := os.Open(path)
	if err != nil {
		return by
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if !sc.Scan() || sc.Text() != "git-board md versions 1" {
		return by
	}
	for sc.Scan() {
		p := strings.Fields(sc.Text())
		if len(p) != 6 {
			continue
		}
		var k known
		var e [4]error
		k.stat.size, e[0] = strconv.ParseInt(p[1], 10, 64)
		k.stat.mtime, e[1] = strconv.ParseInt(p[2], 10, 64)
		k.stat.ctime, e[2] = strconv.ParseInt(p[3], 10, 64)
		k.stat.ino, e[3] = strconv.ParseUint(p[4], 10, 64)
		if e[0] == nil && e[1] == nil && e[2] == nil && e[3] == nil {
			k.version = p[5]
			by[p[0]] = k
		}
	}
	return by
}
