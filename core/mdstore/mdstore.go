// Package mdstore is the Markdown folder datastore (ADR 0002, section 5,
// "As a datastore"): a folder in the code checkout keeps each entity's
// packs, one JSON line per pack, in .board/<kind>/<id>.jsonl, beside the
// Markdown files internal/store renders from them. The board is then
// files: committed with the code, reviewed in a pull request, diffed with
// git diff.
//
// A pack file only grows. .board/.gitattributes merges them with git's
// union driver, so two branches that both wrote an issue merge with both
// packs kept: oplog orders them, as it orders packs from two clones.
//
// A version is the file's hash: it changes whenever the file does, a
// branch switched to included, though two branches' files may hold as many
// packs. A write takes the entity's lock file (<id>.jsonl.lock), checks the
// hash and appends: one writer at a time, on this machine and on a shared
// disk.
package mdstore

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/guilbep/git-board/core/datastore"
	"github.com/guilbep/git-board/core/internal/filelock"
)

// Board is the folder of a dir that holds the packs.
const Board = ".board"

// gitattributes merges the pack files line by line.
const gitattributes = "*.jsonl merge=union\n"

// gitignore keeps the lock files out of git: the packs' and the board's.
const gitignore = "*.lock\n*.takeover\n"

// staleLock is how old a lock file is when its writer is taken for dead.
const staleLock = 30 * time.Second

var entityID = regexp.MustCompile(`^[0-9a-f]{40}$`)

// Store is a board kept in a folder.
type Store struct {
	dir    string // the folder of the Markdown files; the packs are in dir/.board
	folder string // the machine's own folder
}

var _ datastore.Datastore = (*Store)(nil)

// Open keeps the board in dir, making dir/.board, its .gitattributes and
// its .gitignore when they aren't there. folder is the machine's own
// folder.
func Open(dir, folder string) (*Store, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	s := &Store{dir: abs, folder: folder}
	if err := os.MkdirAll(s.root(), 0o755); err != nil {
		return nil, fmt.Errorf("mdstore: %w", err)
	}
	for name, text := range map[string]string{".gitattributes": gitattributes, ".gitignore": gitignore} {
		path := filepath.Join(s.root(), name)
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
				return nil, fmt.Errorf("mdstore: %w", err)
			}
		}
	}
	return s, nil
}

// Dir is the folder the Markdown files go in.
func (s *Store) Dir() string { return s.dir }

// Name is where the board is kept, for a user to read.
func (s *Store) Name() string { return "Markdown files" }

// Where is the folder.
func (s *Store) Where() string { return s.dir }

func (s *Store) root() string { return filepath.Join(s.dir, Board) }

func (s *Store) path(kind, id string) string { return filepath.Join(s.root(), kind, id+".jsonl") }

// Heads are a kind's entities, each with its count of packs.
func (s *Store) Heads(_ context.Context, kind string) (map[string]string, error) {
	entries, err := os.ReadDir(filepath.Join(s.root(), kind))
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("mdstore: %w", err)
	}
	out := map[string]string{}
	for _, e := range entries {
		id, ok := strings.CutSuffix(e.Name(), ".jsonl")
		if !ok || !entityID.MatchString(id) {
			continue
		}
		data, err := readFile(s.path(kind, id))
		if err != nil {
			return nil, err
		}
		if len(bytes.TrimSpace(data)) > 0 {
			out[id] = version(data)
		}
	}
	return out, nil
}

// Packs are an entity's packs, and how many.
func (s *Store) Packs(_ context.Context, kind, id string) ([][]byte, string, error) {
	data, err := readFile(s.path(kind, id))
	if err != nil {
		return nil, "", err
	}
	packs, err := lines(data)
	if err != nil {
		return nil, "", err
	}
	if len(packs) == 0 {
		return nil, "", fmt.Errorf("%w: %s %s", datastore.ErrNotFound, kind, id)
	}
	return packs, version(data), nil
}

// version is a file's hash, "" for none.
func version(data []byte) string {
	if len(bytes.TrimSpace(data)) == 0 {
		return ""
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:16])
}

// readFile is a pack file's bytes; none for a file that isn't there.
func readFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("mdstore: %w", err)
	}
	return data, nil
}

// lines are a pack file's packs. A merge may leave a line twice: it is
// one pack, as oplog counts it.
func lines(data []byte) ([][]byte, error) {
	var out [][]byte
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(nil, 64<<20)
	for sc.Scan() {
		if line := bytes.TrimSpace(sc.Bytes()); len(line) > 0 {
			out = append(out, bytes.Clone(line))
		}
	}
	return out, sc.Err()
}

// Add appends a pack if the file is still at version prev ("" for one
// that must not exist yet).
func (s *Store) Add(_ context.Context, kind, id string, pack []byte, prev string) (string, error) {
	if bytes.ContainsAny(pack, "\n\r") {
		return "", fmt.Errorf("mdstore: a pack is one line")
	}
	path := s.path(kind, id)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("mdstore: %w", err)
	}
	unlock, err := lock(path + ".lock")
	if err != nil {
		return "", err
	}
	defer unlock()
	data, err := readFile(path)
	if err != nil {
		return "", err
	}
	if version(data) != prev {
		return "", fmt.Errorf("%w: %s %s", datastore.ErrMoved, kind, id)
	}
	line := append(bytes.Clone(pack), '\n')
	if len(data) > 0 && data[len(data)-1] != '\n' { // a hand edit took the last line feed
		line = append([]byte{'\n'}, line...)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return "", fmt.Errorf("mdstore: %w", err)
	}
	_, err = f.Write(line)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", fmt.Errorf("mdstore: %w", err)
	}
	return version(append(data, line...)), nil
}

// lock takes a lock file, waiting while another writer holds it; one left
// by a writer dead staleLock ago is taken over.
func lock(path string) (func(), error) {
	for start := time.Now(); ; {
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			f.Close()
			return func() { os.Remove(path) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("mdstore: %w", err)
		}
		if fi, err := os.Stat(path); err == nil && time.Since(fi.ModTime()) > staleLock {
			takeOver(path)
			continue
		}
		if time.Since(start) > staleLock {
			return nil, fmt.Errorf("%w: %s is held", datastore.ErrMoved, path)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// takeOver removes a stale lock file. Waiters take over one at a time,
// holding the OS lock on path.takeover, and look again under it: one that
// found the lock stale never deletes the fresh lock another took over
// meanwhile. The OS lets that lock go when its holder dies, so a takeover
// cut short keeps no one out; the file stays, as a lock file in use is
// never removed.
func takeOver(path string) {
	f, err := os.OpenFile(path+".takeover", os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		time.Sleep(time.Millisecond)
		return
	}
	defer f.Close()
	if err := filelock.TryLock(f); err != nil {
		time.Sleep(time.Millisecond) // another takes over: look again
		return
	}
	defer filelock.Unlock(f)
	if fi, err := os.Stat(path); err == nil && time.Since(fi.ModTime()) > staleLock {
		os.Remove(path)
	}
}

// LockFiles takes the board's lock, .board/board.lock, waiting while
// another holds it, in this process or another, until ctx is done: the
// Markdown files are read in, written and rendered by one at a time
// (internal/store). The OS lets it go when its holder dies; the file
// stays, as a lock file in use is never removed.
func (s *Store) LockFiles(ctx context.Context) (unlock func(), err error) {
	f, err := os.OpenFile(filepath.Join(s.root(), "board.lock"), os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, fmt.Errorf("mdstore: %w", err)
	}
	for {
		err := filelock.TryLock(f)
		if err == nil {
			return func() { filelock.Unlock(f); f.Close() }, nil
		}
		if !errors.Is(err, filelock.ErrHeld) {
			f.Close()
			return nil, fmt.Errorf("mdstore: locking %s: %w", f.Name(), err)
		}
		select {
		case <-ctx.Done():
			f.Close()
			return nil, fmt.Errorf("mdstore: %s is held: %w", f.Name(), ctx.Err())
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// Folder is the machine's own folder.
func (s *Store) Folder() string { return s.folder }
