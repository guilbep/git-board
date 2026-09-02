// Package board loads, sorts and groups issues — the shared core behind the
// CLI's ls/view, the TUI, and the web API. Pulled out of cmd/git-board's
// package main because it now has two callers in two separate binaries.
package board

import (
	"fmt"
	"sort"
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

// SortByField sorts issues in place by a "field" or "field desc" spec.
// A no-op if spec is "". ponytail: string sort — fine for label fields like
// "prio" (the only sort a shipped view uses); wrong for numeric fields like
// "est" ("16" < "2"). Add a numeric-field lookup from config if that's ever
// sorted.
func SortByField(issues []gitbugio.Issue, spec string) {
	if spec == "" {
		return
	}
	key, direction, _ := strings.Cut(spec, " ")
	desc := strings.TrimSpace(direction) == "desc"
	sort.SliceStable(issues, func(i, j int) bool {
		vi, vj := FieldValue(issues[i], key), FieldValue(issues[j], key)
		if desc {
			return vi > vj
		}
		return vi < vj
	})
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
