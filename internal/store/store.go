// Package store is git-board's seam over the core (core/...): issues,
// people, releases and the repository's own git history, as the board,
// the server, the TUI and the commands read and write them. It was
// gitbugio, for git-bug, which it wrapped until M5.
package store

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/osfs"
	git "github.com/go-git/go-git/v5"

	"github.com/guilbep/git-board/core/gitstore"
	"github.com/guilbep/git-board/core/index"
	"github.com/guilbep/git-board/core/issue"
	"github.com/guilbep/git-board/core/oplog"
	"github.com/guilbep/git-board/core/people"
	"github.com/guilbep/git-board/core/syncer"
	"github.com/guilbep/git-board/internal/config"
)

// Issue is an issue as git-board's fields and filters read it.
type Issue struct {
	ID     string   `json:"id"`
	Title  string   `json:"title"`
	Status string   `json:"status"` // "open" or "closed"
	Labels []string `json:"labels"`
	// Inherited are the labels it takes from its project ([roles]) for the
	// roles it doesn't set itself; internal/board sets them.
	Inherited []string `json:"inherited,omitempty"`
	GithubURL string   `json:"github_url,omitempty"` // "" if not linked to a GitHub issue
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
	// CreatedAt is when it was opened: it breaks a tie between issues one
	// sync stamped at once.
	CreatedAt time.Time `json:"created_at"`
	// AuthorLogins are who opened it, for author:: name, login, forge
	// logins.
	AuthorLogins []string `json:"-"`
	// CommenterLogins are the logins of who commented, the author
	// excepted, for involves:.
	CommenterLogins []string `json:"-"`
	// AuthorID and CommenterIDs are the same people by identity: you are
	// your identity, with or without a login.
	AuthorID     string   `json:"-"`
	CommenterIDs []string `json:"-"`
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
		if f.Number != "" {
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
	return n != "" && slices.ContainsFunc(is.Forges, func(f Forge) bool { return f.Number == n })
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

// Errors an id can answer.
var (
	// ErrNotFound: no issue, person, comment or release has that id.
	ErrNotFound = errors.New("not found")
	// ErrAmbiguous: several issues or persons have an id with that prefix.
	ErrAmbiguous = errors.New("ambiguous id prefix")
	// ErrNoMe: no one is chosen to write as yet.
	ErrNoMe = errors.New("choose who you write as first")
)

// Backend is an open repository: its store and its index.
type Backend struct {
	s   *gitstore.Store
	ix  *index.Index
	dir string // the git directory, for what only git does
	// flat are the kinds stored as plain labels, by label (SetFlatKinds)
	flat atomic.Pointer[map[string]bool]
	// cfg says which labels the forges' sync holds (SetHeld)
	cfg atomic.Pointer[config.Config]

	// ponytail: issues and persons loaded once per head, in memory; the
	// index's summaries when a cold listing of thousands gets slow
	mu      sync.Mutex
	issues  map[string]*issue.Issue
	persons map[string]*people.Person
	heads   map[string]gitstore.Hash // ref -> head of what's loaded

	// Tokens finds a forge account's token, Approve stores one: nil, the
	// environment and git's credential helper (tokens.go). Tests set
	// their own, so that none asks the machine's helper.
	Tokens  func(ctx context.Context, kind, baseURL, login string) (string, error)
	Approve func(ctx context.Context, baseURL, login, token string) error

	// the code's commits, read with go-git itself (codeRepo, code.go)
	codeOnce sync.Once
	code     *git.Repository
	codeErr  error
}

// FindGitDir is the absolute git directory of the repository at path, or
// of the one above it, as git finds it.
func FindGitDir(path string) (string, error) {
	out, err := exec.Command("git", "-C", path, "rev-parse", "--absolute-git-dir").Output()
	if err != nil {
		return "", fmt.Errorf("%s: %w", path, ErrNoRepo)
	}
	return strings.TrimSpace(string(out)), nil
}

// Open opens the repository at path, or the one above it, as git finds it.
func Open(path string) (*Backend, error) {
	dir, err := FindGitDir(path)
	if err != nil {
		return nil, fmt.Errorf("opening the repository: %w", err)
	}
	s, err := gitstore.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening the repository: %w", err)
	}
	ix, err := index.Open(context.Background(), s)
	if err != nil {
		return nil, fmt.Errorf("opening the index: %w", err)
	}
	b := &Backend{s: s, ix: ix, dir: dir, issues: map[string]*issue.Issue{}, persons: map[string]*people.Person{}, heads: map[string]gitstore.Hash{}}
	// the kind form and the held labels; a config that doesn't parse is the
	// command's to report
	if cfg, err := config.Load(config.PathIn(path)); err == nil {
		b.SetFlatKinds(cfg)
		b.SetHeld(cfg)
	}
	return b, nil
}

// OpenCwd is Open for the current working directory.
func OpenCwd() (*Backend, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return Open(cwd)
}

// Close closes the index.
func (b *Backend) Close() error { return b.ix.Close() }

// Store is the core's store of this repository.
func (b *Backend) Store() *gitstore.Store { return b.s }

// GitDir is the repository's git directory.
func (b *Backend) GitDir() string { return b.dir }

// Storage is git-board's own folder, <common dir>/board, for its caches:
// the inbox's files, the assignable people, the media.
type Storage = billy.Filesystem

// Storage is git-board's folder for its caches, in the git common dir.
func (b *Backend) Storage() Storage { return osfs.New(filepath.Join(b.s.CommonDir(), "board")) }

var ctx = context.Background()

// numberName is an issue named by a number: its project number, CO2-3075,
// or a forge's, #2518.
var numberName = regexp.MustCompile(`^(#[0-9]+|[A-Za-z][A-Za-z0-9]*-[0-9]+)$`)

// resolve is the whole id of an issue or a person ("issue", "person") by
// a prefix of it; an issue by its project number or a forge's too, as the
// palette's jump takes them.
func (b *Backend) resolve(kind, prefix string) (string, error) {
	if kind == "issue" && numberName.MatchString(prefix) {
		return b.numbered(prefix)
	}
	id, err := b.ix.Resolve(ctx, kind, prefix)
	switch {
	case errors.Is(err, index.ErrNoMatch):
		return "", fmt.Errorf("%w: %s %s", ErrNotFound, kind, prefix)
	case errors.Is(err, index.ErrAmbiguous):
		return "", fmt.Errorf("%w: %s %s", ErrAmbiguous, kind, prefix)
	case err != nil:
		return "", fmt.Errorf("%w: %v", ErrNotFound, err) // a bad prefix names nothing
	}
	return id, nil
}

// numbered is the issue a number names: #n a forge's (HasForgeNumber),
// CO2-3075 its project number.
// ponytail: reads every issue, as a list does; an index of the numbers
// when an address by number costs
func (b *Backend) numbered(name string) (string, error) {
	issues, err := b.ListIssues()
	if err != nil {
		return "", err
	}
	var ids []string
	for _, is := range issues {
		if forge, ok := strings.CutPrefix(name, "#"); ok && is.HasForgeNumber(forge) || strings.EqualFold(is.Number, name) {
			ids = append(ids, is.ID)
		}
	}
	switch len(ids) {
	case 0:
		return "", fmt.Errorf("%w: issue %s", ErrNotFound, name)
	case 1:
		return ids[0], nil
	}
	return "", fmt.Errorf("%w: issue %s, on %d issues", ErrAmbiguous, name, len(ids))
}

// me is the person id git-board writes as.
func (b *Backend) me() (string, error) {
	id, err := people.Me(ctx, b.s)
	if err == nil && id == "" {
		err = ErrNoMe
	}
	return id, err
}

// loadIssue is an issue and its log, loaded again only when its head moved.
func (b *Backend) loadIssue(id string) (*issue.Issue, *oplog.Log, error) {
	l, err := oplog.Load(b.s, "issue", id)
	if errors.Is(err, oplog.ErrNotFound) {
		return nil, nil, fmt.Errorf("%w: issue %s", ErrNotFound, id)
	}
	if err != nil {
		return nil, nil, err
	}
	is, err := issue.Apply(l)
	if err != nil {
		return nil, nil, err
	}
	b.mu.Lock()
	b.issues[id], b.heads[oplog.RefName("issue", id)] = is, l.Head
	b.mu.Unlock()
	return is, l, nil
}

// all is every entity of a kind ("issue", "person") at its head, through
// the cache: load reads one again. One that fails to load is left out and
// logged, as the index keeps a broken entity out.
func all[T any](b *Backend, kind string, cache map[string]T, load func(id string) (T, error)) (map[string]T, error) {
	prefix := oplog.RefName(kind, "")
	refs, err := b.s.Refs(prefix)
	if err != nil {
		return nil, err
	}
	out := map[string]T{}
	for name, head := range refs {
		id := strings.TrimPrefix(name, prefix)
		if !oplog.IsID(id) {
			continue
		}
		b.mu.Lock()
		v, ok := cache[id]
		ok = ok && b.heads[name] == head
		b.mu.Unlock()
		if !ok {
			if v, err = load(id); err != nil {
				log.Printf("%s %s: %v", kind, id, err)
				continue
			}
		}
		out[id] = v
	}
	return out, nil
}

func (b *Backend) allIssues() (map[string]*issue.Issue, error) {
	return all(b, "issue", b.issues, func(id string) (*issue.Issue, error) {
		is, _, err := b.loadIssue(id)
		return is, err
	})
}

func (b *Backend) allPersons() (map[string]*people.Person, error) {
	return all(b, "person", b.persons, func(id string) (*people.Person, error) {
		l, err := oplog.Load(b.s, "person", id)
		if err != nil {
			return nil, err
		}
		p, err := people.Apply(l)
		if err != nil {
			return nil, err
		}
		b.mu.Lock()
		b.persons[id], b.heads[oplog.RefName("person", id)] = p, l.Head
		b.mu.Unlock()
		return p, nil
	})
}

// view is what turning core issues into Issues reads once: the counter,
// the forges' links and the persons.
type view struct {
	c       *Counter
	records map[string][]syncer.Record
	persons map[string]*people.Person
}

// ponytail: folds every sync entity on each call, cheap while they are
// small; cache by the sync refs' heads once real projects fill them (M6).
func (b *Backend) view() (view, error) {
	records, err := syncer.Records(b.s)
	if err != nil {
		return view{}, err
	}
	persons, err := b.allPersons()
	if err != nil {
		return view{}, err
	}
	return view{c: b.counter(), records: records, persons: persons}, nil
}

// issueFrom is a core issue as git-board reads it: its labels shown, its
// forges, who opened it and who commented, and its project number when
// the project numbers its issues.
func (b *Backend) issueFrom(is *issue.Issue, v view) Issue {
	forges := forgesOf(v.records[is.ID])
	out := Issue{ID: is.ID, Title: is.Title, Status: string(is.Status), Labels: b.shown(slices.Clone(is.Labels)), Forges: forges,
		UpdatedAt: time.Unix(is.Updated, 0), CreatedAt: time.Unix(is.Created, 0), AuthorID: is.Author}
	for _, f := range forges {
		if f.Name == "GitHub" && out.GithubURL == "" {
			out.GithubURL = f.URL
		}
	}
	out.Shown = out.ShownTitle()
	if v.c != nil {
		out.Counted = true
		if n := v.c.NumberOf(is.ID, forges); n > 0 {
			out.Number = v.c.Format(n)
		}
	}
	if a := v.persons[is.Author]; a != nil {
		out.AuthorLogins = append([]string{a.Name}, logins(a)...)
	}
	for _, c := range is.Comments {
		if c.Author == is.Author || slices.Contains(out.CommenterIDs, c.Author) {
			continue
		}
		out.CommenterIDs = append(out.CommenterIDs, c.Author)
		if p := v.persons[c.Author]; p != nil {
			out.CommenterLogins = append(out.CommenterLogins, logins(p)...)
		}
	}
	return out
}

// logins are a person's logins on forges, each once.
func logins(p *people.Person) []string {
	var res []string
	for _, a := range p.Accounts {
		if !slices.Contains(res, a.Login) {
			res = append(res, a.Login)
		}
	}
	return res
}

// ListIssues returns every issue, oldest first. Filtering is done
// client-side via internal/filter.
func (b *Backend) ListIssues() ([]Issue, error) {
	issues, err := b.allIssues()
	if err != nil {
		return nil, fmt.Errorf("listing the issues: %w", err)
	}
	v, err := b.view()
	if err != nil {
		return nil, err
	}
	res := make([]Issue, 0, len(issues))
	for _, is := range issues {
		res = append(res, b.issueFrom(is, v))
	}
	slices.SortFunc(res, func(x, y Issue) int { return cmp.Or(x.CreatedAt.Compare(y.CreatedAt), cmp.Compare(x.ID, y.ID)) })
	return res, nil
}

// Labels is every label in use, on any issue.
func (b *Backend) Labels() []string {
	labels, err := b.ix.Labels(ctx)
	if err != nil {
		log.Printf("labels in use: %v", err)
	}
	return b.shown(labels)
}

// GetIssue resolves an issue by id prefix.
func (b *Backend) GetIssue(idPrefix string) (Issue, error) {
	is, _, err := b.issue(idPrefix)
	if err != nil {
		return Issue{}, err
	}
	v, err := b.view()
	if err != nil {
		return Issue{}, err
	}
	return b.issueFrom(is, v), nil
}

// issue is an issue by id prefix, and its log.
func (b *Backend) issue(idPrefix string) (*issue.Issue, *oplog.Log, error) {
	id, err := b.resolve("issue", idPrefix)
	if err != nil {
		return nil, nil, err
	}
	return b.loadIssue(id)
}

// GetLabels returns the current labels for an issue, by id prefix.
func (b *Backend) GetLabels(idPrefix string) ([]string, error) {
	is, _, err := b.issue(idPrefix)
	if err != nil {
		return nil, err
	}
	return b.shown(slices.Clone(is.Labels)), nil
}

// do writes ops, made by whoever git-board writes as, to an issue by id
// prefix: ops gets the author and the issue as it is.
func (b *Backend) do(idPrefix string, ops func(me string, is *issue.Issue, l *oplog.Log) ([]oplog.Op, error)) (*issue.Issue, error) {
	me, err := b.me()
	if err != nil {
		return nil, err
	}
	is, l, err := b.issue(idPrefix)
	if err != nil {
		return nil, err
	}
	list, err := ops(me, is, l)
	if err != nil || len(list) == 0 {
		return is, err
	}
	return issue.Do(ctx, b.s, is.ID, list...)
}

// ApplyLabelChange adds and removes labels on an issue in one operation,
// and returns the resulting labels. A flat kind is written as stored, and
// read back as kind:bug.
func (b *Backend) ApplyLabelChange(idPrefix string, added, removed []string) ([]string, error) {
	is, err := b.do(idPrefix, func(me string, is *issue.Issue, _ *oplog.Log) ([]oplog.Op, error) {
		added, removed := b.stored(added, removed, is.Labels)
		added = slices.DeleteFunc(slices.Clone(added), func(l string) bool { return slices.Contains(is.Labels, l) })
		removed = slices.DeleteFunc(slices.Clone(removed), func(l string) bool { return !slices.Contains(is.Labels, l) })
		if len(added) == 0 && len(removed) == 0 {
			return nil, nil
		}
		return []oplog.Op{issue.ChangeLabels(me, added, removed)}, nil
	})
	if err != nil {
		return nil, fmt.Errorf("changing labels on %s: %w", idPrefix, err)
	}
	return b.shown(slices.Clone(is.Labels)), nil
}

// ProjectBases returns what a GitHub Project and git-board last agreed on
// for an issue, by field: its metadata under prefix (one per Project).
func (b *Backend) ProjectBases(idPrefix, prefix string) (map[string]string, error) {
	is, _, err := b.issue(idPrefix)
	if err != nil {
		return nil, err
	}
	res := map[string]string{}
	for k, v := range is.Meta {
		if field, ok := strings.CutPrefix(k, prefix); ok {
			res[field] = v
		}
	}
	return res, nil
}

// SetProjectBases records what a GitHub Project and git-board now agree
// on, in the issue's metadata, so it travels with the issue.
func (b *Backend) SetProjectBases(idPrefix, prefix string, values map[string]string) error {
	meta := make(map[string]string, len(values))
	for field, v := range values {
		meta[prefix+field] = v
	}
	_, err := b.do(idPrefix, func(me string, is *issue.Issue, _ *oplog.Log) ([]oplog.Op, error) {
		if len(meta) == 0 {
			return nil, nil
		}
		return []oplog.Op{issue.SetMeta(me, is.BodyOp, meta)}, nil
	})
	if err != nil {
		return fmt.Errorf("recording Project values on %s: %w", idPrefix, err)
	}
	return nil
}

// CloseBug closes an issue, which `close` layers a status:* label swap
// on: named so as not to be taken for Close. Closing a closed issue
// writes nothing.
func (b *Backend) CloseBug(idPrefix string) error { return b.SetStatus(idPrefix, "closed") }

// SearchText returns the issues whose title, description or a comment
// hold text (the index's search), by id.
func (b *Backend) SearchText(text string) (map[string]bool, error) {
	found, err := b.ix.Issues(ctx, index.Query{Text: text})
	if err != nil {
		return nil, err
	}
	res := make(map[string]bool, len(found))
	for _, s := range found {
		res[s.ID] = true
	}
	return res, nil
}

// Pull fetches remote's entities and merges them into this clone's.
func (b *Backend) Pull(remote string) error {
	if err := b.Fetch(remote); err != nil {
		return err
	}
	if err := b.ResetGuard(remote); err != nil {
		return err
	}
	_, err := oplog.MergeFetched(ctx, b.s, remote)
	return err
}

// Push pushes the entities to remote, then fetches them back, so the
// remote's copies here say what it has. It answers no output.
// ponytail: a fetch after each push; the remote's refs set from ours when
// that fetch costs.
func (b *Backend) Push(remote string) (string, error) {
	if err := b.checkRemote(remote); err != nil {
		return "", err
	}
	if err := b.guarded(remote); err != nil {
		return "", err
	}
	// a push URL that refuses (a mirror) fails it, and the others took it
	err := errors.Join(b.s.Push(ctx, remote), b.pushMark(remote))
	return "", errors.Join(err, b.s.Fetch(ctx, remote))
}

// Fetch brings a remote's entities in as its copies here
// (refs/board-remotes/<remote>/): nothing local changes.
func (b *Backend) Fetch(remote string) error {
	if err := b.checkRemote(remote); err != nil {
		return err
	}
	if err := b.s.Fetch(ctx, remote); err != nil {
		return err
	}
	if err := b.fetchResetMark(remote); err != nil {
		return err
	}
	return b.fetchMark(remote)
}

var remoteName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)

// validRemote refuses a name git would read as an option, or that would
// leave refs/board-remotes/.
func validRemote(remote string) error {
	if !remoteName.MatchString(remote) || strings.Contains(remote, "..") {
		return fmt.Errorf("not a remote: %q", remote)
	}
	return nil
}

// checkRemote refuses a remote this repository doesn't have.
func (b *Backend) checkRemote(remote string) error {
	has, err := b.hasRemote(remote)
	if err == nil && !has {
		err = fmt.Errorf("no remote %q", remote)
	}
	return err
}
