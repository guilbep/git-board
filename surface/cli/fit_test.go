package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/guilbep/git-board/app"
	"github.com/guilbep/git-board/domain/people"
	"github.com/guilbep/git-board/forge"
	"github.com/guilbep/git-board/forge/fake"
	"github.com/guilbep/git-board/tools/ghproject"
	"github.com/guilbep/git-board/tools/ghproject/projectfake"
)

// The co2 case, end to end (docs/prd/config-fit.md, The test): a team
// with a repository, its issues and a Project of nine statuses, six
// Categories and three dated iterations starts from the default file.
// The check proposes twelve changes; fit writes them once the six
// questions are answered; nothing the file had is gone; the next check
// warns of nothing. It swaps app.GitHubAPI, sets GITHUB_TOKEN and chdirs: serial.
func TestTheCo2Fit(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	t.Chdir(dir)
	file := filepath.Join(dir, ".git-board.toml")

	// the forge: 80 issues with a Category on the Project and no kind, one
	// on each of two statuses the file lacks, and one off the Project that
	// carries the third
	categories := []struct {
		name string
		n    int
	}{{"SPECS", 27}, {"TASK", 18}, {"FIX", 14}, {"DATA", 13}, {"DOC", 5}, {"TEXT", 3}}
	b, err := app.Open(ctx, dir, app.Options{})
	if err != nil {
		t.Fatal(err)
	}
	me, err := people.Kind.Create(ctx, b.Kernel, "", op("create", 1700000000, "", `{"name":"me","email":""}`))
	if err != nil {
		t.Fatal(err)
	}
	b.Close()
	if out, err := exec.Command("git", "-C", dir, "config", "board.me", string(me.ID)).CombinedOutput(); err != nil {
		t.Fatalf("git config: %v\n%s", err, out)
	}
	f := fake.New("github.com/o/r")
	author := forge.Account{ID: "7", Login: "dev"}
	for i := 1; i <= 80; i++ {
		f.Add(forge.Issue{Author: author, Title: fmt.Sprintf("Issue %d", i)})
	}
	f.Add(forge.Issue{Author: author, Title: "On tests metier", Labels: []string{"status:tests-metier"}})
	f.Add(forge.Issue{Author: author, Title: "On comite", Labels: []string{"status:comite"}})
	f.Add(forge.Issue{Author: author, Title: "Off the Project", Labels: []string{"status:qa"}})
	opened := app.OpenForge
	app.OpenForge = func(context.Context, forge.Table) (forge.Forge, error) { return f, nil }
	t.Cleanup(func() { app.OpenForge = opened })
	forges := app.DefaultConfig + "\n[forges.gh]\nkind = \"github\"\nproject = \"o/r\"\nsync = true\n"
	old := forges + "\n[github-project]\nowner = \"o\"\nnumber = 1\n"
	if err := os.WriteFile(file, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}

	// the Project
	p := projectfake.New("o", 1)
	statuses := []string{"Backlog", "Comité Spécialisé", "Ready", "In progress", "In review", "Ready for validation", "QA Validation", "Tests Metier", "Validated"}
	choices := func(names []string) (cs []projectfake.Choice) {
		for _, n := range names {
			cs = append(cs, projectfake.Choice{ID: "o-" + n, Name: n})
		}
		return cs
	}
	p.AddField(projectfake.Field{ID: "F-status", Name: "Status", DataType: "SINGLE_SELECT", Choices: choices(statuses)})
	p.AddField(projectfake.Field{ID: "F-category", Name: "Category", DataType: "SINGLE_SELECT", Choices: choices([]string{"SPECS", "TASK", "FIX", "DATA", "DOC", "TEXT"})})
	// GitHub lists the iterations to come before the completed ones
	p.AddField(projectfake.Field{ID: "F-sprint", Name: "Sprint", DataType: "ITERATION", Choices: []projectfake.Choice{
		{ID: "i-3", Name: "Sprint 3", Start: "2026-11-30", Days: 14},
		{ID: "i-1", Name: "Sprint 1", Start: "2026-11-02", Days: 14},
		{ID: "i-2", Name: "Sprint 2", Start: "2026-11-16", Days: 14}}})
	n := 0
	add := func(values map[string]string) {
		n++
		p.Add(projectfake.Item{Number: n, URL: fmt.Sprintf("https://github.com/o/r/issues/%d", n), Title: fmt.Sprintf("Issue %d", n), Values: values})
	}
	for _, c := range categories {
		for range c.n {
			add(map[string]string{"Status": "Backlog", "Category": c.name})
		}
	}
	add(map[string]string{"Status": "Tests Metier"})
	add(map[string]string{"Status": "Comité Spécialisé"})
	srv := p.Server(t)
	was := app.GitHubAPI
	app.GitHubAPI = srv.URL
	t.Cleanup(func() { app.GitHubAPI = was })
	t.Setenv("GITHUB_TOKEN", "t")
	proposals := func(r ghproject.Report) (got []string) {
		for _, p := range r.Fit {
			l := fmt.Sprintf("%s -> ? clears %d", p.ID, p.Clears)
			if p.Ticked {
				l = fmt.Sprintf("%s -> %s %s clears %d", p.ID, p.Do, p.Value, p.Clears)
			}
			got = append(got, l)
		}
		return got
	}
	want := []string{
		"status:Comité Spécialisé -> ? clears 1",
		"status:QA Validation -> ? clears 0",
		"status:Tests Metier -> option tests-metier clears 1",
		"category:DATA -> ? clears 13",
		"category:DOC -> alias docs clears 5",
		"category:FIX -> alias bug clears 14",
		"category:SPECS -> ? clears 27",
		"category:TASK -> ? clears 18",
		"category:TEXT -> ? clears 3",
		"sprint:Sprint 1 -> sprint sprint-1 clears 0",
		"sprint:Sprint 2 -> sprint sprint-2 clears 0",
		"sprint:Sprint 3 -> sprint sprint-3 clears 0",
	}

	// a new team's first check, of a board with no issue yet: each part
	// counted, and the twelve proposals with the warnings the pull brings.
	// No label says a status here, so each option takes the Project's name.
	out, errs, code := run(t, "-C", dir, "sync", "--check")
	for _, want := range []string{"parts: 0 pull-changes, 83 pull-new, 0 create, 0 send", "config: 12 changes clear 82 warnings: git board project fit", "checked: nothing written"} {
		if code != 0 || !strings.Contains(out, want) {
			t.Fatalf("the first check: %d, want %q in\n%s\n%s", code, want, out[max(0, len(out)-600):], errs)
		}
	}
	if b, err = app.Open(ctx, dir, app.Options{Who: true}); err != nil {
		t.Fatal(err)
	}
	r, err := b.ProjectRun(ctx, ghproject.Both, false, 0)
	if err != nil {
		t.Fatal(err)
	}
	fresh := slices.Clone(want)
	fresh[0], fresh[1] = "status:Comité Spécialisé -> option comite-specialise clears 1", "status:QA Validation -> option qa-validation clears 0"
	if got := proposals(r); !slices.Equal(got, fresh) || !r.First || r.Plan.Counts().Matched != 0 {
		t.Fatalf("the proposals of a board with no issue, first %v:\n%s\nwant:\n%s", r.First, strings.Join(got, "\n"), strings.Join(fresh, "\n"))
	}
	b.Close()

	// co2's board is older than its Project's table: its issues came with
	// their status labels. The new issues alone are pulled: nothing goes
	// to the forge, and the next check has none left to pull.
	if err := os.WriteFile(file, []byte(forges), 0o644); err != nil {
		t.Fatal(err)
	}
	out, errs, code = run(t, "-C", dir, "sync", "--only", "pull-new")
	if code != 0 || !strings.Contains(out, "synced") || len(f.Writes()) != 0 {
		t.Fatalf("sync --only pull-new: %d, forge writes %v\n%s\n%s", code, f.Writes(), out[max(0, len(out)-600):], errs)
	}
	out, errs, code = run(t, "-C", dir, "sync", "--check")
	if code != 0 || !strings.Contains(out, "parts: 0 pull-changes, 0 pull-new, 0 create, 0 send") {
		t.Fatalf("the check after the pull: %d\n%s\n%s", code, out[max(0, len(out)-600):], errs)
	}
	if err := os.WriteFile(file, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}

	// the check: 82 warnings, and the line that names the fit
	out, errs, code = run(t, "-C", dir, "project", "sync", "--check")
	for _, want := range []string{"82 warning(s)", "config: 12 changes clear 82 warnings: git board project fit"} {
		if code != 0 || !strings.Contains(out, want) {
			t.Fatalf("check: %d, want %q in\n%s\n%s", code, want, out[max(0, len(out)-600):], errs)
		}
	}

	// the twelve proposals: six ticked, six asked
	if b, err = app.Open(ctx, dir, app.Options{Who: true}); err != nil {
		t.Fatal(err)
	}
	if r, err = b.ProjectRun(ctx, ghproject.Both, false, 0); err != nil {
		t.Fatal(err)
	}
	if got := proposals(r); !slices.Equal(got, want) || !r.First {
		t.Fatalf("the proposals:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if c := r.Fit[0].Choices; !slices.Equal(c, []string{"map=comite-specialise", "map=comite", "map=qa", "unmapped"}) {
		t.Errorf("Comité Spécialisé offers %q", c)
	}

	// fit without --apply: the diff, the six questions, nothing written
	out, errs, code = run(t, "-C", dir, "project", "fit")
	for _, want := range []string{`+aliases = { DOC = "docs", FIX = "bug" }`, "+[[sprints.list]]", "tests-metier",
		"--map 'status:QA Validation=<qa-validation|comite|qa>'", "--add-kind category:DATA", "--unmapped category:SPECS",
		"6 changes clear 20 warnings, 6 question(s) open", "checked: nothing written"} {
		if code != 0 || !strings.Contains(out, want) {
			t.Fatalf("fit: %d, want %q in\n%s\n%s", code, want, out, errs)
		}
	}
	if text, _ := os.ReadFile(file); string(text) != old {
		t.Fatalf("fit without --apply wrote the file:\n%s", text)
	}

	// a file that moved after the check: the write is refused as stale
	answers := map[string]string{"status:Comité Spécialisé": "map=comite", "status:QA Validation": "map=qa",
		"category:TASK": "map=feat", "category:DATA": "add-kind", "category:SPECS": "unmapped", "category:TEXT": "unmapped"}
	old += "\n# the team's own note\n"
	if err := os.WriteFile(file, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Fit(r, nil, answers, false); app.Classify(err) != app.Stale {
		t.Errorf("a fit on a moved file: %v", err)
	}
	if text, _ := os.ReadFile(file); string(text) != old {
		t.Fatalf("the refused fit wrote the file:\n%s", text)
	}
	b.Close()

	// --apply with the six questions answered
	out, errs, code = run(t, "-C", dir, "project", "fit", "--apply",
		"--map", "status:Comité Spécialisé=comite", "--map", "status:QA Validation=qa", "--map", "category:TASK=feat",
		"--add-kind", "category:DATA", "--unmapped", "category:SPECS", "--unmapped", "category:TEXT")
	if code != 0 || !strings.Contains(out, "wrote .git-board.toml: 12 changes clear 82 warnings") {
		t.Fatalf("apply: %d\n%s\n%s", code, out, errs)
	}
	text, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	fitted := string(text)

	// nothing is gone: every line of the old file is in the new one, in
	// order, but the two one-line lists that took a value more
	at := 0
	lines := strings.Split(fitted, "\n")
	for _, l := range strings.Split(old, "\n") {
		if strings.HasPrefix(l, `options = ["backlog"`) || strings.HasPrefix(l, "kinds = ") {
			continue
		}
		i := slices.Index(lines[at:], l)
		if i < 0 {
			t.Fatalf("the line %q is gone, or moved:\n%s", l, fitted)
		}
		at += i + 1
	}
	if b, err = app.Open(ctx, dir, app.Options{Who: true}); err != nil {
		t.Fatalf("the fitted file: %v\n%s", err, fitted)
	}
	defer b.Close()
	s := b.Config.Schema
	status := s.Fields["status"]
	if want := []string{"backlog", "comite", "ready", "in-progress", "in-review", "ready-for-validation", "qa", "tests-metier", "validated"}; !slices.Equal(status.Options, want) {
		t.Errorf("status options %q", status.Options)
	}
	if status.Project["comite"] != "Comité Spécialisé" || status.Project["qa"] != "QA Validation" || len(status.Project) != 2 {
		t.Errorf("status project %v", status.Project)
	}
	kinds := s.Fields["kind"]
	if want := []string{"bug", "feat", "polish", "docs", "refactor", "perf", "test", "style", "chore", "design", "data"}; !slices.Equal(kinds.Options, want) {
		t.Errorf("kinds %q", kinds.Options)
	}
	if a := kinds.Aliases; a["FIX"] != "bug" || a["DOC"] != "docs" || a["TASK"] != "feat" || len(a) != 3 {
		t.Errorf("kind aliases %v", a)
	}
	var sprints []string
	for _, sp := range s.Sprints.List {
		sprints = append(sprints, sp.Slug+" "+sp.Title+" "+sp.Start.Format("2006-01-02"))
	}
	if want := []string{"sprint-1 Sprint 1 2026-11-02", "sprint-2 Sprint 2 2026-11-16", "sprint-3 Sprint 3 2026-11-30"}; !slices.Equal(sprints, want) {
		t.Errorf("sprints %q", sprints)
	}
	if gp, _ := b.Config.Tools.GithubProject.Get(); !slices.Equal(gp.Unmapped["category"], []string{"SPECS", "TEXT"}) {
		t.Errorf("unmapped %v", gp.Unmapped)
	}
	if len(b.Config.Views) != 3 {
		t.Errorf("views %d", len(b.Config.Views))
	}

	// the second check: no warning, nothing left to fit
	out, errs, code = run(t, "-C", dir, "project", "sync", "--check")
	for _, want := range []string{" 0 warning(s)", "2 Category values unmapped by choice"} {
		if code != 0 || !strings.Contains(out, want) {
			t.Fatalf("second check: %d, want %q in\n%s\n%s", code, want, out[max(0, len(out)-600):], errs)
		}
	}
	if strings.Contains(out, "config:") {
		t.Errorf("the second check still names the fit:\n%s", out[max(0, len(out)-600):])
	}

	// the Project named again, as the page's "Write it in the file" does:
	// the table is written whole, and the values unmapped by choice stay
	b.Close()
	if out, errs, code = run(t, "-C", dir, "project", "configure", "--owner", "o", "--project", "1"); code != 0 {
		t.Fatalf("configure: %d\n%s\n%s", code, out, errs)
	}
	text, _ = os.ReadFile(file)
	if b, err = app.Open(ctx, dir, app.Options{Who: true}); err != nil {
		t.Fatalf("the file after configure: %v\n%s", err, text)
	}
	if gp, _ := b.Config.Tools.GithubProject.Get(); !slices.Equal(gp.Unmapped["category"], []string{"SPECS", "TEXT"}) {
		t.Errorf("unmapped after configure %v", gp.Unmapped)
	}
}
