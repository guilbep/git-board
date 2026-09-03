// Package gitbugio is a thin wrapper around git-bug's cache.RepoCache API —
// the seam that lets internal/fields and internal/filter (and any future
// TUI/web frontend) talk to git-bug issues without knowing about git-bug's
// package layout. Field writes go through this in-process library call, not
// the git-bug CLI binary.
package gitbugio

import (
	"fmt"
	"os"

	"github.com/git-bug/git-bug/cache"
	"github.com/git-bug/git-bug/entities/common"
	"github.com/git-bug/git-bug/query"
	"github.com/git-bug/git-bug/repository"
)

// gitBugNamespace matches git-bug's own CLI (commands/execenv/env.go).
const gitBugNamespace = "git-bug"

// githubURLMetaKey is the Create-operation metadata key git-bug's own
// GitHub bridge stamps on every bug it imports (bridge/github/github.go's
// unexported metaKeyGithubUrl = "github-url"; bridge/github/import.go sets
// it via Bugs().NewRaw's metadata map at creation time). A bug that was
// bridged in from GitHub carries the issue's GitHub URL under this key on
// its first operation; a bug created locally in git-bug has no such key.
// This is how internal/ghbridge matches a GitHub Project item back to the
// git-bug issue it corresponds to, without parsing titles.
const githubURLMetaKey = "github-url"

// Issue is the subset of a git-bug bug that git-board fields/filters need.
type Issue struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Status    string   `json:"status"` // "open" or "closed"
	Labels    []string `json:"labels"`
	GithubURL string   `json:"github_url,omitempty"` // "" if not bridged in from GitHub
}

// Backend is an open connection to a git-bug repo cache.
type Backend struct {
	repo repository.ClockedRepo
	c    *cache.RepoCache
}

// Open opens the git-bug repo cache rooted at path (or an ancestor of it,
// same discovery as `git-bug`/`git` itself).
func Open(path string) (*Backend, error) {
	repo, err := repository.OpenGoGitRepo(path, gitBugNamespace, nil)
	if err != nil {
		return nil, fmt.Errorf("opening git-bug repo: %w", err)
	}
	c, err := cache.NewRepoCacheNoEvents(repo)
	if err != nil {
		return nil, fmt.Errorf("building git-bug cache: %w", err)
	}
	return &Backend{repo: repo, c: c}, nil
}

// OpenCwd is Open for the current working directory.
func OpenCwd() (*Backend, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return Open(cwd)
}

// NewForRepo wraps an already-open repo (e.g. repository.CreateGoGitTestRepo
// in tests) in a Backend, skipping the on-disk lookup Open does.
func NewForRepo(repo repository.ClockedRepo) (*Backend, error) {
	c, err := cache.NewRepoCacheNoEvents(repo)
	if err != nil {
		return nil, fmt.Errorf("building git-bug cache: %w", err)
	}
	return &Backend{repo: repo, c: c}, nil
}

// Close releases the underlying repo cache.
func (b *Backend) Close() error {
	return b.c.Close()
}

// TestCache exposes the underlying *cache.RepoCache for test fixture setup
// (creating bugs/identities) in the gitbugiotest package. Production code
// should go through the Backend methods above instead.
func (b *Backend) TestCache() *cache.RepoCache {
	return b.c
}

// ListIssues returns every bug in the repo. Filtering is done client-side
// via internal/filter, so no query is needed here.
func (b *Backend) ListIssues() ([]Issue, error) {
	ids, err := b.c.Bugs().Query(query.NewQuery())
	if err != nil {
		return nil, fmt.Errorf("listing bugs: %w", err)
	}
	issues := make([]Issue, len(ids))
	for i, id := range ids {
		excerpt, err := b.c.Bugs().ResolveExcerpt(id)
		if err != nil {
			return nil, fmt.Errorf("resolving %s: %w", id, err)
		}
		issues[i] = excerptToIssue(id.String(), excerpt)
	}
	return issues, nil
}

func excerptToIssue(id string, e *cache.BugExcerpt) Issue {
	labels := make([]string, len(e.Labels))
	for i, l := range e.Labels {
		labels[i] = l.String()
	}
	status := "open"
	if e.Status == common.ClosedStatus {
		status = "closed"
	}
	return Issue{ID: id, Title: e.Title, Status: status, Labels: labels, GithubURL: e.CreateMetadata[githubURLMetaKey]}
}

// GetIssue resolves a bug by id prefix.
func (b *Backend) GetIssue(idPrefix string) (Issue, error) {
	excerpt, err := b.c.Bugs().ResolveExcerptPrefix(idPrefix)
	if err != nil {
		return Issue{}, fmt.Errorf("resolving %s: %w", idPrefix, err)
	}
	return excerptToIssue(excerpt.Id().String(), excerpt), nil
}

// GetLabels returns the current labels for a bug, by id prefix.
func (b *Backend) GetLabels(idPrefix string) ([]string, error) {
	issue, err := b.GetIssue(idPrefix)
	if err != nil {
		return nil, err
	}
	return issue.Labels, nil
}

// ApplyLabelChange adds/removes labels on a bug in one operation and commits
// it. Mirrors the exact call git-bug's own `bug label new`/`bug label rm`
// commands make (bug.ChangeLabels then Commit). Returns the resulting label
// set. Callers must pre-filter added/removed against the current labels —
// git-bug returns an error if a call would add an already-set label or
// remove a label that isn't set.
func (b *Backend) ApplyLabelChange(idPrefix string, added, removed []string) ([]string, error) {
	bug, err := b.c.Bugs().ResolvePrefix(idPrefix)
	if err != nil {
		return nil, fmt.Errorf("resolving %s: %w", idPrefix, err)
	}

	if len(added) > 0 || len(removed) > 0 {
		if _, _, err := bug.ChangeLabels(added, removed); err != nil {
			return nil, fmt.Errorf("changing labels on %s: %w", idPrefix, err)
		}
		if err := bug.Commit(); err != nil {
			return nil, fmt.Errorf("committing label change on %s: %w", idPrefix, err)
		}
	}

	labels := make([]string, len(bug.Snapshot().Labels))
	for i, l := range bug.Snapshot().Labels {
		labels[i] = l.String()
	}
	return labels, nil
}

// CloseBug marks a bug closed and commits it — the underlying operation
// `close` layers a status:* label swap on top of. Named CloseBug, not
// Close, so it can't be confused with (*Backend).Close (the repo cache).
// git-bug's own bug.Close() is a no-op status-wise if already closed, so
// this is safe to call more than once.
func (b *Backend) CloseBug(idPrefix string) error {
	bug, err := b.c.Bugs().ResolvePrefix(idPrefix)
	if err != nil {
		return fmt.Errorf("resolving %s: %w", idPrefix, err)
	}
	if _, err := bug.Close(); err != nil {
		return fmt.Errorf("closing %s: %w", idPrefix, err)
	}
	return bug.Commit()
}
