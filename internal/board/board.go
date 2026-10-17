// Package board loads, sorts and groups issues — the shared core behind the
// CLI's ls/view, the TUI, and the web API. Pulled out of cmd/git-board's
// package main because it now has two callers in two separate binaries.
package board

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/filter"
	"github.com/guilbep/git-board/internal/gitbugio"
)

// DefaultFilter is used when no filter expression is given.
const DefaultFilter = "is:open"

// issueBackend is the subset of gitbugio.Backend that loading needs.
type issueBackend interface {
	ListIssues() ([]gitbugio.Issue, error)
}

// LoadMatching lists issues in b matching filterStr, using cfg for @me/sprint
// alias resolution.
func LoadMatching(b issueBackend, cfg *config.Config, filterStr, me string) ([]gitbugio.Issue, error) {
	issues, err := b.ListIssues()
	if err != nil {
		return nil, err
	}
	return Match(b, cfg, filterStr, me, issues, time.Now())
}

// Match narrows issues already loaded to filterStr, as LoadMatching does:
// the TUI's f, on what it loaded.
func Match(b any, cfg *config.Config, filterStr, me string, issues []gitbugio.Issue, today time.Time) ([]gitbugio.Issue, error) {
	conditions, me, err := prepare(b, filterStr, me)
	if err != nil {
		return nil, err
	}
	withRoles(cfg, issues)
	matched := make([]gitbugio.Issue, 0, len(issues))
	for _, issue := range issues {
		if filter.Matches(issue, conditions, cfg, me, today) {
			matched = append(matched, issue)
		}
	}
	return matched, nil
}

// prepare parses filterStr and finds who @me is. Free text looks in titles,
// descriptions and comments when b has a full-text index (gitbugio.Backend
// has git-bug's); else Matches looks at titles.
func prepare(b any, filterStr, me string) ([]filter.Condition, string, error) {
	conditions := filter.Parse(filterStr)
	if filter.RequiresMe(conditions) && me == "" {
		me = MyLogin(b)
	}
	if filter.RequiresMe(conditions) && me == "" {
		return nil, "", ErrNoMe
	}
	if s, ok := b.(interface {
		SearchText(string) (map[string]bool, error)
	}); ok {
		for i, c := range conditions {
			if c.Kind == filter.KindText {
				var err error
				if conditions[i].Hits, err = s.SearchText(c.Value); err != nil {
					return nil, "", err
				}
			}
		}
	}
	if m, ok := b.(interface {
		Mentioning(string) (map[string]bool, error)
	}); ok {
		for i, c := range conditions {
			if c.Kind != filter.KindInvolves && c.Kind != filter.KindFollows {
				continue
			}
			login := c.Value
			if c.Kind == filter.KindFollows || login == "@me" {
				login = me
			}
			var err error
			if conditions[i].Mentions, err = m.Mentioning(login); err != nil {
				return nil, "", err
			}
		}
	}
	return conditions, me, nil
}

// withRoles gives each issue the roles it takes from the project.
func withRoles(cfg *config.Config, issues []gitbugio.Issue) {
	for i := range issues {
		issues[i].Inherited = cfg.RolesFor(issues[i].Labels)
	}
}

// ErrNoMe is returned when @me means nobody: no --me, and no forge login on
// the git-bug identity.
var ErrNoMe = fmt.Errorf("@me needs your forge login: a bridge (git bug bridge new) tags your git-bug identity with it, or pass --me / set GIT_BOARD_ME")

// MyLogin is what @me means without --me: the forge login of the git-bug
// identity git-board writes as, when b knows it (gitbugio.Backend does).
func MyLogin(b any) string {
	if l, ok := b.(interface{ MyLogin() string }); ok {
		return l.MyLogin()
	}
	return ""
}

// FieldValue returns the value of a namespaced label (e.g. "prio" from
// "prio:p1"), or "" if the issue has no such label.
func FieldValue(issue gitbugio.Issue, fieldName string) string {
	prefix := fieldName + ":"
	for _, l := range issue.AllLabels() {
		if strings.HasPrefix(l, prefix) {
			return strings.TrimPrefix(l, prefix)
		}
	}
	return ""
}

// SortByField sorts issues in place by a "field" or "field desc" spec. A
// no-op if spec is "". Compares numerically when cfg marks the field
// [fields.<name>].numeric = true (e.g. "est"); otherwise in the order of
// its configured options (status follows the workflow), values outside
// them after, lexically. Issues without the field sort last either way —
// the web page sorts the same way (web/src/facets.ts).
func SortByField(issues []gitbugio.Issue, spec string, cfg *config.Config) {
	if spec == "" {
		return
	}
	key, direction, _ := strings.Cut(spec, " ")
	desc := strings.TrimSpace(direction) == "desc"
	var field config.Field
	if cfg != nil {
		field = cfg.Fields[key]
	}
	rank := func(v string) int {
		if i := slices.Index(field.Options, v); i >= 0 {
			return i
		}
		return len(field.Options)
	}
	sort.SliceStable(issues, func(i, j int) bool {
		vi, vj := FieldValue(issues[i], key), FieldValue(issues[j], key)
		switch {
		case field.Numeric || field.Type == config.TypeNumber:
			return lessNumeric(vi, vj, desc)
		case vi == "" || vj == "":
			return vi != "" // a value before none, either way
		case rank(vi) != rank(vj):
			return (rank(vi) < rank(vj)) != desc
		case desc:
			return vi > vj
		}
		return vi < vj
	})
}

// lessNumeric compares two field values as numbers when both parse. A
// value that doesn't parse (typically: the field just isn't set on that
// issue) always sorts last, regardless of direction — "no value" isn't
// meaningfully "less than zero" or "greater than everything", it's just
// absent, so pinning it to one end is more predictable than having it
// swap ends depending on asc/desc.
func lessNumeric(a, b string, desc bool) bool {
	fa, aerr := strconv.ParseFloat(a, 64)
	fb, berr := strconv.ParseFloat(b, 64)
	switch {
	case aerr != nil && berr != nil:
		return false
	case aerr != nil:
		return false // a (no value) never sorts before b
	case berr != nil:
		return true // a (has a value) sorts before b (no value)
	case desc:
		return fa > fb
	default:
		return fa < fb
	}
}

// Bucket is one named group of issues, in first-seen order.
type Bucket struct {
	Group  string           `json:"group"`
	Issues []gitbugio.Issue `json:"issues"`
}

// BucketBy groups issues by the value of a field-derived label. group == ""
// yields a single bucket (Group: "") containing every issue, matching an
// ungrouped view.
func BucketBy(issues []gitbugio.Issue, group string) []Bucket {
	buckets := map[string][]gitbugio.Issue{}
	var order []string
	for _, issue := range issues {
		key := ""
		if group != "" {
			key = FieldValue(issue, group)
			if key == "" {
				key = "(none)"
			}
		}
		if _, seen := buckets[key]; !seen {
			order = append(order, key)
		}
		buckets[key] = append(buckets[key], issue)
	}
	out := make([]Bucket, len(order))
	for i, name := range order {
		out[i] = Bucket{Group: name, Issues: buckets[name]}
	}
	return out
}
