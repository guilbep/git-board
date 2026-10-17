// Package lint reports drift between git-bug issue labels and
// .git-board.toml's field/sprint configuration. Read-only: it writes
// nothing, unlike everything else in this CLI so far. Five checks — double
// singles, unknown option values, missing values on a single-cardinality
// field, orphan sprint labels, and dead options — see docs/implementation-plan.md's
// M2 plan for the full rationale — and, with [branches] kinds, the kind:
// two kinds, a title that names another, none (checkKinds). A sixth PRD-listed check, "unknown
// assignees", is out of scope: there's no configured roster of valid
// assignees to check labels against here (unlike the other checks, which
// all validate against .git-board.toml).
package lint

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio"
)

// Check names, used to group Findings for CLI output.
const (
	CheckDoubleSingle    = "double-single"
	CheckUnknownValue    = "unknown-value"
	CheckMissingRequired = "missing-value"
	CheckOrphanSprint    = "orphan-sprint"
	CheckDeadOption      = "dead-option"
	CheckTwoKinds        = "two-kinds"
	CheckKindTitle       = "kind-title"
	CheckNoKind          = "no-kind"
)

// CheckOrder is every check name in the order Run reports them, for a
// stable CLI summary grouping.
var CheckOrder = []string{CheckDoubleSingle, CheckUnknownValue, CheckMissingRequired, CheckOrphanSprint, CheckDeadOption,
	CheckTwoKinds, CheckKindTitle, CheckNoKind}

// Finding is one drift report. IssueID/Title are "" for CheckDeadOption,
// which is a config-inventory question ("does any issue use this option"),
// not a per-issue one.
type Finding struct {
	Check   string
	IssueID string
	Title   string
	Message string
}

// Run checks issues against cfg. Checks 1-4 (double singles, unknown
// values, missing required values, orphan sprints) only look at open
// issues — a closed issue doesn't need lint attention. Check 5 (dead
// options) deliberately looks at every issue including closed ones: it's
// asking whether a configured option is used *anywhere* in the repo, and an
// option that only appears on closed issues (e.g. a terminal "validated"
// status) is exactly the "used" case, not the "dead" one.
func Run(issues []gitbugio.Issue, cfg *config.Config) []Finding {
	open := make([]gitbugio.Issue, 0, len(issues))
	for _, issue := range issues {
		if issue.Status == "open" {
			open = append(open, issue)
		}
	}

	var findings []Finding
	findings = append(findings, checkSingleCardinality(open, cfg)...)
	findings = append(findings, checkUnknownValues(open, cfg)...)
	findings = append(findings, checkOrphanSprints(open, cfg)...)
	findings = append(findings, checkDeadOptions(issues, cfg)...)
	findings = append(findings, checkKinds(open, cfg)...)
	return findings
}

func sortedSingleFields(cfg *config.Config) []string {
	names := make([]string, 0, len(cfg.Fields))
	for name, f := range cfg.Fields {
		if f.Single {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// checkSingleCardinality reports two drifts from one pass over each
// single-cardinality configured field on each open issue: more than one
// label for it (structurally rare — fields.Apply's swap logic prevents it
// going forward — but concurrent git-bug operations from two people could
// still produce it), and none at all (the PRD calls out "prio"
// specifically; every configured single field generalizes for free here,
// and the PRD's own drift numbers show empty status alongside empty
// priority, not just one field).
func checkSingleCardinality(issues []gitbugio.Issue, cfg *config.Config) []Finding {
	fieldNames := sortedSingleFields(cfg)
	var findings []Finding
	for _, issue := range issues {
		for _, name := range fieldNames {
			prefix := name + ":"
			var matches []string
			for _, l := range issue.Labels {
				if strings.HasPrefix(l, prefix) {
					matches = append(matches, l)
				}
			}
			switch {
			case len(matches) == 0:
				findings = append(findings, Finding{
					Check: CheckMissingRequired, IssueID: issue.ID, Title: issue.Title,
					Message: fmt.Sprintf("no %s value", name),
				})
			case len(matches) > 1:
				findings = append(findings, Finding{
					Check: CheckDoubleSingle, IssueID: issue.ID, Title: issue.Title,
					Message: fmt.Sprintf("multiple %s labels: %s", name, strings.Join(matches, ", ")),
				})
			}
		}
	}
	return findings
}

// checkUnknownValues reuses config.Field.KnownValue — the exact
// value-against-Options check internal/ghbridge already applies (and warns
// about) when a `bridge pull` value doesn't map to a configured option.
// KnownValue treats an unconfigured field, or a configured one with no
// Options, as "known" (nothing to validate against) — that's how a sprint
// label (validated separately, against [[sprints.list]]) or a free-form
// field like assignee falls through here untouched.
func checkUnknownValues(issues []gitbugio.Issue, cfg *config.Config) []Finding {
	var findings []Finding
	for _, issue := range issues {
		for _, l := range issue.Labels {
			name, value, ok := strings.Cut(l, ":")
			if !ok || cfg.Fields[name].KnownValue(value) {
				continue
			}
			findings = append(findings, Finding{
				Check: CheckUnknownValue, IssueID: issue.ID, Title: issue.Title,
				Message: fmt.Sprintf("%s has no matching [fields.%s] option", l, name),
			})
		}
	}
	return findings
}

func checkOrphanSprints(issues []gitbugio.Issue, cfg *config.Config) []Finding {
	known := make(map[string]bool, len(cfg.Sprints.List))
	for _, s := range cfg.Sprints.List {
		known[s.Slug] = true
	}

	var findings []Finding
	for _, issue := range issues {
		for _, l := range issue.Labels {
			slug, ok := strings.CutPrefix(l, "sprint:")
			if !ok || known[slug] {
				continue
			}
			findings = append(findings, Finding{
				Check: CheckOrphanSprint, IssueID: issue.ID, Title: issue.Title,
				Message: fmt.Sprintf("%s has no matching [[sprints.list]] entry", l),
			})
		}
	}
	return findings
}

// checkDeadOptions is the inverse of checkUnknownValues: it scans the
// configured *options* against the issue *labels* actually in use, not the
// other way around, so it takes every issue (including closed ones — see
// Run's doc comment) rather than the open-only slice the other checks share.
func checkDeadOptions(issues []gitbugio.Issue, cfg *config.Config) []Finding {
	used := make(map[string]bool)
	for _, issue := range issues {
		for _, l := range issue.Labels {
			used[l] = true
		}
	}

	names := make([]string, 0, len(cfg.Fields))
	for name := range cfg.Fields {
		names = append(names, name)
	}
	sort.Strings(names)

	var findings []Finding
	for _, name := range names {
		for _, opt := range cfg.Fields[name].Options {
			label := name + ":" + opt
			if used[label] {
				continue
			}
			findings = append(findings, Finding{
				Check:   CheckDeadOption,
				Message: fmt.Sprintf("%s: no issue carries it", label),
			})
		}
	}
	return findings
}

var titleKind = regexp.MustCompile(`^\s*\[([^\]]+)\]`) // [BUG](Equipment)

// checkKinds reads the kinds [branches] kinds declares as flat labels (a
// kind: field is a field, which the checks above already see): two kinds
// on one issue; a title prefix naming another kind than the label, which
// wins; and the issues with no kind, as one count, a missing kind being
// the issue template's to fix rather than each issue's.
func checkKinds(issues []gitbugio.Issue, cfg *config.Config) []Finding {
	kinds := cfg.Branches.Kinds
	if len(kinds) == 0 {
		return nil
	}
	var findings []Finding
	missing := 0
	for _, issue := range issues {
		var flat []string
		for _, l := range issue.Labels {
			if _, ok := kinds[l]; ok {
				flat = append(flat, l)
			}
		}
		if len(flat) > 1 {
			sort.Strings(flat)
			findings = append(findings, Finding{Check: CheckTwoKinds, IssueID: issue.ID, Title: issue.Title,
				Message: fmt.Sprintf("kinds %s: keep one", strings.Join(flat, ", "))})
		}
		kind := cfg.Branches.Kind(issue.Labels)
		if kind == "" {
			missing++
			continue
		}
		m := titleKind.FindStringSubmatch(issue.Title)
		if m == nil {
			continue
		}
		named := strings.TrimSpace(m[1])
		for k := range kinds {
			if strings.EqualFold(k, named) && k != kind {
				findings = append(findings, Finding{Check: CheckKindTitle, IssueID: issue.ID, Title: issue.Title,
					Message: fmt.Sprintf("the title says [%s], the label says %s: the label wins", named, kind)})
			}
		}
	}
	if missing > 0 {
		have := "issues have"
		if missing == 1 {
			have = "issue has"
		}
		findings = append(findings, Finding{Check: CheckNoKind,
			Message: fmt.Sprintf("%d open %s none of [branches] kinds (%s)", missing, have, strings.Join(slices.Sorted(maps.Keys(kinds)), ", "))})
	}
	return findings
}
