package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

const fixture = "testdata/git-board.toml"

func mustLoad(t *testing.T) *Config {
	t.Helper()
	cfg, err := Load(fixture)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return cfg
}

func TestParsesReposAndFields(t *testing.T) {
	cfg := mustLoad(t)

	want := []string{"EPFL-ENAC/co2-calculator", "EPFL-ENAC/co2-calculator-back-office-doc"}
	if len(cfg.Repos) != len(want) || cfg.Repos[0] != want[0] || cfg.Repos[1] != want[1] {
		t.Fatalf("Repos = %v, want %v", cfg.Repos, want)
	}

	status := cfg.Fields["status"]
	if !status.Single {
		t.Error("status.Single = false, want true")
	}
	if status.Options[0] != "backlog" || status.Options[len(status.Options)-1] != "validated" {
		t.Errorf("status.Options endpoints = %q..%q", status.Options[0], status.Options[len(status.Options)-1])
	}
	if len(status.Options) != 9 {
		t.Errorf("len(status.Options) = %d, want 9", len(status.Options))
	}

	if !cfg.Fields["est"].Numeric {
		t.Error("est.Numeric = false, want true")
	}
	if cfg.Fields["assignee"].Single {
		t.Error("assignee.Single = true, want false")
	}
}

func TestParsesSprints(t *testing.T) {
	cfg := mustLoad(t)

	if cfg.Sprints.LengthDays != 14 {
		t.Errorf("LengthDays = %d, want 14", cfg.Sprints.LengthDays)
	}
	if cfg.Sprints.StartWeekday != "monday" {
		t.Errorf("StartWeekday = %q, want monday", cfg.Sprints.StartWeekday)
	}
	if len(cfg.Sprints.List) != 2 || cfg.Sprints.List[0].Slug != "delivery" || cfg.Sprints.List[1].Slug != "post-delivery" {
		t.Fatalf("sprint slugs = %+v", cfg.Sprints.List)
	}
	wantStart := time.Date(2026, 8, 24, 0, 0, 0, 0, cfg.Sprints.List[0].Start.Location())
	if !cfg.Sprints.List[0].Start.Equal(wantStart) {
		t.Errorf("delivery.Start = %v, want %v", cfg.Sprints.List[0].Start, wantStart)
	}
	if cfg.Sprints.List[1].Title != "Post-delivery" {
		t.Errorf("post-delivery.Title = %q", cfg.Sprints.List[1].Title)
	}
}

func TestParsesViews(t *testing.T) {
	cfg := mustLoad(t)

	if cfg.Views["current-sprint"].Layout != "board" {
		t.Errorf("current-sprint.Layout = %q", cfg.Views["current-sprint"].Layout)
	}
	if cfg.Views["current-sprint"].Group != "status" {
		t.Errorf("current-sprint.Group = %q", cfg.Views["current-sprint"].Group)
	}
	if cfg.Views["backlog"].Sort != "prio asc" {
		t.Errorf("backlog.Sort = %q", cfg.Views["backlog"].Sort)
	}
	if cfg.Views["mine"].Filter != "assignee:@me is:open" {
		t.Errorf("mine.Filter = %q", cfg.Views["mine"].Filter)
	}
	// the file's order, not the alphabet's: the tabs, the first opens
	if got := cfg.ViewNames(); !slices.Equal(got, []string{"current-sprint", "backlog", "mine"}) {
		t.Errorf("ViewNames = %v", got)
	}
}

func TestResolveSprintCurrentNextPrevious(t *testing.T) {
	cfg := mustLoad(t)
	today := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC) // inside "delivery" sprint window

	current, err := ResolveSprint(cfg.Sprints, "current", today)
	if err != nil || current == nil || current.Slug != "delivery" {
		t.Fatalf("current = %+v, err = %v", current, err)
	}
	next, err := ResolveSprint(cfg.Sprints, "next", today)
	if err != nil || next == nil || next.Slug != "post-delivery" {
		t.Fatalf("next = %+v, err = %v", next, err)
	}
	previous, err := ResolveSprint(cfg.Sprints, "previous", today)
	if err != nil || previous != nil {
		t.Fatalf("previous = %+v, err = %v, want nil", previous, err)
	}
}

func TestSetGithubProjectAppendsToExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "git-board.toml")
	original, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatalf("write copy: %v", err)
	}

	if err := SetGithubProject(path, "EPFL-ENAC", 40); err != nil {
		t.Fatalf("SetGithubProject: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load after set: %v", err)
	}
	if cfg.GithubProject.Owner != "EPFL-ENAC" || cfg.GithubProject.Number != 40 {
		t.Fatalf("GithubProject = %+v, want {EPFL-ENAC 40}", cfg.GithubProject)
	}
	// Untouched tables must still parse fine.
	if len(cfg.Fields["status"].Options) != 9 {
		t.Errorf("status options lost after SetGithubProject: %v", cfg.Fields["status"].Options)
	}
	if cfg.Views["backlog"].Sort != "prio asc" {
		t.Errorf("views lost after SetGithubProject: %+v", cfg.Views["backlog"])
	}
}

func TestSetGithubProjectIsIdempotentOnRerun(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "git-board.toml")
	original, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatalf("write copy: %v", err)
	}

	if err := SetGithubProject(path, "old-owner", 1); err != nil {
		t.Fatalf("first SetGithubProject: %v", err)
	}
	if err := SetGithubProject(path, "EPFL-ENAC", 40); err != nil {
		t.Fatalf("second SetGithubProject: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.GithubProject.Owner != "EPFL-ENAC" || cfg.GithubProject.Number != 40 {
		t.Fatalf("GithubProject = %+v, want the second call's values, not both", cfg.GithubProject)
	}
}

func TestSetGithubProjectCreatesNewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "git-board.toml")

	if err := SetGithubProject(path, "EPFL-ENAC", 40); err != nil {
		t.Fatalf("SetGithubProject on missing file: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.GithubProject.Owner != "EPFL-ENAC" || cfg.GithubProject.Number != 40 {
		t.Fatalf("GithubProject = %+v", cfg.GithubProject)
	}
}

// The next table's comments are its own: a change or a removal of the
// table above keeps them.
func TestWithAndWithoutGithubProjectKeepTheComments(t *testing.T) {
	t.Parallel()
	src := "# mine\n[github-project]\nowner = \"old\"\nnumber = 1\nwrite = true\n\n# the people\n[roles]\nlead = \"me\"\n"
	text := WithGithubProject(src, GithubProject{Owner: "EPFL-ENAC", Number: 40, Write: true})
	want := "# mine\n[github-project]\nowner = \"EPFL-ENAC\"\nnumber = 40\nwrite = true\n\n# the people\n[roles]\nlead = \"me\"\n"
	if text != want {
		t.Errorf("changed:\n%s\nwant:\n%s", text, want)
	}
	if text = WithoutGithubProject(text); text != "# mine\n\n# the people\n[roles]\nlead = \"me\"\n" {
		t.Errorf("removed:\n%q", text)
	}
}

func TestSprintWindowsComputesEndFromNextStartOrLength(t *testing.T) {
	cfg := mustLoad(t)

	windows := SprintWindows(cfg.Sprints)
	if len(windows) != 2 {
		t.Fatalf("len(windows) = %d, want 2", len(windows))
	}
	// Compared as Y-M-D strings, not instants — dateOnly() (used inside
	// SprintWindows) normalizes to UTC, while a bare TOML date decodes in
	// the local zone, so a raw .Equal would fail on timezone offset alone
	// despite naming the same calendar day.
	const layout = "2006-01-02"
	// delivery's end is post-delivery's start (2026-09-07), not
	// delivery.Start + length_days (which would also land on 2026-09-07
	// here by coincidence — the real assertion is against post-delivery's
	// own start value, not the arithmetic).
	if got := windows[0].End.Format(layout); got != "2026-09-07" {
		t.Errorf("delivery.End = %s, want 2026-09-07 (post-delivery's start)", got)
	}
	// post-delivery is last, so its end is Start + length_days (14).
	if got := windows[1].End.Format(layout); got != "2026-09-21" {
		t.Errorf("post-delivery.End = %s, want 2026-09-21 (start + 14d)", got)
	}
}

func TestAppendSprintRoundTrips(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "git-board.toml")
	original, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatalf("write copy: %v", err)
	}

	start := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	if err := AppendSprint(path, "q1-hardening", "Q1 Hardening", start); err != nil {
		t.Fatalf("AppendSprint: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load after AppendSprint: %v", err)
	}
	if len(cfg.Sprints.List) != 3 {
		t.Fatalf("len(Sprints.List) = %d, want 3", len(cfg.Sprints.List))
	}
	added := cfg.Sprints.List[2]
	if added.Slug != "q1-hardening" || added.Title != "Q1 Hardening" || added.Start.Format("2006-01-02") != start.Format("2006-01-02") {
		t.Fatalf("added sprint = %+v, want start 2026-10-05", added)
	}
	// The two pre-existing sprints must still be there, untouched.
	if cfg.Sprints.List[0].Slug != "delivery" || cfg.Sprints.List[1].Slug != "post-delivery" {
		t.Fatalf("existing sprints lost: %+v", cfg.Sprints.List)
	}
	// Everything else in the file must still parse fine.
	if len(cfg.Fields["status"].Options) != 9 {
		t.Errorf("status options lost after AppendSprint: %v", cfg.Fields["status"].Options)
	}
	if cfg.Views["backlog"].Sort != "prio asc" {
		t.Errorf("views lost after AppendSprint: %+v", cfg.Views["backlog"])
	}
	if cfg.Repos[0] != "EPFL-ENAC/co2-calculator" {
		t.Errorf("repos lost after AppendSprint: %v", cfg.Repos)
	}
}

func TestAppendSprintRejectsDuplicateSlug(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "git-board.toml")
	original, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatalf("write copy: %v", err)
	}

	err = AppendSprint(path, "delivery", "Duplicate", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("expected an error for a duplicate slug, got nil")
	}

	// The file must be untouched — no partial write on the rejected path.
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after rejected AppendSprint: %v", err)
	}
	if string(after) != string(original) {
		t.Errorf("file changed despite rejected AppendSprint")
	}
}

func TestResolveSprintBeforeFirstStart(t *testing.T) {
	cfg := mustLoad(t)
	today := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	if current, _ := ResolveSprint(cfg.Sprints, "current", today); current != nil {
		t.Errorf("current = %+v, want nil", current)
	}
	next, err := ResolveSprint(cfg.Sprints, "next", today)
	if err != nil || next == nil || next.Slug != "delivery" {
		t.Fatalf("next = %+v, err = %v", next, err)
	}
	if previous, _ := ResolveSprint(cfg.Sprints, "previous", today); previous != nil {
		t.Errorf("previous = %+v, want nil", previous)
	}
}

func TestLoadMissingFileIsAnEmptyConfig(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), ".git-board.toml"))
	if err != nil || cfg == nil {
		t.Fatalf("Load = %v, %v; want an empty config", cfg, err)
	}
	if len(cfg.Fields)+len(cfg.Views)+len(cfg.Sprints.List) != 0 {
		t.Errorf("config not empty: %+v", cfg)
	}
}

func TestKnownValueNumericTakesAnyNumber(t *testing.T) {
	f := Field{Single: true, Numeric: true, Options: []string{"1", "2"}}
	if !f.KnownValue("3.5") || !f.KnownValue("33") {
		t.Error("a number outside the options was refused")
	}
	if f.KnownValue("lots") {
		t.Error("a non-number was taken")
	}
}

// From a subdirectory, the config is the repository root's, found or not.
func TestPathFindsTheRepositoryRoot(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "frontend", "src")
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(sub)
	same := func(a, b string) bool { // macOS: /var is /private/var
		a, _ = filepath.EvalSymlinks(a)
		b, _ = filepath.EvalSymlinks(b)
		return a == b
	}
	if p := Path(); filepath.Base(p) != FileName || !same(filepath.Dir(p), root) {
		t.Errorf("without a config: Path = %q, want %s at %q", p, FileName, root)
	}
	if err := os.WriteFile(filepath.Join(root, FileName), []byte("[fields.prio]\nsingle = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(Path())
	if err != nil || !cfg.Fields["prio"].Single {
		t.Errorf("Load(Path()) = %+v, %v, want the root's config", cfg, err)
	}
}

func TestBranchesKind(t *testing.T) {
	b := Branches{Kinds: map[string]string{"bug": "fix", "feat": "feat"}}
	tests := []struct {
		labels []string
		want   string
	}{
		{[]string{"prio:p1", "bug"}, "bug"},      // the label, not its branch's fix
		{[]string{"bug", "kind:chore"}, "chore"}, // the kind field wins
		{[]string{"FEAT"}, ""},                   // labels are exact: FEAT isn't feat
		{[]string{"urgent"}, ""},
	}
	for _, tt := range tests {
		if got := b.Kind(tt.labels); got != tt.want {
			t.Errorf("Kind(%v) = %q, want %q", tt.labels, got, tt.want)
		}
	}
}

// [branches] kinds are the kind field, in the file's order, unless
// [fields.kind] has options: then kind:* labels are.
func TestFlatKinds(t *testing.T) {
	t.Parallel()
	cfg, _, err := parse([]byte(`[fields.kind.colors]
bug = "red"

[branches]
kinds = { test = "test", bug = "fix", feat = "feat" }
`))
	if err != nil {
		t.Fatal(err)
	}
	k := cfg.Fields["kind"]
	if !cfg.FlatKinds() || !k.Flat || !k.Single || !slices.Equal(k.Options, []string{"test", "bug", "feat"}) || k.Colors["bug"] != "red" {
		t.Errorf("kind = %+v, flat %v", k, cfg.FlatKinds())
	}
	if cfg.Label("kind", "bug") != "bug" || cfg.Label("kind", "chore") != "kind:chore" || cfg.Label("status", "ready") != "status:ready" {
		t.Errorf("Label: %q %q %q", cfg.Label("kind", "bug"), cfg.Label("kind", "chore"), cfg.Label("status", "ready"))
	}

	cfg, _, err = parse([]byte("[fields.kind]\noptions = [\"bug\", \"chore\"]\n\n[branches]\nkinds = { bug = \"fix\" }\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.FlatKinds() || cfg.Fields["kind"].Flat || cfg.Label("kind", "bug") != "kind:bug" {
		t.Errorf("kind:* form: flat %v, Label %q", cfg.FlatKinds(), cfg.Label("kind", "bug"))
	}
	// a Config built by hand, as tests do
	if !(&Config{Branches: Branches{Kinds: map[string]string{"bug": "fix"}}}).FlatKinds() {
		t.Error("kinds without a kind field: not flat")
	}
}

func TestDescriptionsAreGitHubsLength(t *testing.T) {
	t.Parallel()
	ok := strings.Repeat("é", 100)
	if _, _, err := parse([]byte("[fields.kind.descriptions]\nbug = \"" + ok + "\"\n")); err != nil {
		t.Errorf("100 characters: %v", err)
	}
	if _, _, err := parse([]byte("[fields.kind.descriptions]\nbug = \"" + ok + "e\"\n")); err == nil || !strings.Contains(err.Error(), "101") {
		t.Errorf("101 characters: %v", err)
	}
}

func TestSetFieldTable(t *testing.T) {
	t.Parallel()
	colors := map[string]string{"bug": "#d73a4a", "nice to have": "teal"}
	for _, tt := range []struct{ name, in, want string }{
		{"inline", "[fields.kind]\nsingle = true\ncolors = { bug = \"red\" }  # org\n\n# Estimate\n[fields.est]\n",
			"[fields.kind]\nsingle = true\n\n[fields.kind.colors]\nbug = \"#d73a4a\"\n\"nice to have\" = \"teal\"\n\n# Estimate\n[fields.est]\n"},
		{"table", "[fields.kind]\nsingle = true\n\n[fields.kind.colors]\nbug = \"red\"  # org\n\n# Estimate\n[fields.est]\n",
			"[fields.kind]\nsingle = true\n\n[fields.kind.colors]\nbug = \"#d73a4a\"\n\"nice to have\" = \"teal\"\n\n# Estimate\n[fields.est]\n"},
		{"no field table", "[branches]\nkinds = { bug = \"fix\" }\n",
			"[branches]\nkinds = { bug = \"fix\" }\n\n[fields.kind.colors]\nbug = \"#d73a4a\"\n\"nice to have\" = \"teal\"\n"},
	} {
		if got := SetFieldTable(tt.in, "kind", "colors", colors); got != tt.want {
			t.Errorf("%s:\n%s\nwant\n%s", tt.name, got, tt.want)
		} else if _, _, err := parse([]byte(got)); err != nil {
			t.Errorf("%s: %v", tt.name, err)
		}
	}
}

func TestWithRelease(t *testing.T) {
	t.Parallel()
	in := "# the board\n[board]\nname = \"x\"\n\n[release]\nenvironments = []\n\n[branches]\nbase = \"main\"\n"
	r := Release{Binaries: []string{"./cmd/git-board"}, Platforms: []string{"linux/amd64", "darwin/arm64"}, Build: "make dist", Remote: "origin"}
	got := WithRelease(in, r)
	want := "# the board\n[board]\nname = \"x\"\n\n[release]\nbinaries = [\"./cmd/git-board\"]\nplatforms = [\"linux/amd64\", \"darwin/arm64\"]\nbuild = \"make dist\"\n\n[branches]\nbase = \"main\"\n"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	cfg, _, err := Check([]byte(got))
	if err != nil || !cfg.Release.Enabled() || cfg.Release.Build != "make dist" {
		t.Errorf("check: %v, %+v", err, cfg.Release)
	}
	if got := WithRelease("", Release{Binaries: []string{"."}, Remote: "upstream"}); got != "[release]\nbinaries = [\".\"]\nremote = \"upstream\"\n" {
		t.Errorf("empty file: %q", got)
	}
}

// [repos] assignees is native unless the file says labels, and nothing else.
func TestAssigneesMode(t *testing.T) {
	t.Parallel()
	if got := mustLoad(t).Assignees; got != "native" {
		t.Errorf("default %q, want native", got)
	}
	dir := t.TempDir()
	for value, want := range map[string]string{"labels": "labels", "lables": ""} {
		path := filepath.Join(dir, value+".toml")
		if err := os.WriteFile(path, []byte("[repos]\nassignees = \""+value+"\"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		cfg, err := Load(path)
		if want == "" {
			if err == nil {
				t.Errorf("%q: loaded, want an error", value)
			}
			continue
		}
		if err != nil || cfg.Assignees != want {
			t.Errorf("%q: %v, %v; want %q", value, cfg, err, want)
		}
	}
}

// The forges' sync holds what has another home: on GitHub, with a
// Project, the fields the Project holds; everywhere, the fields naming
// issues, and the milestone.
func TestHeld(t *testing.T) {
	t.Parallel()
	c := &Config{GithubProject: GithubProject{Owner: "o", Number: 1}}
	for _, tc := range []struct {
		key, label string
		want       bool
	}{
		{"github.com/o/r", "status:ready", true},
		{"github.com/o/r", "Status:Ready", true},
		{"github.com/o/r", "prio:p1", true},
		{"github.com/o/r", "sprint:sprint-12", true},
		{"github.com/o/r", "est:6", true},
		{"github.com/o/r", "moscow:must", true},
		{"github.com/o/r", "hold:blocked", false},
		{"github.com/o/r", "bug", false},
		{"github.com/o/r", "parent:vm-12", true},
		{"gitlab.epfl.ch/42400", "status:ready", false},
		{"gitlab.epfl.ch/42400", "blocked-by:vm-70", true},
		{"gitlab.epfl.ch/42400", "milestone:v3-1", true},
	} {
		if got := c.Held(tc.key, tc.label); got != tc.want {
			t.Errorf("Held(%s, %s) = %v, want %v", tc.key, tc.label, got, tc.want)
		}
	}
	if (&Config{}).Held("github.com/o/r", "status:ready") {
		t.Error("without a Project, status: held on GitHub")
	}
}
