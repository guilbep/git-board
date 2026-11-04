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
// A version is how many packs the file holds. A write takes the entity's
// lock file (<id>.jsonl.lock), checks the count and appends: one writer at
// a time, on this machine and on a shared disk.
package mdstore

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/guilbep/git-board/core/datastore"
)

// Board is the folder of a dir that holds the packs.
const Board = ".board"

// gitattributes merges the pack files line by line.
const gitattributes = "*.jsonl merge=union\n"

// staleLock is how old a lock file is when its writer is taken for dead.
const staleLock = 30 * time.Second

var entityID = regexp.MustCompile(`^[0-9a-f]{40}$`)

// Store is a board kept in a folder.
type Store struct {
	dir    string // the folder of the Markdown files; the packs are in dir/.board
	folder string // the machine's own folder
}

var _ datastore.Datastore = (*Store)(nil)

// Open keeps the board in dir, making dir/.board and its .gitattributes
// when they aren't there. folder is the machine's own folder.
func Open(dir, folder string) (*Store, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	s := &Store{dir: abs, folder: folder}
	if err := os.MkdirAll(s.root(), 0o755); err != nil {
		return nil, fmt.Errorf("mdstore: %w", err)
	}
	ga := filepath.Join(s.root(), ".gitattributes")
	if _, err := os.Stat(ga); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(ga, []byte(gitattributes), 0o644); err != nil {
			return nil, fmt.Errorf("mdstore: %w", err)
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
		packs, err := read(s.path(kind, id))
		if err != nil {
			return nil, err
		}
		if len(packs) > 0 {
			out[id] = strconv.Itoa(len(packs))
		}
	}
	return out, nil
}

// Packs are an entity's packs, and how many.
func (s *Store) Packs(_ context.Context, kind, id string) ([][]byte, string, error) {
	packs, err := read(s.path(kind, id))
	if err != nil {
		return nil, "", err
	}
	if len(packs) == 0 {
		return nil, "", fmt.Errorf("%w: %s %s", datastore.ErrNotFound, kind, id)
	}
	return packs, strconv.Itoa(len(packs)), nil
}

// read is a pack file's lines; none for a file that isn't there. A merge
// may leave a line twice: it is one pack, as oplog counts it.
func read(path string) ([][]byte, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("mdstore: %w", err)
	}
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

// Add appends a pack if the file still holds prev packs ("" or 0 for one
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
	packs, err := read(path)
	if err != nil {
		return "", err
	}
	want := 0
	if prev != "" {
		if want, err = strconv.Atoi(prev); err != nil {
			return "", fmt.Errorf("%w: %s %s: version %q", datastore.ErrMoved, kind, id, prev)
		}
	}
	if len(packs) != want || (prev != "" && want == 0) {
		return "", fmt.Errorf("%w: %s %s", datastore.ErrMoved, kind, id)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return "", fmt.Errorf("mdstore: %w", err)
	}
	_, err = f.Write(append(bytes.Clone(pack), '\n'))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", fmt.Errorf("mdstore: %w", err)
	}
	return strconv.Itoa(len(packs) + 1), nil
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
			os.Remove(path)
			continue
		}
		if time.Since(start) > staleLock {
			return nil, fmt.Errorf("%w: %s is held", datastore.ErrMoved, path)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// Folder is the machine's own folder.
func (s *Store) Folder() string { return s.folder }
