package ghbridge

import (
	"errors"
	"fmt"
	"strings"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/store"
)

// DefaultItemLimit is how many Project items a sync reads by default.
const DefaultItemLimit = 2000

// ErrNoProject is returned when .git-board.toml names no GitHub Project.
var ErrNoProject = errors.New("no [github-project] in .git-board.toml: git board project configure --owner <org> --project <n>")

// ErrNoWrite is returned when keeping git-board's value would write to a
// Project the config doesn't allow writing to.
var ErrNoWrite = errors.New("keeping git-board's value writes to the Project: set write = true under [github-project] in .git-board.toml")

// ErrNotConflict is returned when resolving a field that isn't a conflict.
var ErrNotConflict = errors.New("isn't a conflict now")

// Project is how a sync reads and writes a GitHub Project: Gh, or a fake in
// tests.
type Project struct {
	Items  func(owner string, number, limit int) ([]Item, error)
	Meta   func(owner string, number int) (ProjectMeta, error)
	Writer ProjectWriter
	// Assignable lists who can be assigned on a repository; nil skips the
	// check.
	Assignable func(repo string) ([]string, error)
}

// Gh is the Project through the gh CLI.
var Gh = Project{Items: FetchItems, Meta: FetchProjectMeta, Writer: GhWriter{}, Assignable: FetchAssignable}

// Repo is the git-bug side of a sync.
type Repo interface {
	ListIssues() ([]store.Issue, error)
	ProjectBases(idPrefix, prefix string) (map[string]string, error)
	LabelWriter
	BaseWriter
}

var _ Repo = (*store.Backend)(nil)

// State is what a sync decides from, read from both sides.
type State struct {
	Issues []store.Issue
	Items  []Item
	Meta   ProjectMeta
	Bases  map[string]map[string]string
	// Truncated is Remote's: the read filled its limit.
	Truncated bool
}

// LoadState reads the issues, the Project's items and fields, and the
// recorded bases.
func LoadState(repo Repo, p Project, cfg *config.Config, limit int) (State, error) {
	r, err := FetchRemote(p, cfg, limit)
	if err != nil {
		return State{}, err
	}
	return r.State(repo, cfg)
}

// Remote is the Project's side of a State: its items and fields, read
// over the network and nothing from the repository, so it can be read
// off the goroutine that holds the repository.
type Remote struct {
	Items []Item
	Meta  ProjectMeta
	// Truncated is whether the read filled its limit: gh stops there, so
	// the Project may have more items, which this sync leaves out.
	Truncated bool
}

// FetchRemote reads the Project's items and fields.
func FetchRemote(p Project, cfg *config.Config, limit int) (Remote, error) {
	gp := cfg.GithubProject
	if gp.Owner == "" || gp.Number == 0 {
		return Remote{}, ErrNoProject
	}
	var r Remote
	var err error
	if r.Items, err = p.Items(gp.Owner, gp.Number, limit); err != nil {
		return Remote{}, err
	}
	r.Truncated = limit > 0 && len(r.Items) >= limit
	if r.Meta, err = p.Meta(gp.Owner, gp.Number); err != nil {
		return Remote{}, err
	}
	// Only a sync allowed to write assigns anyone.
	if p.Assignable != nil && gp.Write {
		r.Meta.Assignable = map[string][]string{}
		for _, it := range r.Items {
			repo := repoOf(it.Content.URL)
			if _, done := r.Meta.Assignable[repo]; done || it.Content.Type != "Issue" {
				continue
			}
			if r.Meta.Assignable[repo], err = p.Assignable(repo); err != nil {
				return Remote{}, err
			}
		}
	}
	return r, nil
}

// State adds the repository's side to r: the issues as they are now, and
// the values both sides last agreed on.
func (r Remote) State(repo Repo, cfg *config.Config) (State, error) {
	s := State{Items: r.Items, Meta: r.Meta, Truncated: r.Truncated}
	var err error
	if s.Issues, err = repo.ListIssues(); err != nil {
		return State{}, err
	}
	if s.Bases, err = LoadBases(repo, BasePrefix(cfg.GithubProject), s.Items, s.Issues); err != nil {
		return State{}, err
	}
	return s, nil
}

// Plan decides every synced field, without writing anything.
func (s State) Plan(cfg *config.Config) SyncPlan {
	return BuildSyncPlan(s.Items, s.Issues, s.Bases, s.Meta, cfg)
}

// ResolveConflict keeps one side of a conflict, decided again from what
// both sides hold in s: git-board's value written to the Project (ours), or
// the Project's set on the label. It returns the value both now hold. A
// number two repositories of the Project have in conflict is refused:
// ResolveConflictIn names the repository.
func ResolveConflict(repo Repo, p Project, cfg *config.Config, s State, number int, field string, ours bool) (string, error) {
	return ResolveConflictIn(repo, p, cfg, s, "", number, field, ours)
}

// ResolveConflictIn is ResolveConflict for the issue number of one
// repository, owner/name; "" is any, when only one has it in conflict.
func ResolveConflictIn(repo Repo, p Project, cfg *config.Config, s State, repoName string, number int, field string, ours bool) (string, error) {
	if ours && !cfg.GithubProject.Write {
		return "", ErrNoWrite
	}
	var found []Conflict
	for _, c := range s.Plan(cfg).Conflicts {
		if c.Number == number && c.Field == field && (repoName == "" || strings.EqualFold(repoOf(c.URL), repoName)) {
			found = append(found, c)
		}
	}
	if len(found) > 1 {
		var in []string
		for _, c := range found {
			in = append(in, fmt.Sprintf("%s#%d", repoOf(c.URL), number))
		}
		return "", fmt.Errorf("#%d %s is a conflict in more than one repository, name it: %s", number, field, strings.Join(in, ", "))
	}
	for _, c := range found {
		var item Item
		for _, it := range s.Items {
			if it.Content.Number == number && it.Content.URL == c.URL {
				item = it
			}
		}
		var bug store.Issue
		for _, b := range s.Issues {
			if b.ID == c.BugID {
				bug = b
			}
		}
		resolved, err := Resolve(c, ours, item, bug, s.Meta, cfg)
		if err != nil {
			return "", err
		}
		if _, err := ApplySync(repo, p.Writer, repo, resolved, cfg, s.Meta); err != nil {
			return "", err
		}
		if ours {
			return c.Local, nil
		}
		return c.Project, nil
	}
	if repoName != "" {
		return "", fmt.Errorf("%s#%d %s %w", repoName, number, field, ErrNotConflict)
	}
	return "", fmt.Errorf("#%d %s %w", number, field, ErrNotConflict)
}
