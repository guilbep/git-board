package branches

import (
	"strings"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio"
)

const id = "abc1234def5678abc1234def5678abc1234def5678abc1234def5678abcd1234"

var co2 = config.Branches{
	Name:  "{kind}/{number}-{slug}",
	Kinds: map[string]string{"bug": "fix", "feat": "feat", "perf": "perf", "task": "chore"},
}

func issue(title string, number string, labels ...string) gitbugio.Issue {
	is := gitbugio.Issue{ID: id, Title: title, Labels: labels}
	if number != "" {
		is.Forges = []gitbugio.Forge{{Name: "GitHub", Number: number}}
	}
	return is
}

func TestName(t *testing.T) {
	const example = "[BUG](Equipment) Other equipment shows the wrong example (#2518)"
	tests := []struct {
		name string
		cfg  config.Branches
		is   gitbugio.Issue
		want string
	}{
		{"co2's example", co2, issue(example, "2518", "bug"), "fix/2518-other-equipment-shows-the-wrong-example"},
		{"no kind drops {kind}/", co2, issue(example, "2518"), "2518-other-equipment-shows-the-wrong-example"},
		{"GitHub's by default", config.Branches{}, issue(example, "2518", "bug"), "2518-other-equipment-shows-the-wrong-example"},
		{"the kind field wins over a flat label", co2, issue("Add export", "7", "bug", "kind:feat"), "feat/7-add-export"},
		{"a kind not in kinds is used as is", co2, issue("Fix typo", "7", "kind:doc"), "doc/7-fix-typo"},
		{"flat labels in a fixed order", co2, issue("Speed up", "7", "task", "perf"), "perf/7-speed-up"},
		{"a label not in kinds isn't a kind", co2, issue("Speed up", "7", "urgent"), "7-speed-up"},
		{"no forge number: the git-bug id", co2, issue("Local only", "", "feat"), "feat/abc1234-local-only"},
		{"accents folded", config.Branches{}, issue("Écran d'accueil : déjà vu, ça marche", "3"), "3-ecran-d-accueil-deja-vu-ca-marche"},
		{"[PREFIX] without a scope", config.Branches{}, issue("[BUG] Crash on save", "3"), "3-crash-on-save"},
		{"[PREFIX] and (Scope) apart", config.Branches{}, issue("[Bug]: (Planner) - Process Emission Data vs Graph", "3"), "3-process-emission-data-vs-graph"},
		{"a title of symbols only", config.Branches{}, issue("!!! ???", "3"), "3"},
		{"long title cut on a word", config.Branches{}, issue("Show the emission factors of every equipment category on one single page", "3"),
			"3-show-the-emission-factors-of-every-equipment"},
		{"one long word cut hard", config.Branches{}, issue(strings.Repeat("a", 60), "3"), "3-" + strings.Repeat("a", 50)},
		{"invalid ref characters from the config", config.Branches{Name: "~{kind}^/..{number}:{slug}.lock", Kinds: map[string]string{"bug": "bug fix.."}}, issue("Crash", "3", "bug"), "bug-fix/3-crash"},
		{"a project number with its prefix", co2, gitbugio.Issue{ID: id, Title: "SPA routes", Number: "VM-62", Counted: true, Labels: []string{"bug"}}, "fix/vm-62-spa-routes"},
		{"a kind part left empty", config.Branches{Name: "team/{kind}-{number}"}, issue("Crash", "3"), "team/3"},
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
	for _, is := range []gitbugio.Issue{issue("Local only", ""), issue("On GitHub", "2518", "bug"),
		{ID: id, Title: "SPA routes", Number: "VM-62", Counted: true, Labels: []string{"bug"}, Forges: []gitbugio.Forge{{Name: "GitHub", Number: "66"}}}} {
		if name := Name(co2, is); !Names(name, is) {
			t.Errorf("Names(%q) = false for the name it suggests", name)
		}
	}
}

// Once the project numbers its issues, a branch's number is the project's:
// fix/3060-… is CO2-3060's, not GitHub #3060's; an issue the counter hasn't
// numbered yet names its branch by its id.
func TestNamesProjectNumbers(t *testing.T) {
	x := gitbugio.Issue{ID: "aaaaaaaaaa", Title: "Bouncer pool", Number: "CO2-3060", Counted: true, Forges: []gitbugio.Forge{{Name: "GitHub", Number: "3071"}}}
	y := gitbugio.Issue{ID: "bbbbbbbbbb", Title: "CSV export", Number: "CO2-3075", Counted: true, Forges: []gitbugio.Forge{{Name: "GitHub", Number: "3060"}}}
	waiting := gitbugio.Issue{ID: "cccccccccc", Title: "New one", Counted: true, Forges: []gitbugio.Forge{{Name: "GitHub", Number: "3080"}}}
	for _, tc := range []struct {
		branch string
		is     gitbugio.Issue
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
	if got := Name(config.Branches{}, x); got != "co2-3060-bouncer-pool" {
		t.Errorf("Name(x) = %q", got)
	}
	if got := Name(config.Branches{}, waiting); got != "ccccccc-new-one" {
		t.Errorf("Name(waiting) = %q", got)
	}
}
