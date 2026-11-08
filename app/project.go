package app

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/guilbep/git-board/forge"
	"github.com/guilbep/git-board/forge/github"
	"github.com/guilbep/git-board/tools"
	"github.com/guilbep/git-board/tools/ghproject"
)

// The GitHub Project bridge (tools/ghproject): [github-project]'s
// Project, its fields kept in step with the labels. The token is the
// GitHub forge's (ADR 0010): the posting account of the first GitHub
// [forges] table, else yours on the host (what forge login stores by
// default), else any account on github.com.

// ErrNoProject is a bridge command on a board whose .git-board.toml
// names no Project.
var ErrNoProject = ghproject.ErrNoProject

// OpenProjectV2 opens a Project: GitHub's API, or a fake in tests.
var OpenProjectV2 = func(ctx context.Context, host, login, owner string, number int) (ghproject.Project, error) {
	api := ""
	if host == "github.com" {
		api = GitHubAPI
	}
	return github.OpenProject(ctx, github.Options{Host: host, Login: login, API: api}, owner, number)
}

// githubForge is the board's GitHub forge table: the first declared,
// else github.com's, with no mapping.
func (c *Config) githubForge() forge.Table {
	for _, f := range c.Forges.List {
		if f.Kind == "github" {
			return f
		}
	}
	return forge.Table{Name: "gh", Kind: "github", Host: "github.com"}
}

// gitHubRepo is the board's github.com repository, owner/repo, when one
// table names it: whose numbers below the counter's start are the
// issues' own (ADR 0016). Two such tables, or none: "", and the counter's
// own line or the sync records decide (issues.Counter.GitHub).
func (c *Config) gitHubRepo() string {
	repo := ""
	for _, f := range c.Forges.List {
		if f.Kind != "github" || !strings.EqualFold(f.Host, "github.com") {
			continue
		}
		if repo != "" && !strings.EqualFold(repo, f.Project) {
			return ""
		}
		repo = cmp.Or(repo, f.Project)
	}
	return repo
}

// Project is the bridge's configuration: [github-project], the fields and
// the GitHub forge's mappings. A broken [github-project] is its problem.
func (c *Config) Project() (*ghproject.Config, error) {
	gp, probs := c.Tools.GithubProject.Get()
	if len(probs) > 0 {
		return nil, usage(probs[0].Err)
	}
	return &ghproject.Config{Owner: gp.Owner, Number: gp.Number, Write: gp.Write, Holds: gp.Columns(), Schema: c.Schema, Forge: c.githubForge()}, nil
}

// projectSession is the bridge opened: its config, the Project, the board.
func (b *Board) projectSession(ctx context.Context) (*ghproject.Config, ghproject.Project, error) {
	c, err := b.Config.Project()
	if err != nil {
		return nil, nil, err
	}
	if !c.Configured() {
		return nil, nil, usage(ErrNoProject)
	}
	if err := b.switched(ctx); err != nil {
		return nil, nil, err
	}
	host := cmp.Or(c.Forge.Host, "github.com")
	login := c.Forge.Login
	if login == "" { // a table with no login, or no table: yours, what
		// forge login stores by default, else any account on the host
		login = b.myLogin(ctx, host)
	}
	p, err := OpenProjectV2(ctx, host, login, c.Owner, c.Number)
	if err != nil {
		return nil, nil, err
	}
	return c, p, nil
}

// ProjectRun reads the Project and the issues, plans side's run (labels:
// pull, project: push, both: sync), and with apply writes it, to the
// Project with write = true alone. limit is how many items it reads, 0
// the default.
func (b *Board) ProjectRun(ctx context.Context, side ghproject.Side, apply bool, limit int) (ghproject.Report, error) {
	c, p, err := b.projectSession(ctx)
	if err != nil {
		return ghproject.Report{}, err
	}
	return ghproject.Run(ctx, p, ghproject.Service{S: b.Issues}, c, cmp.Or(limit, ghproject.DefaultLimit), side, apply)
}

// ProjectResolve keeps one side of a conflict: git-board's value written
// to the Project (ours), or the Project's on the label. ref is the issue
// on GitHub: its number, owner/repo#number, or its URL. It answers the
// value both now hold.
func (b *Board) ProjectResolve(ctx context.Context, ref, field string, ours bool) (string, error) {
	repo, number, err := ParseIssueRef(ref)
	if err != nil {
		return "", usage(err)
	}
	c, p, err := b.projectSession(ctx)
	if err != nil {
		return "", err
	}
	if ours && !c.Write {
		return "", usage(ghproject.ErrNoWrite)
	}
	bd := ghproject.Service{S: b.Issues}
	s, err := ghproject.Load(ctx, p, bd, c, ghproject.DefaultLimit)
	if err != nil {
		return "", err
	}
	kept, err := ghproject.ResolveIn(ctx, bd, p, c, s, repo, number, field, ours)
	switch {
	case errors.Is(err, ghproject.ErrNotConflict):
		return "", notFound(fmt.Errorf("%w: run git board project sync to see the conflicts", err))
	case errors.Is(err, ghproject.ErrNoOption), errors.Is(err, ghproject.ErrAmbiguous):
		return "", usage(err)
	}
	return kept, err
}

// ParseIssueRef reads 2605, owner/repo#2605 or the issue's GitHub URL.
func ParseIssueRef(ref string) (repo string, number int, err error) {
	n := ref
	if rest, ok := strings.CutPrefix(ref, "https://github.com/"); ok {
		parts := strings.Split(strings.TrimSuffix(rest, "/"), "/")
		if len(parts) != 4 || parts[2] != "issues" {
			return "", 0, fmt.Errorf("issue %q: an issue's URL is https://github.com/<owner>/<repo>/issues/<number>", ref)
		}
		repo, n = parts[0]+"/"+parts[1], parts[3]
	} else if r, num, ok := strings.Cut(ref, "#"); ok {
		if strings.Count(r, "/") != 1 {
			return "", 0, fmt.Errorf("issue %q: owner/repo#number", ref)
		}
		repo, n = r, num
	}
	if number, err = strconv.Atoi(n); err != nil {
		return "", 0, fmt.Errorf("issue %q: its number, owner/repo#number or its URL", ref)
	}
	return repo, number, nil
}

// ConfigureProject writes [github-project]'s owner and number, write
// kept for the same Project alone; owner "" removes the table. It answers
// the file's path.
func ConfigureProject(dir string, gp tools.GithubProject, owner string, number int) (string, error) {
	var table any // nil removes it
	if owner != "" {
		if number <= 0 || strings.ContainsAny(owner, "/ \t\r\n") {
			return "", usage(fmt.Errorf("a Project is an owner and its number: --owner EPFL-ENAC --project 40"))
		}
		// the fields it holds are stated: the table's own, else the default
		t := map[string]any{"owner": owner, "number": int64(number), "fields": slices.Clone(gp.Columns())}
		if gp.Write && strings.EqualFold(gp.Owner, owner) && gp.Number == number {
			t["write"] = true
		}
		table = t
	}
	_, err := SetConfigKeys(dir, map[string]any{"github-project": table}, "", false)
	return configPath(dir), err
}

func usage(err error) error    { return &Error{Class: Usage, Err: err} }
func notFound(err error) error { return &Error{Class: NotFound, Err: err} }

// ProjectNote is a run of the bridge in one line: what it changed, and
// what it held.
func ProjectNote(r ghproject.Report) string {
	n := r.Plan.Counts()
	parts := []string{fmt.Sprintf("%d issue(s) matched", n.Matched)}
	if n.LabelChanges > 0 {
		parts = append(parts, fmt.Sprintf("%d with label changes", n.LabelChanges))
	}
	if w := n.ProjectWrites - r.Skipped; w > 0 {
		parts = append(parts, fmt.Sprintf("%d Project write(s)", w))
	}
	if r.Skipped > 0 {
		parts = append(parts, fmt.Sprintf("%d not written (no write = true)", r.Skipped))
	}
	if n.Conflicts > 0 {
		parts = append(parts, fmt.Sprintf("%d conflict(s) held", n.Conflicts))
	}
	if r.Truncated {
		parts = append(parts, fmt.Sprintf("only the first %d items read", r.Items))
	}
	return strings.Join(parts, ", ")
}
