// Package gitbugiotest builds a real, throwaway git-bug repo cache for
// tests — via git-bug's own repository.CreateGoGitTestRepo test helper —
// instead of a hand-rolled fake. It's fast (single-digit milliseconds), so
// internal/gitbugio and internal/fields tests exercise the real
// cache.RepoCache API. Split into its own package (rather than a
// gitbugio_test.go helper) so other packages' tests can use it too.
package gitbugiotest

import (
	"testing"

	"github.com/git-bug/git-bug/repository"

	"github.com/guilbep/git-board/internal/gitbugio"
)

// New spins up a repo cache in a t.TempDir() with a user identity
// configured, ready to create/query bugs.
func New(t testing.TB) *gitbugio.Backend {
	t.Helper()

	repo := repository.CreateGoGitTestRepo(t, false)
	b, err := gitbugio.NewForRepo(repo)
	if err != nil {
		t.Fatalf("NewForRepo: %v", err)
	}
	t.Cleanup(func() { _ = b.Close() })

	id, err := b.TestCache().Identities().NewFromGitUser()
	if err != nil {
		t.Fatalf("NewFromGitUser: %v", err)
	}
	if err := b.TestCache().SetUserIdentity(id); err != nil {
		t.Fatalf("SetUserIdentity: %v", err)
	}

	return b
}

// NewIssue creates a bug with the given title and returns its id.
func NewIssue(t testing.TB, b *gitbugio.Backend, title string) string {
	t.Helper()
	bug, _, err := b.TestCache().Bugs().New(title, "")
	if err != nil {
		t.Fatalf("Bugs().New: %v", err)
	}
	return bug.Id().String()
}

// CloseIssue closes the given bug (by id prefix) so tests can exercise
// status:closed filtering.
func CloseIssue(t testing.TB, b *gitbugio.Backend, idPrefix string) {
	t.Helper()
	bug, err := b.TestCache().Bugs().ResolvePrefix(idPrefix)
	if err != nil {
		t.Fatalf("ResolvePrefix: %v", err)
	}
	if _, err := bug.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := bug.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}
