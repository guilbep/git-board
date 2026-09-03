// Package board loads, sorts and groups issues — the shared core behind the
// CLI's ls/view, the TUI, and the web API. Pulled out of cmd/git-board's
// package main because it now has two callers in two separate binaries.
package board

import (
	"fmt"
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
	conditions := filter.Parse(filterStr)
	if filter.RequiresMe(conditions) && me == "" {
		return nil, fmt.Errorf("filter uses @me but no current user configured (--me or GIT_BOARD_ME)")
	}

	issues, err := b.ListIssues()
	if err != nil {
		return nil, err
	}

	today := time.Now()
	matched := make([]gitbugio.Issue, 0, len(issues))
	for _, issue := range issues {
		if filter.Matches(issue, conditions, cfg, me, today) {
			matched = append(matched, issue)
		}
	}
	return matched, nil
}

// FieldValue returns the value of a namespaced label (e.g. "prio" from
// "prio:p1"), or "" if the issue has no such label.
func FieldValue(issue gitbugio.Issue, fieldName string) string {
	prefix := fieldName + ":"
	for _, l := range issue.Labels {
		if strings.HasPrefix(l, prefix) {
			return strings.TrimPrefix(l, prefix)
		}
	}
	return ""
}

// SortByField sorts issues in place by a "field" or "field desc" spec. A
// no-op if spec is "". Compares numerically when cfg marks the field
// [fields.<name>].numeric = true (e.g. "est"), lexically otherwise (fine
// for label fields like "prio", where "p0" < "p1" already sorts right).
func SortByField(issues []gitbugio.Issue, spec string, cfg *config.Config) {
	if spec == "" {
		return
	}
	key, direction, _ := strings.Cut(spec, " ")
	desc := strings.TrimSpace(direction) == "desc"
	numeric := cfg != nil && cfg.Fields[key].Numeric
	sort.SliceStable(issues, func(i, j int) bool {
		vi, vj := FieldValue(issues[i], key), FieldValue(issues[j], key)
		if numeric {
			return lessNumeric(vi, vj, desc)
		}
		if desc {
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
