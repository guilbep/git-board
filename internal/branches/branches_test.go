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
	for _, is := range []gitbugio.Issue{issue("Local only", ""), issue("On GitHub", "2518", "bug")} {
		if name := Name(co2, is); !Names(name, is) {
			t.Errorf("Names(%q) = false for the name it suggests", name)
		}
	}
}
