package mdfiles

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/guilbep/git-board/next/domain/issues"
	"github.com/guilbep/git-board/next/kernel"
)

// Folder is a Markdown folder's files, on a board: read in before each
// write, rendered after (Writing).
type Folder struct {
	dir    string // the Markdown files'
	known  string // what is known of them: in this machine's folder
	lock   func(ctx context.Context) (func(), error)
	Issues *issues.Service
	notice func(format string, args ...any)

	mu      sync.Mutex // one write at a time in this process; lock across processes
	wmu     sync.Mutex // written
	written map[kernel.ID]bool
	parsed  int // the files whose bytes were read: the tests'
}

// Open is the folder dir's files on the board svc writes, svc's writes
// bracketed by them. lock takes the folder's lock (the datastore's
// LockFiles); folder is this machine's own; notice is told what a write
// leaves aside (a conflict, a file that didn't render), as v1 logs it.
func Open(dir, folder string, lock func(context.Context) (func(), error), svc *issues.Service, notice func(string, ...any)) *Folder {
	h := sha256.Sum256([]byte(dir))
	f := &Folder{dir: dir, known: filepath.Join(folder, "v2", "md-"+hex.EncodeToString(h[:8])+".files"),
		lock: lock, Issues: svc, notice: notice, written: map[kernel.ID]bool{}}
	svc.Board.Observe(f.observe)
	svc.Writing = f.Writing
	return f
}

// Dir is the folder the files are in.
func (f *Folder) Dir() string { return f.dir }

// observe keeps the issues this process wrote, rendered at the write's
// end.
func (f *Folder) observe(_ context.Context, cs []kernel.Change) {
	f.wmu.Lock()
	defer f.wmu.Unlock()
	for _, c := range cs {
		if c.Kind == issues.Kind.Name() {
			f.written[c.ID] = true
		}
	}
}

type within struct{ f *Folder }

// Writing begins a write: under the folder's lock, held until it ends,
// the hand edits are read in; its end renders the files and lets the
// lock go. A write within one is in it already.
func (f *Folder) Writing(ctx context.Context) (context.Context, func(), error) {
	if ctx.Value(within{f}) != nil {
		return ctx, func() {}, nil
	}
	f.mu.Lock()
	unlock, err := f.lock(ctx)
	if err != nil {
		f.mu.Unlock()
		return ctx, nil, fmt.Errorf("markdown: %w", err)
	}
	ctx = context.WithValue(ctx, within{f}, true)
	k := f.load()
	f.read(ctx, k)
	return ctx, func() {
		defer f.mu.Unlock()
		defer unlock()
		if err := f.render(ctx, k); err != nil {
			f.notice("markdown: %v", err)
		}
		f.save(k)
	}, nil
}

// Render renders the files, under the folder's lock, reading nothing
// in: a board copied into the folder (v1's renderTo).
func (f *Folder) Render(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	unlock, err := f.lock(ctx)
	if err != nil {
		return fmt.Errorf("markdown: %w", err)
	}
	defer unlock()
	k := f.load()
	defer f.save(k)
	return f.render(ctx, k)
}

// stat is what tells a file changed: a write changes its size, or its
// times.
type stat struct {
	Size, Mtime, Ctime int64
	Ino                uint64
}

// changed is when the file last changed: its ctime, which no one sets,
// unless its mtime is later. A tool may set the mtime back (cp -p, rsync
// -t, touch -d): a file just edited so is still a recent one.
func (s stat) changed() int64 { return max(s.Mtime, s.Ctime) }

// entry is what is known of a file, by name: its stat when it was last
// written or read, at Seen, the hash of its bytes then, the issue it
// holds ("" for a file that isn't one), and the version it was rendered
// at ("" when it wasn't, or was read since).
type entry struct {
	Stat    stat
	Seen    int64
	Hash    string
	ID      kernel.ID
	Version kernel.Version
}

// known is what is known of the folder's files: by name, and the sum
// they were rendered under (issues.Heads).
type known struct {
	Sum   string
	Files map[string]entry
}

// The file of what is known: its format's line, the sum's, then a line a
// file: name, id, version, size, mtime, ctime, inode, seen, hash; each
// field after a tab, "-" for none. A name holding a tab or a line feed
// isn't kept: such a file is read each time.
const knownFormat = "git-board md files 1"

func (f *Folder) load() *known {
	k := &known{Files: map[string]entry{}}
	data, err := os.ReadFile(f.known)
	if err != nil {
		return k
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) < 2 || lines[0] != knownFormat {
		return k
	}
	k.Sum = lines[1]
	for _, l := range lines[2:] {
		p := strings.Split(l, "\t")
		if len(p) != 9 {
			continue
		}
		var e entry
		var n [5]int64
		ok := true
		for i := range n {
			v, err := strconv.ParseInt(p[3+i], 10, 64)
			ok = ok && err == nil
			n[i] = v
		}
		if !ok {
			continue
		}
		e.Stat = stat{Size: n[0], Mtime: n[1], Ctime: n[2], Ino: uint64(n[3])}
		e.Seen, e.Hash = n[4], p[8]
		if p[1] != "-" {
			e.ID = kernel.ID(p[1])
		}
		if p[2] != "-" {
			e.Version = kernel.Version(p[2])
		}
		k.Files[p[0]] = e
	}
	return k
}

// save writes what is known, for the next process: a reader sees the
// old file or the new one.
func (f *Folder) save(k *known) {
	var b strings.Builder
	b.WriteString(knownFormat + "\n" + k.Sum + "\n")
	for name, e := range k.Files {
		if strings.ContainsAny(name, "\t\n") {
			continue
		}
		fmt.Fprintf(&b, "%s\t%s\t%s\t%d\t%d\t%d\t%d\t%d\t%s\n", name, cmp.Or(string(e.ID), "-"), cmp.Or(string(e.Version), "-"),
			e.Stat.Size, e.Stat.Mtime, e.Stat.Ctime, int64(e.Stat.Ino), e.Seen, e.Hash)
	}
	if err := os.MkdirAll(filepath.Dir(f.known), 0o755); err != nil {
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(f.known), filepath.Base(f.known)+".tmp*")
	if err != nil {
		return
	}
	_, err = tmp.WriteString(b.String())
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), f.known)
	}
	if err != nil {
		os.Remove(tmp.Name())
	}
}

func hashOf(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:16])
}

// unchanged says a file is as it was last written or read: its stat
// the one kept, and kept a second or more after its last change, so that
// a coarse clock can't hide a write since; else its bytes hash the same.
// data is its bytes when they were read.
func unchanged(path string, info fs.FileInfo, e entry, ok bool) (same bool, data []byte, err error) {
	st := statOf(info)
	if ok && e.Stat == st && e.Seen-st.changed() > int64(time.Second) {
		return true, nil, nil
	}
	data, err = os.ReadFile(path)
	if err != nil {
		return false, nil, err
	}
	return ok && hashOf(data) == e.Hash, data, nil
}

// remember keeps a file's stat and hash, as just written or read.
func remember(k *known, path, name string, data []byte, id kernel.ID, v kernel.Version) {
	e := entry{Hash: hashOf(data), ID: id, Version: v, Seen: time.Now().UnixNano()}
	if info, err := os.Stat(path); err == nil {
		e.Stat = statOf(info)
	}
	k.Files[name] = e
}

// read reads the hand edits in: each *.md file whose stat moved since it
// was last written or read, in the order of their names (v1 reads them
// all, the unchanged ones to no effect). What it can't read in is told,
// and the write goes on.
func (f *Folder) read(ctx context.Context, k *known) {
	entries, err := os.ReadDir(f.dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		f.notice("markdown: reading %s: %v", f.dir, err)
		return
	}
	on := make(map[string]bool, len(entries))
	im := &importer{f: f}
	var conflicts []string
	var failed error
	for _, de := range entries { // by name
		name := de.Name()
		if !strings.HasSuffix(name, ".md") || de.IsDir() {
			continue
		}
		on[name] = true
		if failed != nil {
			continue
		}
		path := filepath.Join(f.dir, name)
		info, err := de.Info()
		if err != nil {
			continue // gone meanwhile
		}
		e, ok := k.Files[name]
		same, data, err := unchanged(path, info, e, ok)
		if data != nil {
			f.parsed++
		}
		if err != nil {
			continue
		}
		if same {
			if data != nil { // read to be sure: from now on, its stat is
				remember(k, path, name, data, e.ID, e.Version)
			}
			continue
		}
		pf, perr := parse(data)
		if perr != nil { // not an issue's: left alone
			remember(k, path, name, data, "", "")
			continue
		}
		im.conflicts = nil
		id, rewrite, err := im.read(ctx, name, pf)
		conflicts = append(conflicts, im.conflicts...)
		if err != nil {
			failed = fmt.Errorf("%s: %w", name, err)
			continue
		}
		if rewrite != nil {
			if err := os.WriteFile(path, rewrite, 0o644); err != nil {
				failed = err
				continue
			}
			data = rewrite
		}
		remember(k, path, name, data, id, "") // rendered again, or removed, at the end
	}
	for name := range k.Files {
		if !on[name] {
			delete(k.Files, name) // taken by hand: rendered again
		}
	}
	if failed != nil {
		f.notice("markdown: reading %s: %v", f.dir, failed)
	}
	for _, c := range conflicts {
		f.notice("markdown: %s", c)
	}
}

// render writes the files where they moved from the board: each visible
// issue whose version, name, or sum moved since its file was written,
// and the ones this process wrote; a file of an issue under another
// name, or of a deleted one, is removed (v1's ExportMarkdown, which
// renders every issue).
func (f *Folder) render(ctx context.Context, k *known) error {
	f.wmu.Lock()
	written := f.written
	f.written = map[kernel.ID]bool{}
	f.wmu.Unlock()
	heads, sum, err := f.Issues.Heads(ctx)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(f.dir, 0o755); err != nil {
		return err
	}
	all := sum != k.Sum
	nameOf, err := names(heads)
	if err != nil {
		return err
	}
	listed := make(map[kernel.ID]int, len(heads))
	for i, h := range heads {
		listed[kernel.ID(h.ID)] = i
	}
	at := map[kernel.ID]string{} // a rendered issue's file, as known
	// a file of an issue under another name goes, and a deleted one's
	for _, name := range slices.Sorted(mapsKeys(k.Files)) {
		e := k.Files[name]
		if e.ID == "" || !oneName(name) { // a name an older version kept: never removed outside the folder
			continue
		}
		i, ok := listed[e.ID]
		gone := ok && nameOf[i] != name
		if !ok {
			st, err := issues.Kind.Get(ctx, f.Issues.Board, e.ID)
			gone = err == nil && st.State.Deleted
		}
		if gone {
			if err := os.Remove(filepath.Join(f.dir, name)); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			delete(k.Files, name)
			continue
		}
		if ok && e.Version != "" {
			at[e.ID] = name
		}
	}
	var ids []kernel.ID
	var which []int
	for i, h := range heads {
		id := kernel.ID(h.ID)
		e, ok := k.Files[nameOf[i]]
		if all || written[id] || !ok || at[id] != nameOf[i] || e.ID != id || e.Version != h.Version {
			ids, which = append(ids, id), append(which, i)
		}
	}
	if len(ids) > 0 {
		ths, err := f.Issues.Threads(ctx, ids)
		if err != nil {
			return err
		}
		for j, th := range ths {
			i := which[j]
			name, path := nameOf[i], filepath.Join(f.dir, nameOf[i])
			data, err := Render(th)
			if err != nil {
				return err
			}
			if cur, err := os.ReadFile(path); err != nil || !bytes.Equal(cur, data) {
				if err := os.WriteFile(path, data, 0o644); err != nil {
					return err
				}
			}
			remember(k, path, name, data, ids[j], heads[i].Version)
		}
	}
	k.Sum = sum
	return nil
}

func mapsKeys[V any](m map[string]V) func(func(string) bool) {
	return func(yield func(string) bool) {
		for k := range m {
			if !yield(k) {
				return
			}
		}
	}
}
