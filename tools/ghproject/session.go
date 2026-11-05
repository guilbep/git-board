package ghproject

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/guilbep/git-board/domain/issues"
	"github.com/guilbep/git-board/forge"
	"github.com/guilbep/git-board/forge/github"
	"github.com/guilbep/git-board/kernel"
	bsync "github.com/guilbep/git-board/sync"
)

// DefaultLimit is how many Project items a sync reads by default.
const DefaultLimit = 2000

var (
	// ErrNoProject is a bridge command on a board whose .git-board.toml
	// names no Project.
	ErrNoProject = errors.New("no [github-project] in .git-board.toml: git board project configure --owner <org> --project <n>")
	// ErrNoWrite is keeping git-board's value, which writes to a Project
	// the file doesn't allow writing to.
	ErrNoWrite = errors.New("keeping git-board's value writes to the Project: set write = true under [github-project] in .git-board.toml")
	// ErrNoOption is keeping git-board's value, which the Project's field
	// has no option for.
	ErrNoOption = errors.New("has no option on the Project")
	// ErrAmbiguous is a number two repositories of the Project have in
	// conflict.
	ErrAmbiguous = errors.New("is a conflict in more than one repository")
	// ErrNotConflict is resolving a field that isn't a conflict.
	ErrNotConflict = errors.New("isn't a conflict now")
)

// Project is how the bridge reads and writes a GitHub Project:
// github.ProjectV2, or a fake.
type Project interface {
	Meta(ctx context.Context) (github.ProjectMeta, error)
	Items(ctx context.Context, limit int) ([]Item, bool, error)
	Assignable(ctx context.Context, repo string) ([]string, error)
	Set(ctx context.Context, projectID string, v github.ProjectValue) error
	Assign(ctx context.Context, repo string, number int, add, remove []string) error
}

var _ Project = (*github.ProjectV2)(nil)

// Board is the board's side of the bridge: the issues, their labels, and
// what each last agreed on with the Project.
type Board interface {
	Issues(ctx context.Context) ([]Issue, error)
	Bases(ctx context.Context, id, prefix string) (map[string]string, error)
	ChangeLabels(ctx context.Context, id string, add, remove []string) error
	SetBases(ctx context.Context, id, prefix string, values map[string]string) error
}

// Service is the board's issues as the bridge's Board.
type Service struct{ S *issues.Service }

var _ Board = Service{}

// Issues are every issue, its GitHub issue and its labels.
func (s Service) Issues(ctx context.Context) ([]Issue, error) {
	list, err := s.S.List(ctx, issues.ListReq{})
	if err != nil {
		return nil, err
	}
	out := make([]Issue, 0, len(list))
	for _, is := range list {
		out = append(out, Issue{ID: is.ID, URL: is.GithubURL, Labels: is.Labels})
	}
	return out, nil
}

// Bases are the issue's metadata under prefix.
func (s Service) Bases(ctx context.Context, id, prefix string) (map[string]string, error) {
	meta, err := s.S.Meta(ctx, id)
	if err != nil {
		return nil, err
	}
	res := map[string]string{}
	for k, v := range meta {
		if field, ok := strings.CutPrefix(k, prefix); ok {
			res[field] = v
		}
	}
	return res, nil
}

func (s Service) ChangeLabels(ctx context.Context, id string, add, remove []string) error {
	_, err := s.S.ChangeLabels(ctx, id, add, remove)
	return err
}

// SetBases records values under prefix in the issue's metadata, so they
// travel with it.
func (s Service) SetBases(ctx context.Context, id, prefix string, values map[string]string) error {
	if len(values) == 0 {
		return nil
	}
	meta := make(map[string]string, len(values))
	for field, v := range values {
		meta[prefix+field] = v
	}
	if err := s.S.SetMeta(ctx, id, meta); err != nil {
		return fmt.Errorf("recording Project values on %s: %w", id, err)
	}
	return nil
}

// Remote is the Project's side, read over the network.
type Remote struct {
	Items []Item
	Meta  Meta
	// Truncated says the read stopped at its limit: the Project has more
	// items, which this sync leaves out.
	Truncated bool
}

// Fetch reads the Project's items and fields, and, for a sync allowed to
// write, who can be assigned in each repository its issues are in.
func Fetch(ctx context.Context, p Project, c *Config, limit int) (Remote, error) {
	if !c.Configured() {
		return Remote{}, ErrNoProject
	}
	var r Remote
	var err error
	if r.Items, r.Truncated, err = p.Items(ctx, limit); err != nil {
		return Remote{}, err
	}
	m, err := p.Meta(ctx)
	if err != nil {
		return Remote{}, err
	}
	r.Meta = MetaOf(m)
	if c.Write {
		r.Meta.Assignable = map[string][]string{}
		for _, it := range r.Items {
			repo := repoOf(it.URL)
			if _, done := r.Meta.Assignable[repo]; done || it.Type != "Issue" {
				continue
			}
			if r.Meta.Assignable[repo], err = p.Assignable(ctx, repo); err != nil {
				return Remote{}, err
			}
		}
	}
	return r, nil
}

// State is what a sync decides from, both sides read.
type State struct {
	Remote
	Issues []Issue
	Bases  map[string]map[string]string
}

// Load reads both sides.
func Load(ctx context.Context, p Project, b Board, c *Config, limit int) (State, error) {
	r, err := Fetch(ctx, p, c, limit)
	if err != nil {
		return State{}, err
	}
	return r.State(ctx, b, c)
}

// State adds the board's side to r: the issues as they are now, and the
// values both sides last agreed on.
func (r Remote) State(ctx context.Context, b Board, c *Config) (State, error) {
	s := State{Remote: r, Bases: map[string]map[string]string{}}
	var err error
	if s.Issues, err = b.Issues(ctx); err != nil {
		return State{}, err
	}
	urls := map[string]bool{}
	for _, it := range r.Items {
		urls[it.URL] = true
	}
	for _, is := range s.Issues {
		if is.URL == "" || !urls[is.URL] {
			continue
		}
		if s.Bases[is.ID], err = b.Bases(ctx, is.ID, c.BasePrefix()); err != nil {
			return State{}, err
		}
	}
	return s, nil
}

// Plan decides every synced field, without writing anything.
func (s State) Plan(c *Config) Plan { return BuildPlan(s.Items, s.Issues, s.Bases, s.Meta, c) }

// Apply applies a plan issue by issue: Project edits, then labels, then
// bases, so a run stopped halfway leaves agreeing values that the next run
// records. Without write nothing is written to the Project, and those
// fields' bases aren't recorded; it answers how many edits it skipped. It
// stops at the first failed write.
func Apply(ctx context.Context, b Board, p Project, plan Plan, c *Config, meta Meta) (skipped int, err error) {
	edits := map[string][]Edit{}
	for _, e := range plan.Edits {
		edits[e.IssueID] = append(edits[e.IssueID], e)
	}
	prefix := c.BasePrefix()
	for _, ch := range plan.Changes {
		record := plan.Bases[ch.IssueID]
		for _, e := range edits[ch.IssueID] {
			if !c.Write {
				skipped++
				record = without(record, e.Field)
				continue
			}
			if err := write(ctx, p, meta.ID, e); err != nil {
				return skipped, fmt.Errorf("#%d %s: %w", e.Number, e.Field, err)
			}
		}
		if len(ch.Added)+len(ch.Removed) > 0 {
			if err := b.ChangeLabels(ctx, ch.IssueID, ch.Added, ch.Removed); err != nil {
				return skipped, fmt.Errorf("#%d: %w", ch.Number, err)
			}
		}
		if len(record) > 0 {
			if err := b.SetBases(ctx, ch.IssueID, prefix, record); err != nil {
				return skipped, fmt.Errorf("#%d: %w", ch.Number, err)
			}
		}
	}
	return skipped, nil
}

// write sends one edit: a Project field's value, or the issue's assignees.
func write(ctx context.Context, p Project, projectID string, e Edit) error {
	if e.Kind == EditAssignees {
		return p.Assign(ctx, repoOf(e.URL), e.Number, e.Add, e.Remove)
	}
	return p.Set(ctx, projectID, github.ProjectValue{Item: e.ItemID, Field: e.FieldID, Kind: e.Kind, Arg: e.Arg})
}

// ResolveIn keeps one side of a conflict, decided again from what both
// sides hold in s: git-board's value written to the Project (ours), or
// the Project's set on the label. repo, owner/name, names the
// repository of number; "" is any, when only one has it in conflict. It
// answers the value both now hold.
func ResolveIn(ctx context.Context, b Board, p Project, c *Config, s State, repo string, number int, field string, ours bool) (string, error) {
	if ours && !c.Write {
		return "", ErrNoWrite
	}
	var found []Conflict
	for _, k := range s.Plan(c).Conflicts {
		if k.Number == number && k.Field == field && (repo == "" || strings.EqualFold(repoOf(k.URL), repo)) {
			found = append(found, k)
		}
	}
	if len(found) > 1 {
		var in []string
		for _, k := range found {
			in = append(in, fmt.Sprintf("%s#%d", repoOf(k.URL), number))
		}
		return "", fmt.Errorf("#%d %s %w, name it: %s", number, field, ErrAmbiguous, strings.Join(in, ", "))
	}
	for _, k := range found {
		var it Item
		for _, x := range s.Items {
			if x.Number == number && x.URL == k.URL {
				it = x
			}
		}
		var is Issue
		for _, x := range s.Issues {
			if x.ID == k.IssueID {
				is = x
			}
		}
		resolved, err := Resolve(k, ours, it, is, s.Meta, c)
		if err != nil {
			return "", err
		}
		if _, err := Apply(ctx, b, p, resolved, c, s.Meta); err != nil {
			return "", err
		}
		if ours {
			return k.Local, nil
		}
		return k.Project, nil
	}
	if repo != "" {
		return "", fmt.Errorf("%s#%d %s %w", repo, number, field, ErrNotConflict)
	}
	return "", fmt.Errorf("#%d %s %w", number, field, ErrNotConflict)
}

// Side is which side a run writes: the labels (pull), the Project (push),
// or both (sync).
type Side string

// The sides.
const (
	Labels Side = "labels"
	OnGH   Side = "project"
	Both   Side = "both"
)

// SideOf is the side a sync way writes.
func SideOf(w bsync.Way) Side {
	switch w {
	case bsync.Pull:
		return Labels
	case bsync.Push:
		return OnGH
	}
	return Both
}

// Report is one run: what it planned, and, applied, what it skipped.
type Report struct {
	Project   string    `json:"project"` // owner/number
	Title     string    `json:"title"`   // the Project's, as GitHub answered
	CheckedAt time.Time `json:"checked_at"`
	Side      Side      `json:"side"`
	Applied   bool      `json:"applied"`
	Write     bool      `json:"write"` // write = true: a sync may write to the Project
	Truncated bool      `json:"truncated"`
	Plan      Plan      `json:"-"`
	Items     int       `json:"items"`
	Skipped   int       `json:"skipped"` // edits not written: no write = true
	// Columns are the Project's fields, by name, with their type: where
	// a field lives can say the Project has its column.
	Columns map[string]string `json:"columns"`
}

// Counts are a plan's numbers, as the API answers them.
type Counts struct {
	Matched       int `json:"matched"`
	LabelChanges  int `json:"label_changes"`
	ProjectWrites int `json:"project_writes"`
	Conflicts     int `json:"conflicts"`
	Warnings      int `json:"warnings"`
	Unmatched     int `json:"unmatched"`
}

// Counts counts the plan.
func (p Plan) Counts() Counts {
	n := Counts{Matched: len(p.Changes), ProjectWrites: len(p.Edits), Conflicts: len(p.Conflicts), Unmatched: len(p.Unmatched)}
	for _, c := range p.Changes {
		if len(c.Added)+len(c.Removed) > 0 {
			n.LabelChanges++
		}
		n.Warnings += len(c.Warnings)
	}
	return n
}

// Run reads both sides and plans side's run; apply writes it, to the
// Project with write = true alone.
func Run(ctx context.Context, p Project, b Board, c *Config, limit int, side Side, apply bool) (Report, error) {
	s, err := Load(ctx, p, b, c, limit)
	if err != nil {
		return Report{}, err
	}
	plan := s.Plan(c)
	switch side {
	case Labels:
		plan = plan.LabelsOnly()
	case OnGH:
		plan = plan.ProjectOnly()
	}
	r := Report{Project: c.Name(), Title: s.Meta.Title, CheckedAt: time.Now().UTC(), Side: side, Write: c.Write, Truncated: s.Truncated,
		Plan: plan, Items: len(s.Items), Columns: map[string]string{}}
	for name, f := range s.Meta.Fields {
		r.Columns[name] = f.Type
	}
	if !apply {
		return r, nil
	}
	r.Applied = true
	r.Skipped, err = Apply(ctx, b, p, plan, c, s.Meta)
	return r, err
}

// Mapper is the bridge on the sync engine (bsync.Mapper): the Project's
// fields, a third kind after the issues and the releases. It carries no
// forge's items: a run is given the GitHub forges, and reads the Project
// [github-project] names.
type Mapper struct {
	Project Project
	Board   Board
	Config  *Config
	Limit   int
	Check   bool   // plan, write nothing
	Report  Report // the last run's
}

var _ bsync.Mapper = (*Mapper)(nil)

func (m *Mapper) Kind() string { return "project" }

// Carries says the forge is a GitHub one: the Project is GitHub's.
func (m *Mapper) Carries(f forge.Forge) bool {
	return f != nil && strings.HasPrefix(f.Key(), "github.com/")
}

// Run runs the bridge one way: a pull takes the Project's changes into
// the labels, a push writes git-board's, a sync both.
func (m *Mapper) Run(ctx context.Context, _ *kernel.Board, way bsync.Way, _ []forge.Forge) error {
	r, err := Run(ctx, m.Project, m.Board, m.Config, m.Limit, SideOf(way), !m.Check)
	m.Report = r
	return err
}
