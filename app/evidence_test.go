package app

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/guilbep/git-board/domain/cycle"
	"github.com/guilbep/git-board/domain/issues/issuesync"
	"github.com/guilbep/git-board/kernel"
	bsync "github.com/guilbep/git-board/sync"
)

func gitT(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=dev", "-c", "user.email=dev@example.com", "-c", "commit.gpgsign=false"}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func commitFile(t *testing.T, dir, path, content string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, path)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, path), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	gitT(t, dir, "add", path)
	gitT(t, dir, "commit", "-q", "-m", "add "+path)
	return gitT(t, dir, "rev-parse", "HEAD")
}

// co2Board is a repository on dev, with co2-calculator's statuses and
// the cycle's plan role held by bob, and a person you write as, no login.
func co2Board(t *testing.T) (string, *Board) {
	t.Helper()
	dir := t.TempDir()
	gitT(t, dir, "init", "-q", "-b", "dev")
	gitT(t, dir, "commit", "-q", "--allow-empty", "-m", "start")
	toml := `[fields.status]
single = true
options = ["backlog", "comite", "ready", "in-progress", "in-review", "ready-for-validation", "qa", "tests-metier", "validated"]

[branches]
base = "dev"

[roles]
lead-dev = "bob"

[cycle]
plan = "lead-dev"
`
	if err := os.WriteFile(filepath.Join(dir, ".git-board.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := Open(t.Context(), dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b.Close() })
	me, err := b.People.New(t.Context(), "dev", "dev@example.com")
	if err != nil {
		t.Fatal(err)
	}
	b.Issues.Me = me.ID
	return dir, b
}

// forgeIssue makes an issue linked as GitHub's o/r#n, with these
// operations after its create; its id.
func forgeIssue(t *testing.T, b *Board, n int, title string, ops ...kernel.Op) string {
	t.Helper()
	me := string(b.Issues.Me)
	is, err := issuesync.CreateIssue(t.Context(), b.Kernel, append([]kernel.Op{issuesync.NewCreate(me, title, "")}, ops...)...)
	if err != nil {
		t.Fatal(err)
	}
	u := fmt.Sprintf("https://github.com/o/r/issues/%d", n)
	data, _ := json.Marshal(map[string]string{"issue": is.ID, "forge_id": u, "number": fmt.Sprint(n), "url": u})
	if _, err := bsync.Kind.Create(t.Context(), b.Kernel, bsync.StateID("github.com/o/r"), kernel.Op{Kind: "link", Author: me, Time: time.Now().Unix(), Data: data}); err != nil {
		t.Fatal(err)
	}
	return is.ID
}

const planDoc = "---\nissue: 2518\nstatus: %s\n---\n# Plan\n"

// The evidence is read from the board and git: the issue's fields and
// people, its plan, its branches as lanes, the tests their diffs add,
// its comments' metadata.
func TestEvidence(t *testing.T) {
	t.Parallel()
	dir, b := co2Board(t)
	plan := "docs/src/implementation-plans/2518-x.md"
	commitFile(t, dir, plan, strings.Replace(planDoc, "%s", "accepted", 1))
	gitT(t, dir, "switch", "-q", "-c", "fix/2518-x")
	first := commitFile(t, dir, "api/a_test.go", "package api\n")
	commitFile(t, dir, "api/a.go", "package api\n")
	gitT(t, dir, "switch", "-q", "-c", "plan/2518-x", "dev")
	commitFile(t, dir, plan, strings.Replace(planDoc, "%s", "draft", 1))
	gitT(t, dir, "switch", "-q", "dev")

	me := string(b.Issues.Me)
	id := forgeIssue(t, b, 2518, "Crash on save (#2518)",
		issuesync.ChangeLabels(me, []string{"status:in-progress", "risk:normal", "assignee:erin"}, nil),
		issuesync.NewCommentMeta(me, "Plan approved", map[string]string{"plan:approve": "1", "plan:commit": "abc1234"}),
		issuesync.NewCommentMeta(me, "Run at abc1234", map[string]string{"verify:run": "1", "verify:commit": "abc1234"}),
		issuesync.NewCommentMeta(me, "Checked: one", map[string]string{"verify:criterion": "1", "verify:commit": "abc1234"}),
		issuesync.NewCommentMeta(me, "Signed off qa", map[string]string{"signoff:status": "qa", "signoff:commit": "abc1234", "signoff:why": "by hand"}))
	th, err := b.Issues.Show(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	th.Events[0].Body = "## Acceptance criteria\n- one\n- two\n"
	in, err := b.Evidence(t.Context(), th, "bob", nil, func(string) string { return "<h1>Plan</h1>\n" })
	if err != nil {
		t.Fatal(err)
	}
	if in.Status != "in-progress" || in.Risk != "normal" || len(in.Criteria) != 2 || !slices.Equal(in.Assignees, []string{"erin"}) || in.Me != "bob" {
		t.Errorf("issue: %+v", in)
	}
	if in.Branch != "fix/2518-x" || in.PlanBranch != "plan/2518-x" || in.Base != "dev" || in.Head != gitT(t, dir, "rev-parse", "fix/2518-x") || in.PlanHead != gitT(t, dir, "rev-parse", "plan/2518-x") {
		t.Errorf("branches: branch %q plan %q base %q head %q plan head %q", in.Branch, in.PlanBranch, in.Base, in.Head, in.PlanHead)
	}
	if !slices.Equal(in.Tests, []cycle.TestFile{{Path: "api/a_test.go", Commit: first}}) {
		t.Errorf("Tests = %+v", in.Tests)
	}
	if in.Plan == nil || in.Plan.Path != plan || in.Plan.Ref != "fix/2518-x" || in.Plan.Status != "accepted" || in.Plan.Issue != "2518" || in.Plan.HTML != "<h1>Plan</h1>\n" || in.Plan.By != "" {
		t.Errorf("Plan = %+v", in.Plan)
	}
	if in.People["plan"] != "bob" || in.People["risk"] != "" {
		t.Errorf("People = %v", in.People)
	}
	// written by the person, who has no login: the viewer's own
	if in.PlanApproval == nil || in.PlanApproval.By != "bob" || in.PlanApproval.Commit != "abc1234" {
		t.Errorf("PlanApproval = %+v", in.PlanApproval)
	}
	if len(in.Runs) != 1 || len(in.Checks) != 1 || len(in.Signoffs) != 1 || in.Signoffs[0].By != "bob" {
		t.Errorf("Runs %+v Checks %+v Signoffs %+v", in.Runs, in.Checks, in.Signoffs)
	}
	// another viewer: the person's approval isn't the plan role's
	if in, err = b.Evidence(t.Context(), th, "carol", nil, nil); err != nil || in.PlanApproval != nil {
		t.Errorf("carol: PlanApproval %+v, %v", in.PlanApproval, err)
	}
}
