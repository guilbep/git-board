package cli

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/guilbep/git-board/app"
	"github.com/guilbep/git-board/forge"
	"github.com/guilbep/git-board/forge/fake"
)

// lint --create-labels, from v1's cmd/git-board/lint_test.go: its output
// and exit codes. The forges are fakes, by app.OpenLabeler, a package
// variable: the tests are serial, and chdir (-C).

const createLabelsConfig = `
[repos]
list = ["EPFL-ENAC/co2-calculator"]

[fields.status]
single = true
options = ["backlog", "ready"]

[fields.prio]
single = true
options = ["p0"]
`

// labelForges swaps app.OpenLabeler for fakes, one per host/project,
// each made with these labels; asked collects the projects opened.
func labelForges(t *testing.T, labels ...string) (forges map[string]*fake.Forge, asked *[]string) {
	t.Helper()
	forges, asked = map[string]*fake.Forge{}, &[]string{}
	was := app.OpenLabeler
	t.Cleanup(func() { app.OpenLabeler = was })
	app.OpenLabeler = func(_ context.Context, f forge.Table) (forge.Labeler, error) {
		key := f.Host + "/" + f.Project
		*asked = append(*asked, key)
		if forges[key] == nil {
			forges[key] = fake.New(key)
			for _, l := range labels {
				forges[key].AddLabel(forge.Label{Name: l})
			}
		}
		return forges[key], nil
	}
	return forges, asked
}

// lintBoard is newBoard with this .git-board.toml, the current directory.
func lintBoard(t *testing.T, config string) string {
	t.Helper()
	dir, _ := newBoard(t)
	t.Chdir(dir)
	if err := os.WriteFile(filepath.Join(dir, ".git-board.toml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestLintCreateLabelsPreviewsMissingLabelsWithoutWriting(t *testing.T) {
	dir := lintBoard(t, createLabelsConfig)
	forges, _ := labelForges(t, "status:backlog") // status:ready and prio:p0 are missing
	out, errs, code := run(t, "-C", dir, "lint", "--create-labels")
	if code != 0 {
		t.Fatalf("exit %d: %s\n%s", code, errs, out)
	}
	if w := forges["github.com/EPFL-ENAC/co2-calculator"].Writes(); len(w) != 0 {
		t.Errorf("preview (no --apply) wrote %v", w)
	}
	for _, want := range []string{
		"EPFL-ENAC/co2-calculator: missing prio:p0 #e00000\n",
		"EPFL-ENAC/co2-calculator: missing status:ready #00897f\n",
		"\ndry run — nothing written. Re-run with --apply to create and update these labels.\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("no %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "status:backlog") {
		t.Errorf("a label there is reported missing:\n%s", out)
	}
}

func TestLintCreateLabelsApplyCreatesMissingLabels(t *testing.T) {
	dir := lintBoard(t, createLabelsConfig)
	forges, _ := labelForges(t, "status:backlog")
	out, errs, code := run(t, "-C", dir, "lint", "--create-labels", "--apply")
	if code != 0 {
		t.Fatalf("exit %d: %s\n%s", code, errs, out)
	}
	f := forges["github.com/EPFL-ENAC/co2-calculator"]
	var made []string
	for _, w := range f.Writes() {
		made = append(made, w.Op+" "+strings.Join(w.Args, " "))
	}
	if !slices.Equal(made, []string{"create-label prio:p0 e00000 ", "create-label status:ready 00897f "}) {
		t.Errorf("writes %q", made)
	}
	if !strings.Contains(out, "created prio:p0") || !strings.Contains(out, "created status:ready") || strings.Contains(out, "dry run") {
		t.Errorf("output:\n%s", out)
	}
	// again: nothing left to do
	if out, _, code := run(t, "-C", dir, "lint", "--create-labels", "--apply"); code != 0 || !strings.Contains(out, "EPFL-ENAC/co2-calculator: all labels present\n") {
		t.Errorf("exit %d, output:\n%s", code, out)
	}
}

// An existing label is updated where the file sets its color or
// description, and only there.
func TestLintCreateLabelsUpdatesWhatTheFileSets(t *testing.T) {
	dir := lintBoard(t, createLabelsConfig+"\n[fields.prio.colors]\np0 = \"#b60205\"\n[fields.prio.descriptions]\np0 = \"Now\"\n")
	forges, _ := labelForges(t)
	f := fake.New("github.com/EPFL-ENAC/co2-calculator")
	forges["github.com/EPFL-ENAC/co2-calculator"] = f
	for _, l := range []forge.Label{{Name: "status:backlog", Color: "ededed"}, {Name: "status:ready", Color: "ededed"}, {Name: "prio:p0", Color: "d73a4a", Description: "Urgent"}} {
		f.AddLabel(l)
	}
	out, errs, code := run(t, "-C", dir, "lint", "--create-labels")
	if code != 0 || out != "EPFL-ENAC/co2-calculator: update prio:p0: color d73a4a → b60205, description \"Urgent\" → \"Now\"\n\ndry run — nothing written. Re-run with --apply to create and update these labels.\n" {
		t.Fatalf("exit %d: %s\n%s", code, errs, out)
	}
	out, _, code = run(t, "-C", dir, "lint", "--create-labels", "--apply")
	got, _ := f.Labels(t.Context())
	if code != 0 || !strings.Contains(out, "EPFL-ENAC/co2-calculator: updated prio:p0: color d73a4a → b60205") ||
		!slices.Contains(got, forge.Label{Name: "prio:p0", Color: "b60205", Description: "Now"}) {
		t.Errorf("exit %d, labels %+v, output:\n%s", code, got, out)
	}
}

func TestDescribeChange(t *testing.T) {
	t.Parallel()
	was := forge.Label{Name: "bug", Color: "d73a4a", Description: "Broken"}
	for _, c := range []struct {
		change app.LabelChange
		want   string
	}{
		{app.LabelChange{Label: forge.Label{Name: "feat", Color: "0e8a16", Description: "A new feature"}}, `missing feat #0e8a16 "A new feature"`},
		{app.LabelChange{Label: forge.Label{Name: "bug", Color: "b60205", Description: "Broken"}, Update: true, Was: &was}, "update bug: color d73a4a → b60205"},
		{app.LabelChange{Label: forge.Label{Name: "bug", Color: "d73a4a", Description: "It breaks"}, Update: true, Was: &was}, `update bug: description "Broken" → "It breaks"`},
	} {
		if got := describeChange(c.change); got != c.want {
			t.Errorf("describeChange = %s, want %s", got, c.want)
		}
	}
}

// Without [repos] or [forges], --create-labels plans for the forge
// origin names; with none, it says so, exit 1.
func TestLintCreateLabelsFallsBackToOrigin(t *testing.T) {
	dir := lintBoard(t, "[fields.prio]\nsingle = true\noptions = [\"p0\"]\n")
	_, asked := labelForges(t)
	out, errs, code := run(t, "-C", dir, "lint", "--create-labels")
	if code != 1 || !strings.Contains(errs, "error: no GitHub repository: set [repos] in .git-board.toml, or an origin on GitHub") || len(*asked) != 0 {
		t.Errorf("no repository at all: exit %d, asked %v: %s\n%s", code, *asked, errs, out)
	}
	gitIn(t, dir, "remote", "add", "origin", "git@github.com:o/r.git")
	out, errs, code = run(t, "-C", dir, "lint", "--create-labels")
	if code != 0 || !slices.Equal(*asked, []string{"github.com/o/r"}) || !strings.Contains(out, "o/r: missing prio:p0") {
		t.Errorf("exit %d, asked %v: %s\n%s", code, *asked, errs, out)
	}
}

// A [forges] table is planned for too, GitLab's as GitHub's: a field
// that lives elsewhere on that forge has no label made there.
func TestLintCreateLabelsOnAGitLabForge(t *testing.T) {
	dir := lintBoard(t, createLabelsConfig+"\n[forges.gl]\nkind = \"gitlab\"\nhost = \"gitlab.example.com\"\nproject = \"42\"\nsync = true\n\n[forges.gl.fields.status]\nto = \"none\"\n")
	forges, asked := labelForges(t)
	out, errs, code := run(t, "-C", dir, "lint", "--create-labels", "--apply")
	if code != 0 || !slices.Equal(*asked, []string{"github.com/EPFL-ENAC/co2-calculator", "gitlab.example.com/42"}) {
		t.Fatalf("exit %d, asked %v: %s\n%s", code, *asked, errs, out)
	}
	got, _ := forges["gitlab.example.com/42"].Labels(t.Context())
	if !slices.Equal(got, []forge.Label{{Name: "prio:p0", Color: "e00000"}}) || !strings.Contains(out, "42: created prio:p0 #e00000\n") {
		t.Errorf("GitLab's labels %+v, output:\n%s", got, out)
	}
}

// A forge that fails is told on its line, the others planned for all
// the same, and the run exits 1.
func TestLintCreateLabelsAFailedForge(t *testing.T) {
	dir := lintBoard(t, "[repos]\nlist = [\"o/down\", \"o/up\"]\n\n[fields.prio]\noptions = [\"p0\"]\n")
	labelForges(t)
	open := app.OpenLabeler
	app.OpenLabeler = func(ctx context.Context, f forge.Table) (forge.Labeler, error) {
		if f.Project == "o/down" {
			return nil, errors.New("github: no token for \"\" on github.com")
		}
		return open(ctx, f)
	}
	out, errs, code := run(t, "-C", dir, "lint", "--create-labels")
	if code != 1 || !strings.Contains(errs, "error: one or more repos failed") ||
		!strings.Contains(out, "o/down: github: no token for \"\" on github.com\n") || !strings.Contains(out, "o/up: missing prio:p0") {
		t.Errorf("exit %d: %s\n%s", code, errs, out)
	}
}

func TestLintApplyWithoutCreateLabelsErrors(t *testing.T) {
	dir := lintBoard(t, createLabelsConfig)
	if out, errs, code := run(t, "-C", dir, "lint", "--apply"); code != 1 || !strings.Contains(errs, "--apply only applies together with --create-labels") {
		t.Errorf("exit %d: %s\n%s", code, errs, out)
	}
}

func TestLintCreateLabelsReportsTemplateLabelsTheRepoLacks(t *testing.T) {
	dir := lintBoard(t, createLabelsConfig)
	gitIn(t, dir, "remote", "add", "origin", "https://github.com/EPFL-ENAC/co2-calculator.git")
	templates := map[string]string{
		"feature.yaml": "name: Feature\ntitle: \"[FEAT]() \"\nlabels: [\"enhancement\", \"needs-triage\"]\nbody: []\n",
		"bug.yml":      "name: Bug\nlabels:\n  - bug\n",
		"task.md":      "---\nname: Task\nlabels: Feat\n---\nWhat to do\n", // GitHub's names ignore case
		"config.yml":   "blank_issues_enabled: false\n",                    // the chooser, not a template
	}
	if err := os.MkdirAll(filepath.Join(dir, ".github", "ISSUE_TEMPLATE"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range templates {
		if err := os.WriteFile(filepath.Join(dir, ".github", "ISSUE_TEMPLATE", name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	forges, asked := labelForges(t, "status:backlog", "status:ready", "prio:p0", "bug", "feat")
	out, errs, code := run(t, "-C", dir, "lint", "--create-labels", "--apply")
	if code != 0 {
		t.Fatalf("exit %d: %s\n%s", code, errs, out)
	}
	if !strings.Contains(out, "issue templates: feature.yaml applies enhancement, needs-triage, which EPFL-ENAC/co2-calculator doesn't have: GitHub drops them\n") {
		t.Errorf("feature.yaml's missing labels not reported:\n%s", out)
	}
	if strings.Contains(out, "bug.yml") || strings.Contains(out, "task.md") || strings.Contains(out, "config.yml") {
		t.Errorf("only feature.yaml lacks labels:\n%s", out)
	}
	if w := forges["github.com/EPFL-ENAC/co2-calculator"].Writes(); len(w) != 0 {
		t.Errorf("--apply wrote %v: a template's label is fixed in the template, not made", w)
	}
	if len(*asked) != 1 {
		t.Errorf("origin's labels read again: %v", *asked)
	}
}

// The issue templates' check is a warning: when origin isn't on GitHub,
// or a template doesn't parse, every label can still exist, and CI must
// not fail for it.
func TestLintCreateLabelsTemplateCheckOnlyWarns(t *testing.T) {
	dir := lintBoard(t, createLabelsConfig)
	if err := os.MkdirAll(filepath.Join(dir, ".github", "ISSUE_TEMPLATE"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".github", "ISSUE_TEMPLATE", "bug.yml"), []byte("name: Bug\nlabels: [bug]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	labelForges(t, "status:backlog", "status:ready", "prio:p0", "bug")
	out, errs, code := run(t, "-C", dir, "lint", "--create-labels")
	if code != 0 || !strings.Contains(out, "issue templates: not checked: no origin remote\n") {
		t.Errorf("no origin: exit %d: %s\n%s", code, errs, out)
	}
	gitIn(t, dir, "remote", "add", "origin", "https://gitlab.example.com/g/p.git")
	if out, _, code := run(t, "-C", dir, "lint", "--create-labels"); code != 0 || !strings.Contains(out, "issue templates: not checked: origin is no GitHub repository: https://gitlab.example.com/g/p.git\n") {
		t.Errorf("origin on GitLab: exit %d\n%s", code, out)
	}
	gitIn(t, dir, "remote", "set-url", "origin", "https://github.com/o/r.git")
	if out, _, code := run(t, "-C", dir, "lint", "--create-labels"); code != 0 || strings.Contains(out, "issue templates") {
		t.Errorf("every template's label there: exit %d\n%s", code, out)
	}
	if err := os.WriteFile(filepath.Join(dir, ".github", "ISSUE_TEMPLATE", "broken.yml"), []byte("labels: [bug\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, _, code := run(t, "-C", dir, "lint", "--create-labels"); code != 0 || !strings.Contains(out, "issue templates: not checked: broken.yml") {
		t.Errorf("a template that doesn't parse: exit %d\n%s", code, out)
	}
}
