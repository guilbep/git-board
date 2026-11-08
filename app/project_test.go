package app

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/guilbep/git-board/domain/issues"
	"github.com/guilbep/git-board/domain/issues/issuesync"
	"github.com/guilbep/git-board/forge"
	"github.com/guilbep/git-board/forge/fake"
	"github.com/guilbep/git-board/forge/github"
	"github.com/guilbep/git-board/kernel"
	"github.com/guilbep/git-board/tools/ghproject"
	"github.com/guilbep/git-board/tools/ghproject/projectfake"
)

const projectToml = `[fields.status]
single = true
options = ["backlog", "ready", "in-review", "qa"]

[github-project]
owner = "o"
number = 1
write = true
`

// projectBoard is a repository whose two issues came from github.com/o/r,
// its .git-board.toml naming o's Project 1, opened as who writes; and the
// Project, served, which OpenProjectV2 opens for the test.
func projectBoard(t *testing.T) (*Board, *projectfake.Fake) {
	t.Helper()
	ctx := t.Context()
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git-board.toml"), []byte(projectToml), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := Open(ctx, dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	me, err := issuesync.CreatePerson(ctx, b.Kernel, issuesync.NewPerson("", "me", ""))
	if err != nil {
		t.Fatal(err)
	}
	f := fake.New("github.com/o/r")
	for _, title := range []string{"One", "Two"} {
		f.Add(forge.Issue{Author: forge.Account{ID: "7", Login: "ann"}, Title: title})
	}
	if _, err := b.SyncForges(ctx, WayPull, ForgeRun{Forges: []NamedForge{{Name: "gh", Forge: f}}}); err != nil {
		t.Fatal(err)
	}
	b.Close()
	if out, err := exec.Command("git", "-C", dir, "config", "board.me", string(me.ID)).CombinedOutput(); err != nil {
		t.Fatalf("git config: %v\n%s", err, out)
	}
	if b, err = Open(ctx, dir, Options{Who: true}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b.Close() })

	p := projectfake.New("o", 1)
	p.AddField(projectfake.Field{ID: "F-status", Name: "Status", DataType: "SINGLE_SELECT", Choices: []projectfake.Choice{
		{ID: "o-backlog", Name: "Backlog"}, {ID: "o-ready", Name: "Ready"}, {ID: "o-review", Name: "In review"}}})
	for n, status := range []string{"Ready", "Backlog"} {
		p.Add(projectfake.Item{Number: n + 1, URL: "https://github.com/o/r/issues/" + string(rune('1'+n)), Values: map[string]string{"Status": status}})
	}
	srv := p.Server(t)
	was := OpenProjectV2
	OpenProjectV2 = func(ctx context.Context, host, _, owner string, number int) (ghproject.Project, error) {
		if host != "github.com" {
			t.Errorf("host %q", host)
		}
		return github.OpenProject(ctx, github.Options{API: srv.URL, Token: func(context.Context, forge.Account) (string, error) { return "t", nil }}, owner, number)
	}
	t.Cleanup(func() { OpenProjectV2 = was })
	return b, p
}

var issuesListAll = issues.ListReq{}

func issueOf(t *testing.T, b *Board, n string) string {
	t.Helper()
	list, err := b.Issues.List(t.Context(), issuesListAll)
	if err != nil {
		t.Fatal(err)
	}
	for _, is := range list {
		if strings.HasSuffix(is.GithubURL, "/issues/"+n) {
			return is.ID
		}
	}
	t.Fatalf("no issue #%s", n)
	return ""
}

func statusOf(t *testing.T, b *Board, n string) []string {
	t.Helper()
	list, err := b.Issues.List(t.Context(), issuesListAll)
	if err != nil {
		t.Fatal(err)
	}
	for _, is := range list {
		if strings.HasSuffix(is.GithubURL, "/issues/"+n) {
			return is.Labels
		}
	}
	t.Fatalf("no issue #%s", n)
	return nil
}

// TestProjectLogin: with no [forges] table naming a login, the Project
// bridge reads the token of your own account on the host (what forge
// login stores by default), not any account's.
func TestProjectLogin(t *testing.T) {
	b, _ := projectBoard(t)
	ctx := t.Context()
	me := string(b.People.Me)
	if _, err := issuesync.DoPerson(ctx, b.Kernel, me, issuesync.AddAccount(me, issuesync.Account{Server: "github.com", ID: "42", Login: "ada"})); err != nil {
		t.Fatal(err)
	}
	sessions := func() (string, error) {
		var login string
		was := OpenProjectV2
		OpenProjectV2 = func(ctx context.Context, host, l, owner string, number int) (ghproject.Project, error) {
			login = l
			return was(ctx, host, l, owner, number)
		}
		t.Cleanup(func() { OpenProjectV2 = was })
		_, _, err := b.projectSession(ctx)
		return login, err
	}
	if login, err := sessions(); err != nil || login != "ada" {
		t.Errorf("login %q, %v", login, err)
	}
	if _, err := issuesync.DoPerson(ctx, b.Kernel, me, kernel.Op{Kind: "remove-account", Author: me, Time: 1700000500, Data: json.RawMessage(`{"server":"github.com","id":"42"}`)}); err != nil {
		t.Fatal(err)
	}
	if login, err := sessions(); err != nil || login != "" {
		t.Errorf("login without the account %q, %v", login, err)
	}
}

// The bridge through the app: a pull, a push, a conflict resolved, and
// the whole sync's step.
func TestProjectRun(t *testing.T) {
	b, p := projectBoard(t)
	ctx := t.Context()
	r, err := b.ProjectRun(ctx, ghproject.Labels, true, 0)
	if err != nil {
		t.Fatal(err)
	}
	if n := r.Plan.Counts(); n.Matched != 2 || n.LabelChanges != 2 {
		t.Errorf("pull: %+v", n)
	}
	if got := statusOf(t, b, "1"); !slices.Contains(got, "status:ready") {
		t.Errorf("#1 %v", got)
	}
	if ProjectNote(r) != "2 issue(s) matched, 2 with label changes" {
		t.Errorf("note %q", ProjectNote(r))
	}

	// git-board moves #1, the Project moves #2 the other way too: one
	// write each side, and a conflict once both move one
	id := issueOf(t, b, "1")
	if _, err := b.Issues.ChangeLabels(ctx, id, []string{"status:in-review"}, []string{"status:ready"}); err != nil {
		t.Fatal(err)
	}
	var steps []string
	if err := b.SyncWhole(ctx, Whole{Remote: "origin", Step: func(name, state, note string) { steps = append(steps, name+" "+state+" "+note) }}); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(steps, "project done 2 issue(s) matched, 1 Project write(s)") {
		t.Errorf("steps %q", steps)
	}
	if w := p.Writes(); !slices.Equal(w, []string{"#1 Status = In review"}) {
		t.Errorf("writes %q", w)
	}

	if _, err := b.Issues.ChangeLabels(ctx, id, []string{"status:qa"}, []string{"status:in-review"}); err != nil {
		t.Fatal(err)
	}
	p.Edit("PVTI_1", func(it *projectfake.Item) { it.Values["Status"] = "Backlog" })
	if _, err := b.ProjectResolve(ctx, "o/r#1", "status", true); err == nil || Classify(err) != Usage {
		t.Errorf("ours with no option for qa: %v", err) // qa has no option on the Project
	}
	if _, err := b.ProjectResolve(ctx, "1", "prio", false); Classify(err) != NotFound {
		t.Errorf("not a conflict: %v", err)
	}
	kept, err := b.ProjectResolve(ctx, "https://github.com/o/r/issues/1", "status", false)
	if err != nil || kept != "backlog" {
		t.Fatalf("resolve: %q, %v", kept, err)
	}
	if got := statusOf(t, b, "1"); !slices.Contains(got, "status:backlog") {
		t.Errorf("#1 after resolve %v", got)
	}
}

// configure writes [github-project], write kept for the same Project.
func TestConfigureProject(t *testing.T) {
	b, _ := projectBoard(t)
	gp, _ := b.Config.Tools.GithubProject.Get()
	if _, err := ConfigureProject(b.Dir, gp, "o", 1); err != nil {
		t.Fatal(err)
	}
	text, _, _ := ConfigText(b.Dir)
	if !strings.Contains(string(text), "write = true") {
		t.Errorf("write lost:\n%s", text)
	}
	if _, err := ConfigureProject(b.Dir, gp, "EPFL-ENAC", 40); err != nil {
		t.Fatal(err)
	}
	text, _, _ = ConfigText(b.Dir)
	if !strings.Contains(string(text), `owner = "EPFL-ENAC"`) || !strings.Contains(string(text), "number = 40") || strings.Contains(string(text), "write") {
		t.Errorf("another Project:\n%s", text)
	}
	if _, err := ConfigureProject(b.Dir, gp, "", 0); err != nil {
		t.Fatal(err)
	}
	if text, _, _ = ConfigText(b.Dir); strings.Contains(string(text), "github-project") {
		t.Errorf("removed:\n%s", text)
	}
	if _, err := ConfigureProject(b.Dir, gp, "a/b", 1); Classify(err) != Usage {
		t.Errorf("a bad owner: %v", err)
	}
}

// The board's GitHub repository, for the numbers below the counter's
// start: the one github.com table's; two, or a GitHub Enterprise one
// alone, name none.
func TestGitHubRepoOneTable(t *testing.T) {
	t.Parallel()
	gh := func(host, project string) forge.Table {
		return forge.Table{Kind: "github", Host: host, Project: project}
	}
	for _, c := range []struct {
		list []forge.Table
		want string
	}{
		{nil, ""},
		{[]forge.Table{gh("github.com", "o/a"), {Kind: "gitlab", Host: "gitlab.com", Project: "9"}}, "o/a"},
		{[]forge.Table{gh("github.com", "o/a"), gh("github.com", "O/A")}, "o/a"},
		{[]forge.Table{gh("github.com", "o/a"), gh("github.com", "o/b")}, ""},
		{[]forge.Table{gh("ghe.example.com", "o/a")}, ""},
	} {
		var cfg Config
		cfg.Forges.List = c.list
		if got := cfg.gitHubRepo(); got != c.want {
			t.Errorf("%+v: %q, want %q", c.list, got, c.want)
		}
	}
}
