package app

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/guilbep/git-board/conf"
	"github.com/guilbep/git-board/domain/cycle"
	"github.com/guilbep/git-board/domain/issues"
	"github.com/guilbep/git-board/forge"
	"github.com/guilbep/git-board/tools/ghproject"
)

// git board lint, as v1's internal/lint: drift between the issues' labels
// and .git-board.toml, read-only.

// Check names, used to group Findings for CLI output.
const (
	CheckDoubleSingle    = "double-single"
	CheckUnknownValue    = "unknown-value"
	CheckNearValue       = "near-value"
	CheckMissingRequired = "missing-value"
	CheckOrphanSprint    = "orphan-sprint"
	CheckDeadOption      = "dead-option"
	CheckTwoKinds        = "two-kinds"
	CheckKindTitle       = "kind-title"
	CheckNoKind          = "no-kind"
	CheckLabelOutside    = "label-outside"
	CheckProjectKey      = "project-key"
	CheckKindPrefix      = "kind-prefix"
	CheckTemplate        = "template"
	CheckUnknownKey      = "unknown-key"
)

// LintOrder is every check name in the order Run reports them, for a
// stable CLI summary grouping.
var LintOrder = []string{CheckUnknownKey, CheckDoubleSingle, CheckUnknownValue, CheckNearValue, CheckMissingRequired, CheckOrphanSprint, CheckDeadOption,
	CheckProjectKey, CheckKindPrefix, CheckTemplate, CheckTwoKinds, CheckKindTitle, CheckNoKind, CheckLabelOutside}

// LintFinding is one drift report. IssueID/Title are "" for CheckDeadOption,
// which is a config-inventory question ("does any issue use this option"),
// not a per-issue one.
type LintFinding struct {
	Check   string `json:"check"`
	IssueID string `json:"issue_id"`
	Title   string `json:"title"`
	Message string `json:"message"`
}

// Lint checks the issues against cfg, as v1's git board lint (internal/lint). Checks 1-4 (double singles, unknown
// values, missing required values, orphan sprints) only look at open
// issues — a closed issue doesn't need lint attention. Check 5 (dead
// options) deliberately looks at every issue including closed ones: it's
// asking whether a configured option is used *anywhere* in the repo, and an
// option that only appears on closed issues (e.g. a terminal "validated"
// status) is exactly the "used" case, not the "dead" one.
func Lint(all []issues.Issue, cfg *Config) []LintFinding {
	open := make([]issues.Issue, 0, len(all))
	for _, issue := range all {
		if issue.Status == "open" {
			open = append(open, issue)
		}
	}

	var findings []LintFinding
	findings = append(findings, checkUnknownKeys(cfg)...)
	findings = append(findings, checkSingleCardinality(open, cfg)...)
	findings = append(findings, checkUnknownValues(open, cfg)...)
	findings = append(findings, checkNearValues(open, cfg)...)
	findings = append(findings, checkOrphanSprints(open, cfg)...)
	findings = append(findings, checkDeadOptions(all, cfg)...)
	findings = append(findings, checkProjectKeys(cfg)...)
	findings = append(findings, checkKindPrefixes(cfg)...)
	findings = append(findings, checkTemplate(cfg)...)
	findings = append(findings, checkKinds(open, cfg)...)
	findings = append(findings, checkLabels(open, cfg)...)
	return findings
}

// checkLabels: with a [labels] table, an open issue's plain labels that
// aren't in its list (a kind, in the flat form, is the kind field's),
// each with what to do.
func checkLabels(issues []issues.Issue, cfg *Config) []LintFinding {
	if !cfg.Schema.Labels.Defined {
		return nil
	}
	var findings []LintFinding
	for _, issue := range issues {
		for _, l := range issue.Labels {
			_, kind := cfg.Schema.Kinds[l]
			if strings.Contains(l, ":") || kind || slices.Contains(cfg.Schema.Labels.List, l) {
				continue
			}
			findings = append(findings, LintFinding{Check: CheckLabelOutside, IssueID: issue.ID, Title: issue.Title,
				Message: fmt.Sprintf("label %q isn't in [labels] list: add it there, or remove it", l)})
		}
	}
	return findings
}

func sortedSingleFields(cfg *Config) []string {
	names := make([]string, 0, len(cfg.Schema.Fields))
	for name, f := range cfg.Schema.Fields {
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
// priority, not just one field). With [branches] kinds the kind is
// checkKinds', in either form; no hold is the usual, not on hold.
func checkSingleCardinality(issues []issues.Issue, cfg *Config) []LintFinding {
	fieldNames := slices.DeleteFunc(sortedSingleFields(cfg), func(name string) bool {
		return name == "kind" && len(cfg.Schema.Kinds) > 0
	})
	var findings []LintFinding
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
			case len(matches) == 0 && name != "hold":
				findings = append(findings, LintFinding{
					Check: CheckMissingRequired, IssueID: issue.ID, Title: issue.Title,
					Message: fmt.Sprintf("no %s value", name),
				})
			case len(matches) > 1:
				findings = append(findings, LintFinding{
					Check: CheckDoubleSingle, IssueID: issue.ID, Title: issue.Title,
					Message: fmt.Sprintf("multiple %s labels: %s", name, strings.Join(matches, ", ")),
				})
			}
		}
	}
	return findings
}

// checkUnknownValues reuses fields.Field.KnownValue — the exact
// value-against-Options check tools/ghproject already applies (and warns
// about) when a `project pull` value doesn't map to a configured option.
// KnownValue treats an unconfigured field, or a configured one with no
// Options, as "known" (nothing to validate against) — that's how a sprint
// label (validated separately, against [[sprints.list]]) or a free-form
// field like assignee falls through here untouched.
func checkUnknownValues(issues []issues.Issue, cfg *Config) []LintFinding {
	var findings []LintFinding
	for _, issue := range issues {
		for _, l := range issue.Labels {
			name, value, ok := strings.Cut(l, ":")
			if !ok || cfg.Schema.Fields[name].KnownValue(value) {
				continue
			}
			msg := fmt.Sprintf("%s has no matching [fields.%s] option", l, name)
			if i := slices.IndexFunc(cfg.Schema.Fields[name].Options, func(o string) bool { return fold(o) == fold(value) }); i >= 0 {
				msg += fmt.Sprintf(": did you mean %s:%s?", name, cfg.Schema.Fields[name].Options[i])
			}
			findings = append(findings, LintFinding{Check: CheckUnknownValue, IssueID: issue.ID, Title: issue.Title, Message: msg})
		}
	}
	return findings
}

// fold is a value as a typo leaves it: lower case, letters, digits and the
// point only (-normal and normal, P-2 and p2 are one; 1.5 and 15 are not)
func fold(v string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.' {
			return r
		}
		if r >= 'A' && r <= 'Z' {
			return r + 'a' - 'A'
		}
		return -1
	}, v)
}

// checkNearValues: on a field with no options (undeclared, or declared
// without a list), two spellings of one value are a typo. The spelling
// most open issues carry is the meant one; each issue carrying another is
// told so. A field with options is checkUnknownValues'.
func checkNearValues(issues []issues.Issue, cfg *Config) []LintFinding {
	uses := map[string]map[string]int{} // field+fold -> spelling -> issues
	for _, issue := range issues {
		for _, l := range issue.Labels {
			name, value, ok := strings.Cut(l, ":")
			if !ok || len(cfg.Schema.Fields[name].Options) > 0 || fold(value) == "" {
				continue
			}
			k := name + ":" + fold(value)
			if uses[k] == nil {
				uses[k] = map[string]int{}
			}
			uses[k][value]++
		}
	}
	var findings []LintFinding
	for _, issue := range issues {
		for _, l := range issue.Labels {
			name, value, ok := strings.Cut(l, ":")
			if !ok {
				continue
			}
			spellings := uses[name+":"+fold(value)]
			if len(spellings) < 2 {
				continue
			}
			// the most used, then the shortest, then the first in order
			meant := slices.SortedFunc(maps.Keys(spellings), func(a, b string) int {
				if spellings[a] != spellings[b] {
					return spellings[b] - spellings[a]
				}
				if len(a) != len(b) {
					return len(a) - len(b)
				}
				return strings.Compare(a, b)
			})[0]
			if value == meant {
				continue
			}
			n := spellings[meant]
			findings = append(findings, LintFinding{Check: CheckNearValue, IssueID: issue.ID, Title: issue.Title,
				Message: fmt.Sprintf("%s: did you mean %s:%s (%d issue%s)?", l, name, meant, n, map[bool]string{true: "", false: "s"}[n == 1])})
		}
	}
	return findings
}

func checkOrphanSprints(issues []issues.Issue, cfg *Config) []LintFinding {
	known := make(map[string]bool, len(cfg.Schema.Sprints.List))
	for _, s := range cfg.Schema.Sprints.List {
		known[s.Slug] = true
	}

	var findings []LintFinding
	for _, issue := range issues {
		for _, l := range issue.Labels {
			slug, ok := strings.CutPrefix(l, "sprint:")
			if !ok || known[slug] {
				continue
			}
			findings = append(findings, LintFinding{
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
func checkDeadOptions(issues []issues.Issue, cfg *Config) []LintFinding {
	used := make(map[string]bool)
	for _, issue := range issues {
		for _, l := range issue.Labels {
			used[l] = true
		}
	}

	names := make([]string, 0, len(cfg.Schema.Fields))
	for name := range cfg.Schema.Fields {
		names = append(names, name)
	}
	sort.Strings(names)

	var findings []LintFinding
	for _, name := range names {
		for _, opt := range cfg.Schema.Fields[name].Options {
			label := name + ":" + opt
			if used[label] {
				continue
			}
			findings = append(findings, LintFinding{
				Check:   CheckDeadOption,
				Message: fmt.Sprintf("%s: no issue carries it", label),
			})
		}
	}
	return findings
}

// namedPrefix is the branch prefix a label's description names: "should
// prefix branch by bug/", ENAC's labels' way.
var namedPrefix = regexp.MustCompile(`(?i)prefix(?:es)? (?:the |a |your )?branch(?:es)? (?:by|with) ([\w.-]+)/`)

// checkKindPrefixes: a label's description naming another branch prefix
// than the label itself. A branch starts with its kind's label, always
// (config's checkKindWords), so "prefix branch by fix/" on bug is wrong.
// One finding per label, no issue.
func checkKindPrefixes(cfg *Config) []LintFinding {
	var findings []LintFinding
	check := func(label, d string) {
		if m := namedPrefix.FindStringSubmatch(d); m != nil && !strings.EqualFold(m[1], label) {
			findings = append(findings, LintFinding{Check: CheckKindPrefix,
				Message: fmt.Sprintf("%s's description names the branch prefix %s/: its branches start %s/, fix the description", label, m[1], label)})
		}
	}
	for _, name := range cfg.Schema.FieldOrder() {
		ds := cfg.Schema.Fields[name].Descriptions
		for _, v := range slices.Sorted(maps.Keys(ds)) {
			check(labelOf(cfg.Schema, name, v), ds[v])
		}
	}
	for _, l := range slices.Sorted(maps.Keys(cfg.Schema.Labels.Descriptions)) {
		check(l, cfg.Schema.Labels.Descriptions[l])
	}
	return findings
}

var titleKind = regexp.MustCompile(`^\s*\[([^\]]+)\]`) // [BUG](Equipment)

// checkUnknownKeys names the keys of .git-board.toml git-board doesn't
// read (config.Load): a misspelled one ([github-project] wirte) is its
// zero value, and nothing else says so but a warning on stderr.
func checkUnknownKeys(cfg *Config) []LintFinding {
	var findings []LintFinding
	for _, k := range cfg.File.Unknown() {
		findings = append(findings, LintFinding{Check: CheckUnknownKey,
			Message: fmt.Sprintf("%s: git-board doesn't know this key, it is ignored (misspelled?)", k)})
	}
	return findings
}

// checkProjectKeys suggests the forge field mappings for the fields still
// renaming values the old way: [fields.X] project is read as
// [forges.gh.fields.X.values] since the mappings exist, but the new form
// is the one the Forges page edits. A finding per field, no issue.
func checkProjectKeys(cfg *Config) []LintFinding {
	var findings []LintFinding
	for _, name := range slices.Sorted(maps.Keys(cfg.Schema.Fields)) {
		if len(cfg.Schema.Fields[name].Project) > 0 {
			findings = append(findings, LintFinding{Check: CheckProjectKey,
				Message: fmt.Sprintf("fields.%s.project: move it to [forges.gh.fields.%s.values] — where each field lives on a forge (Settings › Forges › Fields)", name, name)})
		}
	}
	return findings
}

// checkKinds reads the kinds with [branches] kinds, either form (kind:bug,
// or bug, which the backend reads as kind:bug): two kinds on one issue; a
// title starting with a bracket ([BUG]), which a kind never needs, the
// label winning when they disagree; and the issues with no kind, as one
// count, a missing kind being the issue template's to fix rather than each
// issue's.
func checkKinds(issues []issues.Issue, cfg *Config) []LintFinding {
	kinds := cfg.Schema.Kinds
	if len(kinds) == 0 {
		return nil
	}
	var findings []LintFinding
	missing := 0
	for _, issue := range issues {
		var has []string
		for _, l := range issue.Labels {
			k, field := strings.CutPrefix(l, "kind:")
			if _, flat := kinds[l]; field || flat {
				has = append(has, k)
			}
		}
		if has = slices.Compact(slices.Sorted(slices.Values(has))); len(has) > 1 {
			findings = append(findings, LintFinding{Check: CheckTwoKinds, IssueID: issue.ID, Title: issue.Title,
				Message: fmt.Sprintf("kinds %s: keep one", strings.Join(has, ", "))})
		}
		kind := kindOf(cfg.Schema.Kinds, issue.Labels)
		if kind == "" {
			missing++
			continue
		}
		m := titleKind.FindStringSubmatch(issue.Title)
		if m == nil {
			continue
		}
		named := strings.TrimSpace(m[1])
		msg := fmt.Sprintf("the title starts with [%s]: the kind is its label, %s", named, kind)
		for k := range kinds {
			if strings.EqualFold(k, named) && k != kind {
				msg = fmt.Sprintf("the title says [%s], the label says %s: the label wins", named, kind)
			}
		}
		findings = append(findings, LintFinding{Check: CheckKindTitle, IssueID: issue.ID, Title: issue.Title, Message: msg})
	}
	if missing > 0 {
		have := "issues have"
		if missing == 1 {
			have = "issue has"
		}
		findings = append(findings, LintFinding{Check: CheckNoKind,
			Message: fmt.Sprintf("%d open %s none of [branches] kinds (%s)", missing, have, strings.Join(slices.Sorted(maps.Keys(kinds)), ", "))})
	}
	return findings
}

// checkTemplate: what the built-in template the file names ([template])
// has that the file lacks, its fields and their values, and a newer
// version. What the file has more, or spells or colors otherwise (the
// forge's), is the project's, and so are the values of a field its GitHub
// Project holds.
// ponytail: built-in templates only; a file or URL template isn't fetched at lint
func checkTemplate(cfg *Config) []LintFinding {
	t, _ := cfg.Tools.Template.Get()
	name := t.Name
	text, err := starter(name)
	if name == "" || err != nil {
		return nil
	}
	f, err := conf.Parse(conf.FileName, []byte(text))
	if err != nil {
		return nil
	}
	tmpl, err := readConfig(f, nil)
	if err != nil {
		return nil
	}
	var findings []LintFinding
	add := func(format string, a ...any) {
		findings = append(findings, LintFinding{Check: CheckTemplate, Message: fmt.Sprintf(format, a...)})
	}
	if tt, _ := tmpl.Tools.Template.Get(); tt.Version > t.Version {
		add("this file is from template %s version %d; %s is at version %d", name, t.Version, name, tt.Version)
	}
	gp, _ := cfg.Tools.GithubProject.Get()
	inProject := func(field string) bool {
		return len(cfg.Schema.Fields[field].Project) > 0 || gp.Number != 0 && slices.Contains(projectFields, field)
	}
	for _, field := range tmpl.Schema.FieldOrder() {
		have, ok := cfg.Schema.Fields[field]
		if !ok {
			add("%s: template %s has this field, this file doesn't", field, name)
			continue
		}
		if gp.Number != 0 && forge.Target(cfg.Schema, inProject, githubForge(cfg.Forges), field) == forge.TargetProject {
			continue // its values are the Project's, which init read
		}
		var missing []string
		for _, o := range tmpl.Schema.Fields[field].Options {
			if !slices.ContainsFunc(have.Options, func(h string) bool { return ghproject.Slug(h) == ghproject.Slug(o) }) {
				missing = append(missing, o)
			}
		}
		if len(missing) > 0 {
			add("%s: template %s has %s, this file doesn't", field, name, strings.Join(missing, ", "))
		}
	}
	return findings
}

// githubForge is the file's GitHub forge table, else github.com's.
func githubForge(fs forge.Forges) forge.Table {
	for _, f := range fs.List {
		if f.Kind == "github" {
			return f
		}
	}
	return forge.Table{Name: "gh", Kind: "github", Host: "github.com"}
}

// kindOf is an issue's kind with [branches] kinds: kind:<v>, else the
// first flat kind label in order.
func kindOf(kinds map[string]string, labels []string) string {
	for _, l := range labels {
		if v, ok := strings.CutPrefix(l, "kind:"); ok {
			return v
		}
	}
	for _, l := range slices.Sorted(slices.Values(labels)) {
		if _, ok := kinds[l]; ok {
			return l
		}
	}
	return ""
}

// labelOf is a field's value as a label: a flat kind is its bare label.
func labelOf(s *issues.Schema, field, value string) string {
	if _, ok := s.Kinds[value]; ok && field == "kind" && s.FlatKinds() {
		return value
	}
	return field + ":" + value
}

// LintReads is how today's forms of [cycle] were read, so a project can
// write the new ones when it wants to: with the cycle on, else none.
func LintReads(cfg *Config) []string {
	if !cfg.On(cycle.Feature) {
		return nil
	}
	c := cfg.Cycle
	var out []string
	if c.Steps == nil {
		with := ""
		if len(c.Signoffs) > 0 {
			with = " with a sign-off per validation status"
		}
		var ids []string
		for _, s := range c.Stages(cfg.Schema.Fields["status"].Options).Steps {
			ids = append(ids, s.ID)
		}
		out = append(out, fmt.Sprintf("steps: none, so the default list%s: %s", with, strings.Join(ids, ", ")))
	}
	for _, id := range slices.Sorted(maps.Keys(c.Signoffs)) {
		role := c.Signoffs[id]
		out = append(out, fmt.Sprintf("[cycle.signoffs] %s = %q: the sign-off step %s, at status %s, held by the role %s", id, role, id, id, role))
	}
	for _, kind := range slices.Sorted(maps.Keys(c.KindsFile)) {
		if cfg.File.Type("cycle", "kinds", kind) != "String" {
			continue
		}
		r := c.Kinds[kind]
		out = append(out, fmt.Sprintf("[cycle.kinds] %s = %q: [cycle.kinds.%s] tests = %q", kind, r, kind, r))
	}
	return out
}
