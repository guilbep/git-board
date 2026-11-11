package httpapi

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/guilbep/git-board/app"
	"github.com/guilbep/git-board/forge"
	"github.com/guilbep/git-board/tools/ghproject"
)

// The GitHub Project bridge (docs/rewrite/api.md, routes 52 and 53): its
// last check, a check, a run one side or both, a conflict resolved; and
// where each field lives, on each forge. Only an action reads the
// Project: a read answers the last one, which the server keeps.

// projectState is the bridge's last run, kept by the server.
type projectState struct {
	mu   sync.Mutex // one action at a time
	last *ghproject.Report
}

type projectRes struct {
	Project string `json:"project"` // owner/number; "" when none is configured
	Write   bool   `json:"write"`
	// Fields are those the Project holds as its columns; Stated says the
	// file names them ([github-project] fields), else they are the default.
	Fields     []string     `json:"fields"`
	Stated     bool         `json:"stated"`
	Last       *lastRes     `json:"last"`
	Places     []placeRes   `json:"places"`
	Problem    string       `json:"problem,omitempty"` // [github-project] doesn't read
	Kept       string       `json:"kept,omitempty"`    // resolve: the value both now hold
	ResolvedOn *resolvedRes `json:"resolved,omitempty"`
}

type resolvedRes struct {
	Issue string `json:"issue"`
	Field string `json:"field"`
	Keep  string `json:"keep"`
}

// lastRes is a run, as a page shows it.
type lastRes struct {
	ghproject.Report
	Counts    ghproject.Counts      `json:"counts"`
	Conflicts []ghproject.Conflict  `json:"conflicts"`
	Unmatched []ghproject.Unmatched `json:"unmatched"`
	Changes   []changeRes           `json:"changes"`
	// Unmapped says the values skipped by choice, a line a field.
	Unmapped []string `json:"unmapped_by_choice"`
}

// changeRes is what a run does to one issue: its labels, its writes on
// the Project, its warnings.
type changeRes struct {
	Issue    string   `json:"issue"`
	Number   int      `json:"number"`
	URL      string   `json:"url"`
	Title    string   `json:"title"`
	Added    []string `json:"added"`
	Removed  []string `json:"removed"`
	Writes   []string `json:"writes"` // field = value on the Project; assignees on the issue
	Warnings []string `json:"warnings"`
}

// placeRes is where a field lives on a forge: its target, the Project's
// column for the Project, and whether the Project has that column as the
// last check read it (absent before any).
type placeRes struct {
	Field     string `json:"field"`
	Forge     string `json:"forge"`
	Kind      string `json:"kind"`
	To        string `json:"to"`
	Default   string `json:"default"` // its type's target
	Column    string `json:"column,omitempty"`
	OnProject *bool  `json:"on_project,omitempty"`
}

func lastOf(r *ghproject.Report) *lastRes {
	if r == nil {
		return nil
	}
	l := &lastRes{Report: *r, Counts: r.Plan.Counts(), Conflicts: r.Plan.Conflicts, Unmatched: r.Plan.Unmatched, Changes: []changeRes{},
		Unmapped: nonNil(r.Plan.UnmappedLines())}
	if l.Conflicts == nil {
		l.Conflicts = []ghproject.Conflict{}
	}
	if l.Unmatched == nil {
		l.Unmatched = []ghproject.Unmatched{}
	}
	writes := map[string][]string{}
	for _, e := range r.Plan.Edits {
		w := e.Field + " = " + e.Value
		if e.Kind == ghproject.EditAssignees {
			w = fmt.Sprintf("assignees +%v -%v", e.Add, e.Remove)
		}
		writes[e.IssueID] = append(writes[e.IssueID], w)
	}
	for _, c := range r.Plan.Changes {
		if len(c.Added)+len(c.Removed)+len(c.Warnings)+len(writes[c.IssueID]) == 0 {
			continue
		}
		l.Changes = append(l.Changes, changeRes{Issue: c.IssueID, Number: c.Number, URL: c.URL, Title: c.Title,
			Added: nonNil(c.Added), Removed: nonNil(c.Removed), Writes: nonNil(writes[c.IssueID]), Warnings: ghproject.Texts(c.Warnings)})
	}
	return l
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// places are where each field lives on each forge of the board: its
// [forges] tables, else GitHub's.
func places(b *app.Board, c *ghproject.Config, last *ghproject.Report) []placeRes {
	tables := b.Config.Forges.List
	if len(tables) == 0 {
		tables = []forge.Table{c.Forge}
	}
	names := b.Config.Schema.FieldOrder()
	for _, f := range c.Holds {
		if c.Configured() && !slices.Contains(names, f) {
			names = append(names, f)
		}
	}
	out := []placeRes{}
	for _, t := range tables {
		for _, name := range names {
			typ, _, _ := b.Config.Schema.Field(name)
			p := placeRes{Field: name, Forge: t.Name, Kind: t.Kind, To: forge.Target(b.Config.Schema, c.InProject, t, name),
				Default: forge.DefaultTarget(t.Kind, name, typ)}
			if p.To == forge.TargetProject && t.Name == c.Forge.Name {
				p.Column = c.Column(name)
				if last != nil {
					_, on := last.Columns[p.Column]
					p.OnProject = &on
				}
			}
			out = append(out, p)
		}
	}
	return out
}

// projectStatus is route 52: the bridge's configuration, its last run,
// where each field lives.
func (s *Server) projectStatus(ctx context.Context, _ none) (projectRes, error) {
	s.project.mu.Lock()
	last := s.project.last
	s.project.mu.Unlock()
	return s.projectAnswer(ctx, last), nil
}

func (s *Server) projectAnswer(ctx context.Context, last *ghproject.Report) projectRes {
	b := board(ctx)
	c, err := b.Config.Project()
	if err != nil {
		return projectRes{Places: []placeRes{}, Problem: err.Error()}
	}
	gp, _ := b.Config.Tools.GithubProject.Get()
	res := projectRes{Write: c.Write, Places: places(b, c, nil), Fields: []string{}}
	if c.Configured() {
		res.Fields, res.Stated = c.Holds, gp.Fields != nil
		res.Project = c.Name()
		if last != nil && last.Project == c.Name() { // another Project's run says nothing of this one
			res.Last, res.Places = lastOf(last), places(b, c, last)
		}
	}
	return res
}

// configureReq is route 58: the Project named from the page, as project
// configure does (app.ConfigureProject): the fields it holds are written
// with it; an owner "" removes the table.
type configureReq struct {
	Owner  string `json:"owner"`
	Number int    `json:"number"`
}

// configureProject writes [github-project], then answers as GET does.
func (s *Server) configureProject(ctx context.Context, r configureReq) (projectRes, error) {
	err := s.writeConfig(ctx, func() error {
		b := board(ctx)
		gp, _ := b.Config.Tools.GithubProject.Get()
		_, err := app.ConfigureProject(b.Dir, gp, strings.TrimSpace(r.Owner), r.Number)
		return err
	})
	if err != nil {
		return projectRes{}, on("owner", err)
	}
	h := s.boards.get()
	defer s.boards.release(h)
	return s.projectStatus(context.WithValue(ctx, boardKey{}, h.b), none{})
}

type projectReq struct {
	Action string `path:"action"`
	// apply: labels (pull), project (push), both (sync, the default)
	Side ghproject.Side `json:"side"`
	// resolve: the issue on GitHub (2605, owner/repo#2605, its URL), the
	// field, and the side kept: git-board or project
	Issue string `json:"issue"`
	Field string `json:"field"`
	Keep  string `json:"keep"`
	Limit int    `json:"limit"`
}

// projectAction is route 53: check, apply or resolve.
func (s *Server) projectAction(ctx context.Context, r projectReq) (projectRes, error) {
	if !s.project.mu.TryLock() {
		return projectRes{}, &app.Error{Class: app.Busy, Err: errors.New("the GitHub Project is being read: try again once it ends")}
	}
	defer s.project.mu.Unlock()
	b := board(ctx)
	side := r.Side
	switch side {
	case "":
		side = ghproject.Both
	case ghproject.Labels, ghproject.OnGH, ghproject.Both:
	default:
		return projectRes{}, invalid("side", fmt.Errorf("side %q: labels, project or both", side))
	}
	var kept string
	switch r.Action {
	case "check", "apply":
	case "resolve":
		if r.Issue == "" || r.Field == "" {
			return projectRes{}, invalid("issue", errors.New("resolve takes an issue, a field and the side kept"))
		}
		if r.Keep != "git-board" && r.Keep != "project" {
			return projectRes{}, invalid("keep", fmt.Errorf("keep %q: git-board or project", r.Keep))
		}
		var err error
		if kept, err = b.ProjectResolve(ctx, r.Issue, r.Field, r.Keep == "git-board"); err != nil {
			return projectRes{}, err
		}
	default:
		return projectRes{}, &app.Error{Class: app.NotFound, Err: fmt.Errorf("no action %q on the GitHub Project: check, apply or resolve", r.Action)}
	}
	// a resolve checks again: its page shows the conflicts left
	rep, err := b.ProjectRun(ctx, side, r.Action == "apply", r.Limit)
	if err != nil {
		return projectRes{}, err
	}
	s.project.last = &rep
	res := s.projectAnswer(ctx, &rep)
	if r.Action == "resolve" {
		res.Kept, res.ResolvedOn = kept, &resolvedRes{Issue: r.Issue, Field: r.Field, Keep: r.Keep}
	}
	return res, nil
}
