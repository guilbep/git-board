// Package gitbugio is a thin wrapper around git-bug's cache.RepoCache API —
// the seam that lets internal/fields and internal/filter (and any future
// TUI/web frontend) talk to git-bug issues without knowing about git-bug's
// package layout. Field writes go through this in-process library call, not
// the git-bug CLI binary.
package gitbugio

import (
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/git-bug/git-bug/cache"
	bugent "github.com/git-bug/git-bug/entities/bug"
	"github.com/git-bug/git-bug/entities/common"
	"github.com/git-bug/git-bug/entity"
	"github.com/git-bug/git-bug/entity/dag"
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
	ID     string   `json:"id"`
	Title  string   `json:"title"`
	Status string   `json:"status"` // "open" or "closed"
	Labels []string `json:"labels"`
	// Inherited are the labels it takes from its project ([roles]) for the
	// roles it doesn't set itself; internal/board sets them.
	Inherited []string `json:"inherited,omitempty"`
	GithubURL string   `json:"github_url,omitempty"` // "" if not bridged in from GitHub
	Forges    []Forge  `json:"forges,omitempty"`     // where it is on forges
	// Shown is the title as the board shows it (ShownTitle).
	Shown string `json:"shown_title"`
	// Number is its project number, CO2-3075: "" until the counter hands it
	// one, and while the project doesn't number its issues (numbers.go).
	Number string `json:"number,omitempty"`
	// Counted tells the project numbers its issues: a bare number (3075,
	// fix/3075-…) then means a project number; #3075 is still a forge's.
	Counted bool `json:"counted,omitempty"`
	// UpdatedAt is its last change, for "changed since your last visit".
	UpdatedAt time.Time `json:"updated_at"`
	// AuthorLogins are who opened it, for author:: name, login, forge
	// logins.
	AuthorLogins []string `json:"-"`
	// CommenterLogins are the logins of who commented, the author
	// excepted, for involves:.
	CommenterLogins []string `json:"-"`
}

// BareNumber is the number a bare mention names it by (3075, fix/3075-…):
// its project number once the project numbers its issues, else its first
// forge number; "" for none. With HasNumber and HasForgeNumber, the one
// place a number is told to name an issue, for search, branch names and
// titles.
func (is Issue) BareNumber() string {
	if is.Counted {
		return is.Number[strings.LastIndexByte(is.Number, '-')+1:]
	}
	for _, f := range is.Forges {
		if f.Number != "" && f.Gone == "" {
			return f.Number
		}
	}
	return ""
}

// HasNumber tells whether a bare number names the issue: its project
// number once the project numbers its issues, else any of its forge
// numbers.
func (is Issue) HasNumber(n string) bool {
	if is.Counted {
		return n != "" && is.BareNumber() == n
	}
	return is.HasForgeNumber(n)
}

// HasForgeNumber tells whether #n, a forge's own way to name an issue,
// names it.
func (is Issue) HasForgeNumber(n string) bool {
	return n != "" && slices.ContainsFunc(is.Forges, func(f Forge) bool { return f.Number == n && f.Gone == "" })
}

// AllLabels are its own labels and those it inherits: what filters,
// groups and facets see. Edits change its own labels only.
func (is Issue) AllLabels() []string { return append(slices.Clip(is.Labels), is.Inherited...) }

// Headline is its shown title after its project number, as lists show it:
// "CO2-3075 Restarting Postgres empties the bouncer pool"; its title
// before it has one.
func (is Issue) Headline() string {
	if is.Number == "" {
		return is.Title
	}
	return is.Number + " " + is.ShownTitle()
}

var ownNumber = regexp.MustCompile(`\s*\(#(\d+)\)\s*$`) // (#2518)

// ShownTitle is the title without a trailing (#2518) that is one of the
// issue's own forge numbers: the forge links show it already. The title
// itself keeps it, and editing starts from the title.
func (is Issue) ShownTitle() string {
	if m := ownNumber.FindStringSubmatchIndex(is.Title); m != nil && is.HasForgeNumber(is.Title[m[2]:m[3]]) {
		return is.Title[:m[0]]
	}
	return is.Title
}

// Backend is an open connection to a git-bug repo cache.
type Backend struct {
	repo repository.Repo
	c    *cache.RepoCache
	dir  string // the git directory, for what only git does; "" from NewForRepo
}

// Open opens the git-bug repo cache rooted at path (or an ancestor of it,
// same discovery as `git-bug`/`git` itself).
func Open(path string) (*Backend, error) {
	gitDir, err := repository.FindGitDir(path)
	if err != nil {
		return nil, fmt.Errorf("opening git-bug repo: %w", err)
	}
	repo, err := repository.OpenDefault(gitDir, gitBugNamespace)
	if err != nil {
		return nil, fmt.Errorf("opening git-bug repo: %w", err)
	}
	c, err := cache.NewRepoCacheNoEvents(repo, nil)
	if err != nil {
		return nil, fmt.Errorf("building git-bug cache: %w", err)
	}
	return &Backend{repo: repo, c: c, dir: gitDir}, nil
}

// OpenCwd is Open for the current working directory.
func OpenCwd() (*Backend, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return Open(cwd)
}

// NewForRepo wraps an already-open repo (e.g. repotest.NewGoGitRepo
// in tests) in a Backend, skipping the on-disk lookup Open does.
func NewForRepo(repo repository.Repo) (*Backend, error) {
	c, err := cache.NewRepoCacheNoEvents(repo, nil)
	if err != nil {
		return nil, fmt.Errorf("building git-bug cache: %w", err)
	}
	return &Backend{repo: repo, c: c}, nil
}

// Close releases the underlying repo cache.
func (b *Backend) Close() error {
	return b.c.Close()
}

// Pull fetches remote and merges its refs/bugs/* (and identities) into the
// local cache — equivalent to `git bug pull <remote>`, but via the library
// (Fetch, then MergeAll) rather than shelling out, consistent with how
// git-board reads/writes labels in-process everywhere else.
func (b *Backend) Pull(remote string) error {
	if _, err := b.c.Fetch(remote); err != nil {
		return err
	}
	// to the end, as git bug pull does: cache.RepoCache.Pull stops at the
	// first error, leaving the merging goroutine blocked
	var err error
	for m := range b.c.MergeAll(remote) {
		if err == nil && m.Err != nil {
			err = m.Err
		} else if err == nil && m.Status == entity.MergeStatusInvalid {
			err = fmt.Errorf("merge failure: %s", m.Reason)
		}
	}
	return err
}

// Push pushes local refs/bugs/* (and identities) to remote — equivalent
// to `git bug push <remote>`. Returns the underlying git push's output.
func (b *Backend) Push(remote string) (string, error) {
	return b.c.Push(remote)
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
	c := b.counter()
	issues := make([]Issue, len(ids))
	for i, id := range ids {
		excerpt, err := b.c.Bugs().ResolveExcerpt(id)
		if err != nil {
			return nil, fmt.Errorf("resolving %s: %w", id, err)
		}
		issues[i] = b.issueFrom(id.String(), excerpt, c)
	}
	return issues, nil
}

// Labels is every label in use, on any issue.
func (b *Backend) Labels() []string {
	var res []string
	for _, l := range b.c.Bugs().ValidLabels() {
		res = append(res, l.String())
	}
	return res
}

// issueFrom is excerptToIssue with who opened it, for author:, and its
// project number when c, the counter, numbers the project's issues.
func (b *Backend) issueFrom(id string, e *cache.BugExcerpt, c *Counter) Issue {
	issue := excerptToIssue(id, e)
	if c != nil {
		issue.Counted = true
		if n := c.NumberOf(id, issue.Forges); n > 0 {
			issue.Number = c.Format(n)
		}
	}
	if a, err := b.c.Identities().ResolveExcerpt(e.AuthorId); err == nil {
		for _, l := range []string{a.Name, a.Login, a.ImmutableMetadata["github-login"], a.ImmutableMetadata["gitlab-login"], a.ImmutableMetadata["gitea-login"]} {
			if l != "" {
				issue.AuthorLogins = append(issue.AuthorLogins, l)
			}
		}
	}
	for _, p := range e.Participants {
		if p == e.AuthorId {
			continue
		}
		if c, err := b.c.Identities().ResolveExcerpt(p); err == nil {
			for _, l := range []string{c.Login, c.ImmutableMetadata["github-login"], c.ImmutableMetadata["gitlab-login"], c.ImmutableMetadata["gitea-login"]} {
				if l != "" {
					issue.CommenterLogins = append(issue.CommenterLogins, l)
				}
			}
		}
	}
	return issue
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
	forges, _ := forgesOf(e.CreateMetadata)
	is := Issue{ID: id, Title: e.Title, Status: status, Labels: labels, GithubURL: e.CreateMetadata[githubURLMetaKey], Forges: forges,
		UpdatedAt: time.Unix(e.EditUnixTime, 0)}
	is.Shown = is.ShownTitle()
	return is
}

// GetIssue resolves a bug by id prefix.
func (b *Backend) GetIssue(idPrefix string) (Issue, error) {
	excerpt, err := b.c.Bugs().ResolveExcerptPrefix(idPrefix)
	if err != nil {
		return Issue{}, fmt.Errorf("resolving %s: %w", idPrefix, err)
	}
	c := b.counter()
	return b.issueFrom(excerpt.Id().String(), excerpt, c), nil
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

// ProjectBases returns what a GitHub Project and git-board last agreed on for
// a bug, by field: the records under prefix (one per Project), the last
// record of a field winning.
func (b *Backend) ProjectBases(idPrefix, prefix string) (map[string]string, error) {
	bug, err := b.c.Bugs().ResolvePrefix(idPrefix)
	if err != nil {
		return nil, fmt.Errorf("resolving %s: %w", idPrefix, err)
	}
	res := map[string]string{}
	for _, op := range bug.Snapshot().Operations {
		set, ok := op.(*dag.SetMetadataOperation[*bugent.Snapshot])
		if !ok {
			continue
		}
		for k, v := range set.NewMetadata {
			if field, ok := strings.CutPrefix(k, prefix); ok {
				res[field] = v
			}
		}
	}
	return res, nil
}

// SetProjectBases records what a GitHub Project and git-board now agree on,
// in the bug's history, so it travels with the bug.
func (b *Backend) SetProjectBases(idPrefix, prefix string, values map[string]string) error {
	bug, err := b.c.Bugs().ResolvePrefix(idPrefix)
	if err != nil {
		return fmt.Errorf("resolving %s: %w", idPrefix, err)
	}
	meta := make(map[string]string, len(values))
	for field, v := range values {
		meta[prefix+field] = v
	}
	if _, err := bug.SetMetadata(bug.Snapshot().Operations[0].Id(), meta); err != nil {
		return fmt.Errorf("recording Project values on %s: %w", idPrefix, err)
	}
	return bug.Commit()
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

// SearchText returns the issues whose title, description or a comment hold
// text (git-bug's full-text index), by id.
func (b *Backend) SearchText(text string) (map[string]bool, error) {
	q := query.NewQuery()
	q.Search = query.Search{text}
	ids, err := b.c.Bugs().Query(q)
	if err != nil {
		return nil, err
	}
	res := make(map[string]bool, len(ids))
	for _, id := range ids {
		res[id.String()] = true
	}
	return res, nil
}
