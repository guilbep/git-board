package migrate

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/guilbep/git-board/core/forge"
	"github.com/guilbep/git-board/core/forge/fake"
	"github.com/guilbep/git-board/core/model"
	"github.com/guilbep/git-board/core/people"
	"github.com/guilbep/git-board/internal/config"
)

func migrateConfig() *config.Config {
	return &config.Config{
		Fields: map[string]config.Field{
			"assignee": {Type: config.TypePerson},
			"moscow":   {Single: true, Options: []string{"musthave", "couldhave", "wonthave"}, Aliases: map[string]string{"could": "couldhave"}},
		},
		Forges: []config.Forge{
			{Name: "gh", Kind: "github", Host: "github.com", Project: "o/r",
				Fields: map[string]config.FieldMapping{"moscow": {To: config.TargetLabel}}},
			{Name: "gl", Kind: "gitlab", Host: "gitlab.epfl.ch", Project: "7"},
		},
	}
}

// persons is git-board's directory: the accounts a write resolves through,
// GitLab's by user id among them.
var persons = []people.Person{
	{ID: "p1", Name: "Guilbert", Accounts: []people.Account{
		{Server: "github.com", ID: "31", Login: "guilbep"},
		{Server: "gitlab.epfl.ch", ID: "404", Login: "guilbep"},
	}},
	{ID: "p2", Name: "carol", Accounts: []people.Account{{Server: "gitlab.epfl.ch", ID: "9", Login: "carol"}}},
}

func issue(id, url string, labels ...string) model.Issue {
	return model.Issue{ID: id, GithubURL: url, Labels: labels}
}

func openFakes(t *testing.T, fakes map[string]*fake.Forge) func(config.Forge) (forge.Forge, error) {
	t.Helper()
	return func(f config.Forge) (forge.Forge, error) {
		if x, ok := fakes[f.Name]; ok {
			return x, nil
		}
		return nil, &forge.Refused{Why: "no token"}
	}
}

// writes is the fake's writes of one op, with their args.
func writes(f *fake.Forge, op string) [][]string {
	var out [][]string
	for _, w := range f.Writes() {
		if w.Op == op {
			out = append(out, w.Args)
		}
	}
	return out
}

// The preview counts the issues per forge, and says what each target
// needs: the assignees are set by the run, a label target has nothing to
// migrate, the Project is a sync run's to write.
func TestPreviewCountsTheIssues(t *testing.T) {
	t.Parallel()
	cfg := migrateConfig()
	issues := []model.Issue{issue("a", "https://github.com/o/r/issues/1", "assignee:guilbep", "moscow:could")}
	plans := Preview(cfg, "assignee", issues)
	if len(plans) != 2 {
		t.Fatalf("plans = %+v", plans)
	}
	gh := plans[0]
	if gh.Forge != "gh" || gh.Target != config.TargetAssignees || gh.Issues != 1 {
		t.Errorf("gh plan = %+v, want the assignees target and one issue", gh)
	}
	if plans[1].Target != config.TargetAssignees {
		t.Errorf("gl plan = %+v, want the assignees default", plans[1])
	}
	// moscow stays a label on both: nothing to migrate, and the note says so
	plans = Preview(cfg, "moscow", issues)
	for _, p := range plans {
		if p.Note == "" {
			t.Errorf("%s: no note, want the nothing-to-migrate line", p.Forge)
		}
	}
}

// The run sets the assignees and takes the field's labels off the issues
// — the namespaced spelling the storage keeps — and the storage keeps its
// own: the namespaced labels stay in git-board. An issue with no forge
// link has no stake.
func TestRunSetsTheAssigneesAndTakesTheLabelsOff(t *testing.T) {
	t.Parallel()
	cfg := migrateConfig()
	gh, gl := fake.New("github.com/o/r"), fake.New("gitlab.epfl.ch/7")
	gh.Add(forge.Issue{Number: "1"})
	gl.Add(forge.Issue{Number: "2"})
	issues := []model.Issue{
		issue("a", "https://github.com/o/r/issues/1", "assignee:guilbep", "moscow:could"),
		issue("b", "https://gitlab.epfl.ch/7/-/issues/2", "assignee:carol"),
		issue("c", "", "assignee:nolink"),
	}
	said := []string{}
	plans, err := Migrate(context.Background(), cfg, "assignee", issues, persons, openFakes(t, map[string]*fake.Forge{"gh": gh, "gl": gl}),
		func(line string) { said = append(said, line) })
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	assignees := func(f *fake.Forge, n string) []string {
		is, err := f.Issue(context.Background(), forge.Ref{Number: n})
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, a := range is.Assignees {
			out = append(out, a.Login)
		}
		return out
	}
	if got := assignees(gh, "1"); !slices.Equal(got, []string{"guilbep"}) {
		t.Errorf("gh assignees = %v, want guilbep", got)
	}
	if got := assignees(gl, "2"); !slices.Equal(got, []string{"carol"}) {
		t.Errorf("gl assignees = %v, want carol", got)
	}
	// the write carries the resolved account, as GitLab needs the id
	if got := writes(gh, "set-assignees"); len(got) != 1 || !slices.Equal(got[0], []string{"31"}) {
		t.Errorf("gh assignee writes = %v, want the resolved account 31", got)
	}
	if got := writes(gl, "set-assignees"); len(got) != 1 || !slices.Equal(got[0], []string{"9"}) {
		t.Errorf("gl assignee writes = %v, want the resolved account 9", got)
	}
	if got := writes(gh, "change-labels"); len(got) != 1 {
		t.Errorf("gh label writes = %v, want assignee:guilbep off #1", got)
	}
	if got := writes(gl, "change-labels"); len(got) != 1 {
		t.Errorf("gl label writes = %v, want assignee:carol off #2", got)
	}
	// the storage is the labels': the issues keep them
	if !slices.Contains(issues[0].Labels, "assignee:guilbep") {
		t.Errorf("the issue's own labels changed: %v", issues[0].Labels)
	}
	// no link, no stake: the issue is absent from the plans' counts
	for _, p := range plans {
		if p.Issues != 1 {
			t.Errorf("%s: issues = %d, want 1 (the linked one)", p.Forge, p.Issues)
		}
	}
	if !slices.ContainsFunc(said, func(l string) bool { return len(l) > 0 }) {
		t.Errorf("nothing said: %v", said)
	}
}

// A forge that refuses every write keeps its note honest: nothing moved,
// and why — not a count that claims the move happened.
func TestRunSaysWhatDidntMove(t *testing.T) {
	t.Parallel()
	cfg := migrateConfig()
	gh := fake.New("github.com/o/r")
	gh.RefuseLabels("assignee:guilbep")
	gh.Add(forge.Issue{Number: "1"})
	issues := []model.Issue{issue("a", "https://github.com/o/r/issues/1", "assignee:guilbep")}
	plans, err := Migrate(context.Background(), cfg, "assignee", issues, persons, openFakes(t, map[string]*fake.Forge{"gh": gh}), func(string) {})
	if err == nil {
		t.Fatalf("migrate = %+v, want the refusal", plans)
	}
	if plans[0].Labels != 0 || strings.Contains(plans[0].Note, "moved to") {
		t.Errorf("gh plan = %+v, want nothing moved and why", plans[0])
	}
}

// The aliases fold on the way out: moscow:could reads couldhave, so the
// label removed from gh is moscow:couldhave — and a Project target takes
// the old labels off without writing anything itself: the sync does that.
func TestRunReadsTheAliases(t *testing.T) {
	t.Parallel()
	cfg := migrateConfig()
	cfg.Forges[0].Fields["moscow"] = config.FieldMapping{To: config.TargetProject, Field: "MoSCoW"}
	gh := fake.New("github.com/o/r")
	gh.Add(forge.Issue{Number: "1"})
	issues := []model.Issue{issue("a", "https://github.com/o/r/issues/1", "moscow:could")}
	plans, err := Migrate(context.Background(), cfg, "moscow", issues, persons, openFakes(t, map[string]*fake.Forge{"gh": gh}), func(string) {})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if got := writes(gh, "change-labels"); len(got) != 1 || !slices.Contains(got[0], "moscow:couldhave") {
		t.Errorf("label writes = %v, want moscow:couldhave off #1", got)
	}
	if len(plans) != 2 || plans[0].Labels != 1 || plans[1].Labels != 0 { // gl: the issue is GitHub's
		t.Errorf("plans = %+v, want one label taken off on gh, nothing on gl", plans)
	}
}
