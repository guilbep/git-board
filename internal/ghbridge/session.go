package ghbridge

import (
	"errors"
	"fmt"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio"
)

// DefaultItemLimit is how many Project items a sync reads by default.
const DefaultItemLimit = 2000

// ErrNoProject is returned when .git-board.toml names no GitHub Project.
var ErrNoProject = errors.New("no GitHub project configured: run `git-board bridge configure` first")

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
}

// Gh is the Project through the gh CLI.
var Gh = Project{Items: FetchItems, Meta: FetchProjectMeta, Writer: GhWriter{}}

// Repo is the git-bug side of a sync.
type Repo interface {
	ListIssues() ([]gitbugio.Issue, error)
	ProjectBases(idPrefix, prefix string) (map[string]string, error)
	LabelWriter
	BaseWriter
}

var _ Repo = (*gitbugio.Backend)(nil)

// State is what a sync decides from, read from both sides.
type State struct {
	Issues []gitbugio.Issue
	Items  []Item
	Meta   ProjectMeta
	Bases  map[string]map[string]string
}

// LoadState reads the issues, the Project's items and fields, and the
// recorded bases.
func LoadState(repo Repo, p Project, cfg *config.Config, limit int) (State, error) {
	gp := cfg.GithubProject
	if gp.Owner == "" || gp.Number == 0 {
		return State{}, ErrNoProject
	}
	var s State
	var err error
	if s.Issues, err = repo.ListIssues(); err != nil {
		return State{}, err
	}
	if s.Items, err = p.Items(gp.Owner, gp.Number, limit); err != nil {
		return State{}, err
	}
	if s.Meta, err = p.Meta(gp.Owner, gp.Number); err != nil {
		return State{}, err
	}
	if s.Bases, err = LoadBases(repo, BasePrefix(gp), s.Items, s.Issues); err != nil {
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
// the Project's set on the label. It returns the value both now hold.
func ResolveConflict(repo Repo, p Project, cfg *config.Config, s State, number int, field string, ours bool) (string, error) {
	if ours && !cfg.GithubProject.Write {
		return "", ErrNoWrite
	}
	for _, c := range s.Plan(cfg).Conflicts {
		if c.Number != number || c.Field != field {
			continue
		}
		var item Item
		for _, it := range s.Items {
			if it.Content.Number == number && it.Content.URL == c.URL {
				item = it
			}
		}
		var bug gitbugio.Issue
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
	return "", fmt.Errorf("#%d %s %w", number, field, ErrNotConflict)
}
