// Package md is the Markdown folder datastore (docs/spec/datastores.md,
// Markdown folder): a folder of the code checkout keeps each entity's
// packs, one a line, in <dir>/.board/<kind>/<id>.jsonl, committed with the
// code. Git's union driver merges two branches' appends; a reader orders
// the packs by (clock, hash), as it orders two clones'.
//
// A version is the file's hash: it changes whenever the file does, a
// branch switched to included. A write takes the entity's lock file,
// checks the hash and appends. A reader may see an append in parts: a
// last line without its line feed that isn't JSON is not written yet.
package md

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/guilbep/git-board/next/kernel/pack"
	"github.com/guilbep/git-board/next/store"
)

// Board is the folder, in dir, that holds the packs.
const Board = ".board"

// Prefix is board.datastore's prefix for a Markdown folder:
// markdown:<dir>.
const Prefix = "markdown:"

const (
	gitattributes = "*.jsonl merge=union\n/numbers merge=binary\n"
	gitignore     = "*.lock\n*.takeover\n"
)

// staleLock is how old a lock file is when its writer is taken for dead;
// a writer waits as long before it gives up.
const staleLock = 30 * time.Second

// Store is a board kept in a folder.
type Store struct {
	dir    string // the Markdown files'; the packs are in dir/.board
	folder string // this machine's own
}

var (
	_ store.Datastore = (*Store)(nil)
	_ store.Counter   = (*Store)(nil)
)

// Open keeps the board in dir, making dir/.board, its .gitattributes and
// its .gitignore when they aren't there. folder is this machine's own.
func Open(dir, folder string) (*Store, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	s := &Store{dir: abs, folder: folder}
	if err := os.MkdirAll(s.root(), 0o755); err != nil {
		return nil, fmt.Errorf("md: %w", err)
	}
	for name, text := range map[string]string{".gitattributes": gitattributes, ".gitignore": gitignore} {
		path := filepath.Join(s.root(), name)
		if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
			if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
				return nil, fmt.Errorf("md: %w", err)
			}
		}
	}
	return s, nil
}

// Caps: the counter is .board/numbers. A version is a hash, not a name
// for good: a file only grows, but a branch switched to may hold others.
func (s *Store) Caps() store.Caps { return store.Caps{Counter: s, Put: s} }

// Folder is this machine's own folder, never the checkout's.
func (s *Store) Folder() string { return s.folder }

// Dir is the folder the Markdown files go in.
func (s *Store) Dir() string { return s.dir }

// Close has nothing to let go.
func (s *Store) Close() error { return nil }

func (s *Store) root() string { return filepath.Join(s.dir, Board) }

func (s *Store) path(kind, id string) string { return filepath.Join(s.root(), kind, id+".jsonl") }

// Heads lists the kinds' entities: every <id>.jsonl holding a pack, with
// its hash. A file only grows, and is never renamed: one that exists
// throughout a listing is in it.
func (s *Store) Heads(_ context.Context, kinds ...string) ([]store.Head, error) {
	var out []store.Head
	for _, kind := range slices.Sorted(slices.Values(kinds)) {
		entries, err := os.ReadDir(filepath.Join(s.root(), kind))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("md: %w", err)
		}
		for _, e := range entries { // ReadDir sorts by name: by id
			id, ok := strings.CutSuffix(e.Name(), ".jsonl")
			if !ok || !store.IsID(id) || e.IsDir() {
				continue
			}
			data, err := readPacks(s.path(kind, id))
			if err != nil {
				return nil, err
			}
			if v := version(data); v != "" {
				out = append(out, store.Head{Kind: kind, ID: id, Version: v})
			}
		}
	}
	return out, nil
}

// Load reads each entity's file once: its packs and the version they are
// at, the current one whatever version the ref names, as a hash names no
// packs for good.
func (s *Store) Load(ctx context.Context, refs []store.Ref, each func(store.Ref, [][]byte, error) error) error {
	for _, r := range refs {
		if err := ctx.Err(); err != nil {
			return err
		}
		var packs [][]byte
		var data []byte
		var err error
		if store.IsID(r.ID) && r.Kind != "" && !strings.ContainsAny(r.Kind, `/\.`) {
			data, err = readPacks(s.path(r.Kind, r.ID))
		}
		if err == nil {
			r.Version = version(data)
			if packs = lines(data); len(packs) == 0 {
				err = fmt.Errorf("%w: %s %s", store.ErrNotFound, r.Kind, r.ID)
			}
		}
		if err := each(r, packs, err); err != nil {
			return err
		}
	}
	return nil
}

// Add appends a pack if the file is still at version prev ("" for one
// that holds no pack yet). Its clock must be one more than the highest
// of the file's packs: two branches' packs, merged, may share a clock.
func (s *Store) Add(_ context.Context, kind, id string, data []byte, prev string) (string, error) {
	if !store.IsID(id) || kind == "" || strings.ContainsAny(kind, `/\.`) {
		return "", fmt.Errorf("md: bad name %s %s", kind, id)
	}
	if bytes.ContainsAny(data, "\n\r") {
		return "", fmt.Errorf("md: %s %s: a pack is one line", kind, id)
	}
	clock, empty, err := pack.Header(data)
	if err != nil {
		return "", fmt.Errorf("md: %s %s: pack: %w", kind, id, err)
	}
	if empty {
		return "", fmt.Errorf("md: %s %s: %w", kind, id, pack.ErrEmpty)
	}
	path := s.path(kind, id)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("md: %w", err)
	}
	unlock, err := lock(path + ".lock")
	if err != nil {
		return "", err
	}
	defer unlock()
	cur, err := readFile(path)
	if err != nil {
		return "", err
	}
	if cur = whole(cur); version(cur) != prev { // under the lock, no append is half done
		return "", fmt.Errorf("%w: %s %s", store.ErrMoved, kind, id)
	}
	if want := highest(lines(cur)) + 1; clock != want {
		return "", fmt.Errorf("md: %s %s: a pack of clock %d on one of clock %d", kind, id, clock, want-1)
	}
	line := append(slices.Clip(data), '\n')
	if len(cur) > 0 && cur[len(cur)-1] != '\n' { // a hand edit took the last line feed
		line = append([]byte{'\n'}, line...)
	}
	if err := appendFile(path, line); err != nil {
		return "", err
	}
	return version(append(cur, line...)), nil
}

// Put appends the packs the file doesn't hold yet, in one write, if it is
// still at prev: datastore copy's, whose packs are another datastore's,
// clocks and all. It answers the new version, prev when nothing was
// missing.
func (s *Store) Put(_ context.Context, kind, id string, packs [][]byte, prev string) (string, error) {
	if !store.IsID(id) || kind == "" || strings.ContainsAny(kind, `/\.`) {
		return "", fmt.Errorf("md: bad name %s %s", kind, id)
	}
	path := s.path(kind, id)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("md: %w", err)
	}
	unlock, err := lock(path + ".lock")
	if err != nil {
		return "", err
	}
	defer unlock()
	cur, err := readFile(path)
	if err != nil {
		return "", err
	}
	if cur = whole(cur); version(cur) != prev {
		return "", fmt.Errorf("%w: %s %s", store.ErrMoved, kind, id)
	}
	have := map[string]bool{}
	for _, p := range lines(cur) {
		have[string(p)] = true
	}
	var add []byte
	for _, p := range packs {
		p = bytes.TrimSpace(p)
		if bytes.ContainsAny(p, "\n\r") {
			return "", fmt.Errorf("md: %s %s: a pack is one line", kind, id)
		}
		if len(p) > 0 && !have[string(p)] {
			have[string(p)] = true
			add = append(append(add, p...), '\n')
		}
	}
	if len(add) == 0 {
		return prev, nil
	}
	if len(cur) > 0 && cur[len(cur)-1] != '\n' {
		add = append([]byte{'\n'}, add...)
	}
	if err := appendFile(path, add); err != nil {
		return "", err
	}
	return version(append(cur, add...)), nil
}

// appendFile appends b to the file at path, in one write.
func appendFile(path string, b []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("md: %w", err)
	}
	_, err = f.Write(b)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("md: %w", err)
	}
	return nil
}

// highest is the highest clock of packs; a line that doesn't read counts
// for none (the entity is broken: no writer folds it).
func highest(packs [][]byte) uint64 {
	var h uint64
	for _, p := range packs {
		if c, _, err := pack.Header(p); err == nil {
			h = max(h, c)
		}
	}
	return h
}

// version is the first 16 bytes of the SHA-256 of a pack file's whole
// bytes, in hex; "" for one of white space alone, which holds no entity.
func version(data []byte) string {
	if len(bytes.TrimSpace(data)) == 0 {
		return ""
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:16])
}

// readFile is a file's bytes; none for a file that isn't there.
func readFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("md: %w", err)
	}
	return data, nil
}

// readPacks is a pack file's whole packs (whole).
func readPacks(path string) ([]byte, error) {
	data, err := readFile(path)
	return whole(data), err
}

// whole is data without a last line being appended. A reader may see an
// append in parts (v1's 12cfe5d: a read cut at a page, once in 3,000
// appends): Add writes each pack with its line feed, and a pack cut
// short is never JSON, while a whole one whose line feed a hand edit took
// is.
func whole(data []byte) []byte {
	if i := bytes.LastIndexByte(data, '\n'); !json.Valid(data[i+1:]) {
		return data[:i+1]
	}
	return data
}

// lines are a pack file's packs: each line trimmed of white space, the
// empty ones left out. A line twice (a merge's) is read twice, and
// counted once by the order, as two packs of the same bytes are one.
func lines(data []byte) [][]byte {
	var out [][]byte
	for len(data) > 0 {
		line := data
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			line, data = data[:i], data[i+1:]
		} else {
			data = nil
		}
		if line = bytes.Trim(line, " \t\r"); len(line) > 0 {
			out = append(out, line[:len(line):len(line)])
		}
	}
	return out
}

// lock takes a lock file, waiting while another writer holds it, trying
// every 5 ms; one older than staleLock is a dead writer's, taken over.
// After staleLock of waiting, the write fails as moved.
func lock(path string) (func(), error) {
	for start := time.Now(); ; {
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			f.Close()
			return func() { os.Remove(path) }, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, fmt.Errorf("md: %w", err)
		}
		if fi, err := os.Stat(path); err == nil && time.Since(fi.ModTime()) > staleLock {
			takeOver(path)
			continue
		}
		if time.Since(start) > staleLock {
			return nil, fmt.Errorf("%w: %s is held", store.ErrMoved, path)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// takeOver removes a stale lock file. Waiters take over one at a time,
// under the OS lock on path.takeover, and look at its age again under
// it: one that found the lock stale never removes the fresh lock another
// took over meanwhile. The OS lets that lock go when its holder dies; the
// file stays, as a lock file in use is never removed.
func takeOver(path string) {
	f, err := os.OpenFile(path+".takeover", os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		time.Sleep(time.Millisecond)
		return
	}
	defer f.Close()
	if err := tryLock(f); err != nil {
		time.Sleep(time.Millisecond) // another takes over: look again
		return
	}
	defer unlockFile(f)
	if fi, err := os.Stat(path); err == nil && time.Since(fi.ModTime()) > staleLock {
		os.Remove(path)
	}
}

// LockFiles takes the folder's lock, .board/board.lock, waiting while
// another holds it, in this process or another, until ctx is done: the
// Markdown files are read in, written and rendered by one at a time. The
// OS lets it go when its holder dies.
func (s *Store) LockFiles(ctx context.Context) (unlock func(), err error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	f, err := os.OpenFile(filepath.Join(s.root(), "board.lock"), os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, fmt.Errorf("md: %w", err)
	}
	for {
		err := tryLock(f)
		if err == nil {
			return func() { unlockFile(f); f.Close() }, nil
		}
		if !errors.Is(err, errHeld) {
			f.Close()
			return nil, fmt.Errorf("md: locking %s: %w", f.Name(), err)
		}
		select {
		case <-ctx.Done():
			f.Close()
			return nil, fmt.Errorf("md: %s is held: %w", f.Name(), cmp.Or(context.Cause(ctx), ctx.Err()))
		case <-time.After(5 * time.Millisecond):
		}
	}
}
