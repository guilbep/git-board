package branches

import (
	"strings"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"

	"github.com/guilbep/git-board/next/domain/issues"
	"github.com/guilbep/git-board/next/tools/code"
)

const id = "abc1234def5678abc1234def5678abc1234def5678abc1234def5678abcd1234"

// folders is co2's with the issue's folder (docs/design/issue-head, decision 3)
var folders = issues.Branches{Name: "{kind}/{number}/{n}-{slug}", Kinds: co2.Kinds}

var co2 = issues.Branches{
	Name:  "{kind}/{number}-{slug}",
	Kinds: map[string]string{"bug": "fix", "feat": "feat", "perf": "perf", "task": "chore"},
}

func issue(title string, number string, labels ...string) issues.Issue {
	is := issues.Issue{ID: id, Title: title, Labels: labels}
	if number != "" {
		is.Forges = []issues.Forge{{Name: "GitHub", Number: number}}
	}
	return is
}

func TestName(t *testing.T) {
	const example = "[BUG](Equipment) Other equipment shows the wrong example (#2518)"
	tests := []struct {
		name string
		cfg  issues.Branches
		is   issues.Issue
		want string
	}{
		{"co2's example", co2, issue(example, "2518", "bug"), "fix/2518-other-equipment-shows-the-wrong-example"},
		{"no kind drops {kind}/", co2, issue(example, "2518"), "2518-other-equipment-shows-the-wrong-example"},
		{"in the issue's folder by default", issues.Branches{}, issue(example, "2518", "bug"), "2518/1-other-equipment-shows-the-wrong-example"},
		{"a kind's folder", folders, issue(example, "2518", "kind:test"), "test/2518/1-other-equipment-shows-the-wrong-example"},
		{"the kind field wins over a flat label", co2, issue("Add export", "7", "bug", "kind:feat"), "feat/7-add-export"},
		{"a kind not in kinds is used as is", co2, issue("Fix typo", "7", "kind:doc"), "doc/7-fix-typo"},
		{"flat labels in a fixed order", co2, issue("Speed up", "7", "task", "perf"), "perf/7-speed-up"},
		{"a label not in kinds isn't a kind", co2, issue("Speed up", "7", "urgent"), "7-speed-up"},
		{"no forge number: the git-bug id", co2, issue("Local only", "", "feat"), "feat/abc1234-local-only"},
		{"accents folded", issues.Branches{}, issue("Écran d'accueil : déjà vu, ça marche", "3"), "3/1-ecran-d-accueil-deja-vu-ca-marche"},
		{"[PREFIX] without a scope", issues.Branches{}, issue("[BUG] Crash on save", "3"), "3/1-crash-on-save"},
		{"[PREFIX] and (Scope) apart", issues.Branches{}, issue("[Bug]: (Planner) - Process Emission Data vs Graph", "3"), "3/1-process-emission-data-vs-graph"},
		{"a title of symbols only", issues.Branches{}, issue("!!! ???", "3"), "3/1"},
		{"long title cut on a word", issues.Branches{}, issue("Show the emission factors of every equipment category on one single page", "3"),
			"3/1-show-the-emission-factors-of-every-equipment"},
		{"one long word cut hard", issues.Branches{}, issue(strings.Repeat("a", 60), "3"), "3/1-" + strings.Repeat("a", 50)},
		{"invalid ref characters from the config", issues.Branches{Name: "~{kind}^/..{number}:{slug}.lock", Kinds: map[string]string{"bug": "bug fix.."}}, issue("Crash", "3", "bug"), "bug-fix/3-crash"},
		{"a project number with its prefix", co2, issues.Issue{ID: id, Title: "SPA routes", Number: "VM-62", Counted: true, Labels: []string{"bug"}}, "fix/vm-62-spa-routes"},
		{"a kind part left empty", issues.Branches{Name: "team/{kind}-{number}"}, issue("Crash", "3"), "team/3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Name(tt.cfg, tt.is)
			if got != tt.want {
				t.Errorf("Name = %q, want %q", got, tt.want)
			}
			if err := plumbing.NewBranchReferenceName(got).Validate(); err != nil || strings.HasPrefix(got, "-") {
				t.Errorf("%q isn't a branch git takes: %v", got, err)
			}
		})
	}
}

func TestSlugCutsNearFifty(t *testing.T) {
	s := Slug("Show the emission factors of every equipment category on one single page")
	if len(s) > 50 || strings.HasSuffix(s, "-") {
		t.Errorf("Slug = %q (%d), want at most 50, on a word", s, len(s))
	}
}

func TestNames(t *testing.T) {
	is := issue("Crash", "2518")
	tests := []struct {
		branch string
		want   bool
	}{
		{"fix/2518-other-equipment", true},
		{"2518-other-equipment", true},
		{"2518", true},
		{"12518-other", false},
		{"fix/25180-other", false},
		{"fix/12-upgrade-node-2518", false}, // #12's: the first number names the issue
		{"abc1234-local-only", true},
		{"feat/abc1234def-local", true},
		{"abc123-too-short", false},
		{"dev", false},
	}
	for _, tt := range tests {
		if got := Names(tt.branch, is); got != tt.want {
			t.Errorf("Names(%q) = %v, want %v", tt.branch, got, tt.want)
		}
	}
}

func TestNamesWhatNameSuggests(t *testing.T) {
	for _, is := range []issues.Issue{issue("Local only", ""), issue("On GitHub", "2518", "bug"),
		{ID: id, Title: "SPA routes", Number: "VM-62", Counted: true, Labels: []string{"bug"}, Forges: []issues.Forge{{Name: "GitHub", Number: "66"}}}} {
		if name := Name(co2, is); !Names(name, is) {
			t.Errorf("Names(%q) = false for the name it suggests", name)
		}
	}
}

// Once the project numbers its issues, a branch's number is the project's:
// fix/3060-… is CO2-3060's, not GitHub #3060's; an issue the counter hasn't
// numbered yet names its branch by its id.
func TestNamesProjectNumbers(t *testing.T) {
	x := issues.Issue{ID: "aaaaaaaaaa", Title: "Bouncer pool", Number: "CO2-3060", Counted: true, Forges: []issues.Forge{{Name: "GitHub", Number: "3071"}}}
	y := issues.Issue{ID: "bbbbbbbbbb", Title: "CSV export", Number: "CO2-3075", Counted: true, Forges: []issues.Forge{{Name: "GitHub", Number: "3060"}}}
	waiting := issues.Issue{ID: "cccccccccc", Title: "New one", Counted: true, Forges: []issues.Forge{{Name: "GitHub", Number: "3080"}}}
	for _, tc := range []struct {
		branch string
		is     issues.Issue
		want   bool
	}{
		{"fix/3060-bouncer-pool", x, true},
		{"fix/co2-3060-bouncer-pool", x, true},
		{"fix/co2-3060-bouncer-pool", y, false},
		{"fix/3060-bouncer-pool", y, false},
		{"fix/3071-bouncer-pool", x, false},
		{"fix/3080-new-one", waiting, false},
		{"ccccccc-new-one", waiting, true},
	} {
		if got := Names(tc.branch, tc.is); got != tc.want {
			t.Errorf("Names(%q, %s) = %v, want %v", tc.branch, tc.is.Title, got, tc.want)
		}
	}
	if got := Name(issues.Branches{}, x); got != "co2-3060/1-bouncer-pool" {
		t.Errorf("Name(x) = %q", got)
	}
	if got := Name(issues.Branches{}, waiting); got != "ccccccc/1-new-one" {
		t.Errorf("Name(waiting) = %q", got)
	}
}

func TestFolderAndNext(t *testing.T) {
	t.Parallel()
	is := issue("Implement tests", "2771", "kind:test")
	if got := Folder(folders, is); got != "test/2771/" {
		t.Errorf("Folder = %q", got)
	}
	if got := Folder(co2, is); got != "" {
		t.Errorf("Folder without {n} = %q, want none", got)
	}
	if got := Folder(issues.Branches{}, issue("Crash", "2811")); got != "2811/" {
		t.Errorf("Folder by default = %q", got)
	}
	names := []string{"test/2771/1-implement-tests", "test/2771/3-ci", "test/2771-old-form", "test/27710/9-another-issue", "feat/2771/8-another-folder", "other-test/2771/7-not-it"}
	if got := Next("test/2771/", names, nil); got != 4 {
		t.Errorf("Next = %d, want 4", got)
	}
	// a merged branch, deleted since: its number stays taken
	merges := []string{"Merge pull request #3155 from org/test/2771/5-reference-examples", "Merge branch 'test/2771/2-x' into dev"}
	if got := Next("test/2771/", names, merges); got != 6 {
		t.Errorf("Next with the merges = %d, want 6", got)
	}
	if got := Next("test/2771/", nil, nil); got != 1 {
		t.Errorf("Next of none = %d, want 1", got)
	}
	if got := NameAt(folders, is, 4); got != "test/2771/4-implement-tests" {
		t.Errorf("NameAt = %q", got)
	}
}

func TestNamesInFolders(t *testing.T) {
	t.Parallel()
	is := issue("Implement tests", "2771", "kind:test")
	one := issues.Issue{ID: "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210", Title: "The first issue", Forges: []issues.Forge{{Name: "GitHub", Number: "1"}}}
	byID := issue("Local only", "") // abc1234/1-local-only
	footer := issue("PDF footer", "2811")
	counted := issues.Issue{ID: id, Title: "Rounding", Number: "CO2-421", Counted: true}
	for _, tc := range []struct {
		branch string
		is     issues.Issue
		want   bool
	}{
		{"test/2771/1-implement-tests", is, true},
		{"test/2771/2-reference-examples", is, true},
		{"2771/12-x", is, true},
		{"test/2771/1-implement-tests", one, false}, // its {n} is no issue's number
		{"abc1234/1-local-only", byID, true},
		{"abc1234/1-local-only", one, false}, // an id's folder: the 1 after it names no issue
		{"2811/1-pdf-footer", footer, true},  // no kind: the folder alone
		{"test/2771b-x", is, false},          // a letter glued to the number: not a number word
		{"feat/co2-421/1-x", counted, true},  // a project number, its prefix a word of its own
		{"feat/co2-421/1-x", footer, false},
	} {
		if got := Names(tc.branch, tc.is); got != tc.want {
			t.Errorf("Names(%q, %s) = %v, want %v", tc.branch, tc.is.Title, got, tc.want)
		}
	}
	for n := 1; n <= 3; n++ {
		for _, x := range []issues.Issue{is, byID, one} {
			if name := NameAt(folders, x, n); !Names(name, x) || !Valid(name) {
				t.Errorf("NameAt(%s, %d) = %q: not named, or not valid", x.Title, n, name)
			}
		}
	}
}

func TestValid(t *testing.T) {
	t.Parallel()
	for name, want := range map[string]bool{
		"test/2771/1-implement-tests": true,
		"2811/1-pdf":                  true,
		"":                            false,
		"-x":                          false,
		"a..b":                        false,
		"a b":                         false,
		"x.lock":                      false,
		"a//b":                        false,
		"a/":                          false,
		"refs;rm -rf":                 false,
		"$(x)":                        false,
	} {
		if got := Valid(name); got != want {
			t.Errorf("Valid(%q) = %v, want %v", name, got, want)
		}
	}
}

// The command to copy quotes where it starts: a committed [branches] base
// is the file's, and a shell runs what's copied.
func TestSuggestQuotesTheBaseInTheCommand(t *testing.T) {
	t.Parallel()
	b := code.Repo{Dir: repo(t)}
	s, err := Suggest(t.Context(), b, issues.Branches{Base: "main; touch pwned"}, issues.Issue{ID: id, Title: "a fix"})
	if err != nil {
		t.Fatal(err)
	}
	if want := " 'origin/main; touch pwned'"; !strings.HasSuffix(s.Command, want) {
		t.Errorf("command %q, want it to end with %q", s.Command, want)
	}
	s, err = Suggest(t.Context(), b, issues.Branches{Base: "it's"}, issues.Issue{ID: id, Title: "a fix"})
	if err != nil {
		t.Fatal(err)
	}
	if want := ` 'origin/it'\''s'`; !strings.HasSuffix(s.Command, want) {
		t.Errorf("command %q, want it to end with %q", s.Command, want)
	}
	if s, _ = Suggest(t.Context(), b, issues.Branches{Base: "release/1.2"}, issues.Issue{ID: id, Title: "a fix"}); !strings.HasSuffix(s.Command, " origin/release/1.2") {
		t.Errorf("a plain base quoted: %q", s.Command)
	}
}
