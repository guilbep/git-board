package cycle

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio"
	"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=dev", "-c", "user.email=dev@example.com", "-c", "commit.gpgsign=false"}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1")
	if date := os.Getenv("TEST_GIT_DATE"); date != "" {
		cmd.Env = append(cmd.Env, "GIT_AUTHOR_DATE="+date, "GIT_COMMITTER_DATE="+date)
	}
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
	git(t, dir, "add", path)
	git(t, dir, "commit", "-q", "-m", "add "+path)
	return git(t, dir, "rev-parse", "HEAD")
}

// repo is a repository on dev with a git-bug user identity, no login.
func repo(t *testing.T) (string, *gitbugio.Backend) {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "dev")
	git(t, dir, "config", "user.name", "dev")
	git(t, dir, "config", "user.email", "dev@example.com")
	git(t, dir, "commit", "-q", "--allow-empty", "-m", "start")
	b, err := gitbugio.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	who, err := b.TestCache().Identities().NewFromGitUser()
	if err != nil {
		t.Fatal(err)
	}
	if err := b.TestCache().SetUserIdentity(who); err != nil {
		t.Fatal(err)
	}
	return dir, b
}

func co2Config() *config.Config {
	return &config.Config{
		Fields:   map[string]config.Field{"status": {Options: co2Options}},
		Branches: config.Branches{Base: "dev"},
	}
}

const planDoc = "---\nissue: 2518\nstatus: %s\n---\n# Plan\n"

func TestGatherFromRepo(t *testing.T) {
	t.Parallel()
	dir, b := repo(t)
	plan := "docs/src/implementation-plans/2518-x.md"
	commitFile(t, dir, plan, strings.Replace(planDoc, "%s", "accepted", 1))
	git(t, dir, "switch", "-q", "-c", "fix/2518-x")
	first := commitFile(t, dir, "api/a_test.go", "package api\n")
	commitFile(t, dir, "api/a.go", "package api\n")
	git(t, dir, "switch", "-q", "-c", "plan/2518-x", "dev")
	commitFile(t, dir, plan, strings.Replace(planDoc, "%s", "draft", 1))
	git(t, dir, "switch", "-q", "dev")

	id := gitbugiotest.NewImportedIssue(t, b, "Crash on save (#2518)", "https://github.com/o/r/issues/2518")
	if _, err := b.ApplyLabelChange(id, []string{"status:in-progress", "risk:normal", "assignee:erin"}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := b.AddCommentMeta(id, "Plan approved", map[string]string{"plan:approve": "1", "plan:commit": "abc1234"}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.AddCommentMeta(id, "Run at abc1234", map[string]string{"verify:run": "1", "verify:commit": "abc1234"}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.AddCommentMeta(id, "Checked: one", map[string]string{"verify:criterion": "1", "verify:commit": "abc1234"}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.AddCommentMeta(id, "Signed off qa", map[string]string{"signoff:status": "qa", "signoff:commit": "abc1234", "signoff:why": "by hand"}); err != nil {
		t.Fatal(err)
	}
	th, err := b.Thread(id)
	if err != nil {
		t.Fatal(err)
	}
	// the description: NewImportedIssue leaves it empty
	th.Events[0].Body = "## Acceptance criteria\n- one\n- two\n"

	cfg := co2Config()
	cfg.Roles = map[string]string{"lead-dev": "bob"}
	cfg.Cycle = config.Cycle{Plan: "lead-dev"}
	in, err := Gather(b, cfg, th, "bob", nil)
	if err != nil {
		t.Fatal(err)
	}
	if in.Status != "in-progress" || in.Risk != "normal" || len(in.Criteria) != 2 || !slices.Equal(in.Assignees, []string{"erin"}) || in.Me != "bob" {
		t.Errorf("issue: %+v", in)
	}
	if in.Branch != "fix/2518-x" || in.PlanBranch != "plan/2518-x" || in.Base != "dev" || in.Head != git(t, dir, "rev-parse", "fix/2518-x") || in.PlanHead != git(t, dir, "rev-parse", "plan/2518-x") {
		t.Errorf("branches: branch %q plan %q base %q head %q plan head %q", in.Branch, in.PlanBranch, in.Base, in.Head, in.PlanHead)
	}
	if !slices.Equal(in.Tests, []TestFile{{Path: "api/a_test.go", Commit: first}}) {
		t.Errorf("Tests = %+v", in.Tests)
	}
	if in.Plan == nil || in.Plan.Path != plan || in.Plan.Ref != "fix/2518-x" || in.Plan.Status != "accepted" || in.Plan.Issue != "2518" {
		t.Errorf("Plan = %+v", in.Plan)
	}
	// rendered below its front matter; added on dev, so nobody drafted it on the branch
	if in.Plan.HTML != "<h1>Plan</h1>\n" || in.Plan.By != "" {
		t.Errorf("Plan HTML %q, by %q", in.Plan.HTML, in.Plan.By)
	}
	if in.People["plan"] != "bob" || in.People["risk"] != "" {
		t.Errorf("People = %v", in.People)
	}
	// written by the identity, which has no login: the viewer's own
	if in.PlanApproval == nil || in.PlanApproval.By != "bob" || in.PlanApproval.Commit != "abc1234" {
		t.Errorf("PlanApproval = %+v", in.PlanApproval)
	}
	if len(in.Runs) != 1 || in.Runs[0].Commit != "abc1234" || len(in.Checks) != 1 || in.Checks[0].Index != 1 {
		t.Errorf("Runs %+v Checks %+v", in.Runs, in.Checks)
	}
	if len(in.Signoffs) != 1 || in.Signoffs[0].Status != "qa" || in.Signoffs[0].Why != "by hand" || in.Signoffs[0].By != "bob" {
		t.Errorf("Signoffs = %+v", in.Signoffs)
	}

	// another viewer: the identity's approval isn't the plan role's
	if in, err = Gather(b, cfg, th, "carol", nil); err != nil || in.PlanApproval != nil {
		t.Errorf("carol: PlanApproval %+v, %v", in.PlanApproval, err)
	}
}

func TestEvidenceBranch(t *testing.T) {
	dir, b := repo(t)
	t.Setenv("TEST_GIT_DATE", "2026-01-01T10:00:00Z")
	git(t, dir, "switch", "-q", "-c", "2518-old")
	commitFile(t, dir, "old.go", "package x\n")
	t.Setenv("TEST_GIT_DATE", "2026-01-03T10:00:00Z")
	git(t, dir, "switch", "-q", "-c", "plan/2518-p", "dev")
	commitFile(t, dir, "plan.md", "plan\n")
	t.Setenv("TEST_GIT_DATE", "2026-01-02T10:00:00Z")
	git(t, dir, "switch", "-q", "-c", "fix/2518-new", "dev")
	commitFile(t, dir, "new.go", "package x\n")
	git(t, dir, "switch", "-q", "dev")

	id := gitbugiotest.NewImportedIssue(t, b, "Crash (#2518)", "https://github.com/o/r/issues/2518")
	th, err := b.Thread(id)
	if err != nil {
		t.Fatal(err)
	}
	in, err := Gather(b, co2Config(), th, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if in.Branch != "fix/2518-new" || in.PlanBranch != "plan/2518-p" {
		t.Errorf("branch %q, plan branch %q", in.Branch, in.PlanBranch)
	}
}

func TestMatchTest(t *testing.T) {
	t.Parallel()
	for path, want := range map[string]bool{
		"internal/x/y_test.go": true, "web/src/a.test.ts": true, "tests/unit/t.py": true,
		"test_x.py": true, "src/tests.ts": false, "web/src/a.ts": false,
	} {
		matched := false
		for _, pat := range (&config.Config{}).Stages().TestFiles {
			matched = matched || matchTest(pat, path)
		}
		if matched != want {
			t.Errorf("%s: matched %v, want %v", path, matched, want)
		}
	}
}

// A plan drafted on its plan branch says who first committed it there.
func TestGatherPlanDraftedBy(t *testing.T) {
	t.Parallel()
	dir, b := repo(t)
	git(t, dir, "switch", "-q", "-c", "plan/2518-x", "dev")
	commitFile(t, dir, "docs/src/implementation-plans/2518-x.md", strings.Replace(planDoc, "%s", "draft", 1))
	git(t, dir, "switch", "-q", "dev")
	id := gitbugiotest.NewImportedIssue(t, b, "Crash on save (#2518)", "https://github.com/o/r/issues/2518")
	th, err := b.Thread(id)
	if err != nil {
		t.Fatal(err)
	}
	in, err := Gather(b, co2Config(), th, "", nil)
	if err != nil || in.Plan == nil || in.Plan.Ref != "plan/2518-x" || in.Plan.By != "dev" || in.Plan.Status != "draft" {
		t.Errorf("Plan = %+v, %v", in.Plan, err)
	}
}

// A plan file whose issue: is PlanIssue, as the agent's prompt asks, is
// found: with a forge number, and with none (the git-bug id).
func TestGatherPlanIssue(t *testing.T) {
	t.Parallel()
	dir, b := repo(t)
	numbered := gitbugiotest.NewImportedIssue(t, b, "Crash on save (#2518)", "https://github.com/o/r/issues/2518")
	bare, err := b.NewIssue("Export CSV", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{numbered, bare} {
		th, err := b.Thread(id)
		if err != nil {
			t.Fatal(err)
		}
		key := PlanIssue(th.Issue)
		git(t, dir, "switch", "-q", "-c", "plan/"+key+"-x", "dev")
		commitFile(t, dir, "docs/src/implementation-plans/"+key+"-x.md", "---\nissue: "+key+"\nstatus: draft\n---\n# Plan\n")
		git(t, dir, "switch", "-q", "dev")
		in, err := Gather(b, co2Config(), th, "", nil)
		if err != nil || in.Plan == nil || in.Plan.Status != "draft" {
			t.Errorf("issue: %s: Plan = %+v, %v", key, in.Plan, err)
		}
	}
	if th, _ := b.Thread(numbered); PlanIssue(th.Issue) != "2518" {
		t.Errorf("PlanIssue = %q, want 2518", PlanIssue(th.Issue))
	}
}

func TestGatherNoBranch(t *testing.T) {
	t.Parallel()
	b := gitbugiotest.New(t)
	id, err := b.NewIssue("Export CSV", "", []string{"status:ready"})
	if err != nil {
		t.Fatal(err)
	}
	th, err := b.Thread(id)
	if err != nil {
		t.Fatal(err)
	}
	in, err := Gather(b, co2Config(), th, "", nil)
	if err != nil || in.Branch != "" || in.Tests != nil || in.Plan != nil || in.Status != "ready" {
		t.Errorf("Gather = %+v, %v", in, err)
	}
}
