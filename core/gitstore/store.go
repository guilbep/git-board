// Package gitstore reads and writes git objects and refs for the core.
package gitstore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/filesystem"
)

// Hash is a git object id.
type Hash = plumbing.Hash

// ZeroHash is the hash of no object. It stands for an absent ref.
var ZeroHash = plumbing.ZeroHash

// ErrRefMoved is returned when a ref is not where the write read it.
var ErrRefMoved = errors.New("gitstore: ref moved since it was read")

// Store is a git repository.
type Store struct {
	// ponytail: one lock serializes go-git access per Store; per-object locking if it ever shows in a profile
	mu     sync.Mutex
	repo   *git.Repository
	fs     *filesystem.Storage
	gitDir string
}

// TreeEntry is a blob in a tree, with mode 100644.
type TreeEntry struct {
	Name string
	Blob Hash
}

// Commit is a commit's tree and parents. Author, committer and message are
// the entity commit's fixed ones.
type Commit struct {
	Tree    Hash
	Parents []Hash
}

// Open opens the repository at path: a worktree, found upward from path, or
// a bare repository.
func Open(path string) (*Store, error) {
	repo, err := git.PlainOpenWithOptions(path, &git.PlainOpenOptions{DetectDotGit: true})
	if errors.Is(err, git.ErrRepositoryNotExists) {
		repo, err = git.PlainOpen(path) // a bare repository
	}
	if err != nil {
		return nil, fmt.Errorf("gitstore: open %s: %w", path, err)
	}
	fs, ok := repo.Storer.(*filesystem.Storage)
	if !ok {
		return nil, fmt.Errorf("gitstore: open %s: not a filesystem repository", path)
	}
	return &Store{repo: repo, fs: fs, gitDir: fs.Filesystem().Root()}, nil
}

func (s *Store) put(o plumbing.EncodedObject) (Hash, error) {
	return s.repo.Storer.SetEncodedObject(o)
}

// WriteBlob stores data as a blob.
func (s *Store) WriteBlob(data []byte) (Hash, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o := s.repo.Storer.NewEncodedObject()
	o.SetType(plumbing.BlobObject)
	w, err := o.Writer()
	if err != nil {
		return ZeroHash, err
	}
	if _, err := w.Write(data); err != nil {
		return ZeroHash, err
	}
	if err := w.Close(); err != nil {
		return ZeroHash, err
	}
	return s.put(o)
}

// ReadBlob returns a blob's bytes.
func (s *Store) ReadBlob(h Hash) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := retry(s, func() (*object.Blob, error) { return s.repo.BlobObject(h) })
	if err != nil {
		return nil, fmt.Errorf("gitstore: blob %s: %w", h, err)
	}
	r, err := b.Reader()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

// WriteTree stores a tree of blobs. Git wants the entries sorted by name.
func (s *Store) WriteTree(entries []TreeEntry) (Hash, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := object.Tree{}
	for _, e := range entries {
		t.Entries = append(t.Entries, object.TreeEntry{Name: e.Name, Mode: filemode.Regular, Hash: e.Blob})
	}
	// ponytail: sorted by plain name; right for blobs, not for subtrees.
	sort.Slice(t.Entries, func(i, j int) bool { return t.Entries[i].Name < t.Entries[j].Name })
	o := s.repo.Storer.NewEncodedObject()
	if err := t.Encode(o); err != nil {
		return ZeroHash, err
	}
	return s.put(o)
}

// ReadTree returns a tree's entries.
func (s *Store) ReadTree(h Hash) ([]TreeEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, err := retry(s, func() (*object.Tree, error) { return s.repo.TreeObject(h) })
	if err != nil {
		return nil, fmt.Errorf("gitstore: tree %s: %w", h, err)
	}
	var out []TreeEntry
	for _, e := range t.Entries {
		out = append(out, TreeEntry{Name: e.Name, Blob: e.Hash})
	}
	return out, nil
}

// WriteCommit stores an entity commit.
func (s *Store) WriteCommit(c Commit) (Hash, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sig := object.Signature{Name: "git-board", Email: "git-board@localhost", When: time.Now()}
	obj := object.Commit{Author: sig, Committer: sig, Message: "git-board", TreeHash: c.Tree, ParentHashes: c.Parents}
	o := s.repo.Storer.NewEncodedObject()
	if err := obj.Encode(o); err != nil {
		return ZeroHash, err
	}
	return s.put(o)
}

// ReadCommit returns a commit's tree and parents.
func (s *Store) ReadCommit(h Hash) (Commit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := retry(s, func() (*object.Commit, error) { return s.repo.CommitObject(h) })
	if err != nil {
		return Commit{}, fmt.Errorf("gitstore: commit %s: %w", h, err)
	}
	return Commit{Tree: c.TreeHash, Parents: c.ParentHashes}, nil
}

// retry reads again (s.mu held) after re-reading the pack index when an object is not
// found: go-git reads the index once, so a pack written later by git (a
// fetch, a gc) stays hidden until then.
func retry[T any](s *Store, read func() (T, error)) (T, error) {
	v, err := read()
	if errors.Is(err, plumbing.ErrObjectNotFound) {
		s.fs.Reindex()
		return read()
	}
	return v, err
}

// Ref returns where a ref points, or ZeroHash when it is absent.
func (s *Store) Ref(name string) (Hash, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, err := s.repo.Reference(plumbing.ReferenceName(name), false)
	if errors.Is(err, plumbing.ErrReferenceNotFound) {
		return ZeroHash, nil
	}
	if err != nil {
		return ZeroHash, err
	}
	return r.Hash(), nil
}

// Refs returns the refs whose names start with prefix.
func (s *Store) Refs(prefix string) (map[string]Hash, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	it, err := s.repo.References()
	if err != nil {
		return nil, err
	}
	defer it.Close()
	out := map[string]Hash{}
	err = it.ForEach(func(r *plumbing.Reference) error {
		if r.Type() == plumbing.HashReference && strings.HasPrefix(r.Name().String(), prefix) {
			out[r.Name().String()] = r.Hash()
		}
		return nil
	})
	return out, err
}

// UpdateRef moves a ref to next if it still points at prev. A prev of
// ZeroHash means the ref must not exist.
func (s *Store) UpdateRef(name string, next, prev Hash) error {
	err := s.git(context.Background(), "update-ref", name, next.String(), prev.String())
	if err == nil {
		return nil
	}
	msg := err.Error()
	for _, m := range []string{"but expected", "reference already exists", "File exists"} {
		if strings.Contains(msg, m) {
			return fmt.Errorf("%w: %s", ErrRefMoved, msg)
		}
	}
	return err
}

// git runs git on this repository and wraps its stderr in the error.
func (s *Store) git(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", append([]string{"--git-dir", s.gitDir}, args...)...)
	// UpdateRef matches git's refusals by their English text.
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gitstore: git %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return nil
}
