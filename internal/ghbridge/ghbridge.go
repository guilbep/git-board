// Package ghbridge bridges GitHub Project (v2) custom field values into
// git-board's ns:value label model, via `gh project item-list`. Read-only
// against GitHub: it only shells out to `gh project item-list`, never
// writes back to the Project or to GitHub issues — all writes land on the
// local git-bug repo via internal/gitbugio, same as `git-board set`.
//
// Real shape captured from `gh project item-list 40 --owner EPFL-ENAC
// --format json` (2026-09-02):
//
//	{"items":[{"assignees":["BenBotros"],"content":{"body":"...","number":2605,
//	"repository":"EPFL-ENAC/co2-calculator","title":"...","type":"Issue",
//	"url":"https://github.com/EPFL-ENAC/co2-calculator/issues/2605"},
//	"id":"PVTI_...","labels":["bug"],"linked pull requests":["https://.../pull/2633"],
//	"priority":"P0","repository":"https://github.com/EPFL-ENAC/co2-calculator",
//	"sprint":{"duration":14,"iterationId":"912a52aa","startDate":"2026-08-24",
//	"title":"Delivery"},"status":"QA Validation","module":"BackOffice",
//	"category":"FEAT","est. hours":2,"title":"..."}]}
//
// "milestone" and "size" were never observed with a value in a live pull
// (PRD calls them dead/unused) — they're simply not decoded here, so any
// value in either is silently ignored rather than mapped to a label.
// content.type is "Issue" in every observed row; "PullRequest" and
// "DraftIssue" are documented GitHub Project item content types (a PR
// added straight to the project, not just linked from an issue) and are
// excluded defensively even though not seen live — testdata/project-items.json
// includes a synthetic PullRequest row to exercise that path.
package ghbridge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/fields"
	"github.com/guilbep/git-board/internal/gitbugio"
)

// Item is one row of `gh project item-list --format json`'s "items" array.
type Item struct {
	Assignees []string `json:"assignees"`
	Content   Content  `json:"content"`
	Priority  string   `json:"priority"`
	Status    string   `json:"status"`
	Module    string   `json:"module"`
	Category  string   `json:"category"`
	EstHours  *float64 `json:"est. hours"`
	Sprint    *Sprint  `json:"sprint"`
	Title     string   `json:"title"`
}

// Content is a project item's linked issue/PR/draft.
type Content struct {
	Number int    `json:"number"`
	Type   string `json:"type"` // "Issue", "PullRequest", or "DraftIssue"
	URL    string `json:"url"`
}

// Sprint is the project's iteration field, if the item has one set.
type Sprint struct {
	Title string `json:"title"`
}

type listResponse struct {
	Items []Item `json:"items"`
}

// ParseItems decodes `gh project item-list --format json`'s output.
func ParseItems(data []byte) ([]Item, error) {
	var resp listResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing gh project item-list JSON: %w", err)
	}
	return resp.Items, nil
}

// FetchItems shells out to `gh project item-list`, read-only (the `gh` CLI
// is assumed already authenticated with at least read:project scope).
func FetchItems(owner string, number, limit int) ([]Item, error) {
	cmd := exec.Command("gh", "project", "item-list", strconv.Itoa(number),
		"--owner", owner, "--format", "json", "--limit", strconv.Itoa(limit))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gh project item-list --owner %s %d: %w: %s", owner, number, err, stderr.String())
	}
	return ParseItems(out)
}

// FieldResult is one field's planned label change for one issue.
type FieldResult struct {
	Field   string
	Value   string // the normalized value that was (or would be) set
	Added   []string
	Removed []string
}

// IssueChange is the full set of planned label changes for one matched
// GitHub Project item / git-bug bug pair.
type IssueChange struct {
	BugID     string
	GithubURL string
	Number    int
	Title     string
	Fields    []FieldResult
	Added     []string // union of every field's Added, for ApplyPlan/summary
	Removed   []string // union of every field's Removed
	Warnings  []string // e.g. a status value with no matching [fields.status] option
}

// UnmatchedItem is a project item that couldn't be turned into a label
// change: not an Issue (a PR or draft added straight to the project), or
// an Issue with no corresponding git-bug bug (not bridged in yet).
type UnmatchedItem struct {
	Number     int
	URL        string
	Title      string
	Reason     string
	NotAnIssue bool // true for a PR/draft excluded outright; false for an Issue with no matching bug
}

// Plan is the full result of BuildPlan: what would change (or, once
// applied via ApplyPlan, did change) for every matched issue, plus what
// couldn't be matched at all.
type Plan struct {
	Changes   []IssueChange
	Unmatched []UnmatchedItem
}

// BuildPlan maps each project item's fields onto git-bug labels for the
// matching bug (matched by GitHub issue URL — see gitbugio.Issue.GithubURL)
// and computes the add/remove label diff, without writing anything. Pure
// and side-effect-free: safe to call for a --dry-run preview, and ApplyPlan
// reuses its output for the real write.
func BuildPlan(items []Item, bugs []gitbugio.Issue, cfg *config.Config) Plan {
	byURL := make(map[string]gitbugio.Issue, len(bugs))
	for _, b := range bugs {
		if b.GithubURL != "" {
			byURL[b.GithubURL] = b
		}
	}

	var plan Plan
	for _, item := range items {
		if item.Content.Type != "Issue" {
			plan.Unmatched = append(plan.Unmatched, UnmatchedItem{
				Number:     item.Content.Number,
				URL:        item.Content.URL,
				Title:      item.Title,
				Reason:     fmt.Sprintf("not an issue (content type %s), excluded", item.Content.Type),
				NotAnIssue: true,
			})
			continue
		}

		bug, ok := byURL[item.Content.URL]
		if !ok {
			plan.Unmatched = append(plan.Unmatched, UnmatchedItem{
				Number: item.Content.Number,
				URL:    item.Content.URL,
				Title:  item.Title,
				Reason: "no matching git-bug bug (not bridged in via `git-bug bridge pull` yet?)",
			})
			continue
		}

		plan.Changes = append(plan.Changes, planIssue(item, bug, cfg))
	}
	return plan
}

func planIssue(item Item, bug gitbugio.Issue, cfg *config.Config) IssueChange {
	change := IssueChange{
		BugID:     bug.ID,
		GithubURL: bug.GithubURL,
		Number:    item.Content.Number,
		Title:     item.Title,
	}

	add := func(res FieldResult, warning string) {
		switch {
		case warning != "":
			change.Warnings = append(change.Warnings, warning)
		case res.Field != "":
			change.Fields = append(change.Fields, res)
			change.Added = append(change.Added, res.Added...)
			change.Removed = append(change.Removed, res.Removed...)
		}
		// res.Field == "" && warning == "": source value was empty, field
		// left untouched — expected GitHub Project drift, not an error.
	}

	add(planSingleValue(cfg, bug.Labels, "status", slugify(item.Status)))
	add(planSingleValue(cfg, bug.Labels, "prio", slugify(item.Priority)))
	add(planPlainLabel(bug.Labels, "category", item.Category))
	add(planSingleValue(cfg, bug.Labels, "mod", slugify(normalizeModule(item.Module))))
	add(planSingleValue(cfg, bug.Labels, "est", formatEstHours(item.EstHours)))
	add(planSprint(cfg, bug.Labels, sprintTitle(item)))

	assigneeRes := planAssignees(bug.Labels, item.Assignees)
	if len(assigneeRes.Added) > 0 || len(assigneeRes.Removed) > 0 {
		change.Fields = append(change.Fields, assigneeRes)
		change.Added = append(change.Added, assigneeRes.Added...)
		change.Removed = append(change.Removed, assigneeRes.Removed...)
	}

	return change
}

// planSingleValue plans one single-cardinality field. An empty value
// (returned FieldResult.Field == "", no warning) means "leave untouched" —
// GitHub Project drift like the PRD's documented empty-status/empty-priority
// items is expected, not an error. A non-empty value that doesn't match a
// configured [fields.<name>] option list produces a warning instead of a
// label, matching git-bug's "no silent fallbacks" rule.
func planSingleValue(cfg *config.Config, current []string, name, value string) (FieldResult, string) {
	if value == "" {
		return FieldResult{}, ""
	}
	if !cfg.Fields[name].KnownValue(value) {
		return FieldResult{}, fmt.Sprintf("%s value %q has no matching [fields.%s] option, skipped", name, value, name)
	}
	_, added, removed := fields.Diff(current, name, value, true)
	return FieldResult{Field: name, Value: value, Added: added, Removed: removed}, ""
}

// planPlainLabel adds value as a bare, unnamespaced label — not a
// field:value pair — for classifications that are just plain git-bug/
// GitHub labels, not a structured single-cardinality field (Category:
// GitHub's own native issue labels already do this job, no need for a
// second cat:value mechanism duplicating them). Additive only, matching
// git-bug's own native-label import: a bare label carries no "this came
// from field X" marker, so there's nothing to safely remove if the
// source value changes on a later re-run — same tradeoff plain labels
// have always had, not new here. Empty value is a no-op, matching every
// other field's "leave untouched" convention. Not slugified — a plain
// label should look like the other plain labels on the issue (whatever
// casing GitHub used), not the hyphenated form the structured-field
// option-matching convention needs.
func planPlainLabel(current []string, reportAs, value string) (FieldResult, string) {
	if value == "" {
		return FieldResult{}, ""
	}
	for _, l := range current {
		if l == value {
			return FieldResult{}, "" // already present, no-op
		}
	}
	return FieldResult{Field: reportAs, Value: value, Added: []string{value}}, ""
}

func sprintTitle(item Item) string {
	if item.Sprint == nil {
		return ""
	}
	return item.Sprint.Title
}

func planSprint(cfg *config.Config, current []string, title string) (FieldResult, string) {
	if title == "" {
		return FieldResult{}, ""
	}
	for _, s := range cfg.Sprints.List {
		if strings.EqualFold(strings.TrimSpace(s.Title), strings.TrimSpace(title)) {
			_, added, removed := fields.Diff(current, "sprint", s.Slug, true)
			return FieldResult{Field: "sprint", Value: s.Slug, Added: added, Removed: removed}, ""
		}
	}
	return FieldResult{}, fmt.Sprintf("sprint %q has no matching [[sprints.list]] title, skipped", title)
}

// planAssignees reconciles the full "assignee:*" label set against the
// item's current GitHub assignees — unlike the other (single-cardinality)
// fields this adds AND removes so a re-run stays accurate if someone is
// unassigned on GitHub, not just additive.
func planAssignees(current, assignees []string) FieldResult {
	target := make(map[string]bool, len(assignees))
	for _, a := range assignees {
		target["assignee:"+a] = true
	}

	res := FieldResult{Field: "assignee"}
	haveCurrent := make(map[string]bool)
	for _, l := range current {
		if !strings.HasPrefix(l, "assignee:") {
			continue
		}
		haveCurrent[l] = true
		if !target[l] {
			res.Removed = append(res.Removed, l)
		}
	}
	for label := range target {
		if !haveCurrent[label] {
			res.Added = append(res.Added, label)
		}
	}
	// Added is built by ranging a map, so its order is otherwise random
	// across identical runs — sort for a stable dry-run preview/diff.
	slices.Sort(res.Added)
	slices.Sort(res.Removed)
	return res
}

func formatEstHours(v *float64) string {
	if v == nil {
		return ""
	}
	return strconv.FormatFloat(*v, 'f', -1, 64)
}

// genericRE matches "generic" case-insensitively, ignoring surrounding
// whitespace — the normalization for the PRD's documented duplicate
// "Generic" Module options. gh's item-list JSON gives only the option's
// display name, not its underlying option id, so if the duplicate options
// differ in id but coincide in name, the JSON already collapses them; this
// covers the case where they differ in casing/whitespace instead.
var genericRE = regexp.MustCompile(`(?i)^\s*generic\s*$`)

func normalizeModule(value string) string {
	if genericRE.MatchString(value) {
		return "Generic"
	}
	return strings.TrimSpace(value)
}

var slugNonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

// slugify normalizes a GitHub Project field's display value (e.g. "QA
// Validation") into a label-safe slug ("qa-validation"), matching the
// lowercase-hyphenated convention .git-board.toml's [fields.*].options use.
func slugify(value string) string {
	s := slugNonAlnum.ReplaceAllString(strings.ToLower(strings.TrimSpace(value)), "-")
	return strings.Trim(s, "-")
}

// LabelWriter is the subset of gitbugio.Backend ApplyPlan needs.
type LabelWriter interface {
	ApplyLabelChange(idPrefix string, added, removed []string) ([]string, error)
}

var _ LabelWriter = (*gitbugio.Backend)(nil)

// ApplyPlan writes every changed issue in the plan via ApplyLabelChange —
// the exact same call `fields.Apply`/`git-board set` makes, so a bridge
// pull commits like `set` would. No-op changes (nothing added or removed)
// are skipped rather than making a no-op ChangeLabels call, which git-bug
// itself rejects.
func ApplyPlan(w LabelWriter, plan Plan) error {
	for _, change := range plan.Changes {
		if len(change.Added) == 0 && len(change.Removed) == 0 {
			continue
		}
		if _, err := w.ApplyLabelChange(change.BugID, change.Added, change.Removed); err != nil {
			return fmt.Errorf("applying changes to %s (#%d): %w", change.BugID, change.Number, err)
		}
	}
	return nil
}
