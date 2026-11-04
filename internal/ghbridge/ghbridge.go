// Package ghbridge bridges GitHub Project (v2) custom field values into
// git-board's ns:value label model, via `gh project item-list`. Read-only
// against GitHub: it only shells out to `gh project item-list`, never
// writes back to the Project or to GitHub issues — all writes land on the
// local git-bug repo via internal/store, same as `git-board set`.
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
// "size" was never observed with a value in a live pull (PRD calls it
// dead/unused) — it's simply not decoded here, so any value is silently
// ignored rather than mapped to a label. "milestone", the issue's, reads
// {"title":"V0.1 – Test deliverable","description":"…",
// "dueOn":"2026-11-29T00:00:00Z"} (guilbep's Project 1, 2026-10-26).
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
	"maps"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/store"
)

// Item is one row of `gh project item-list --format json`'s "items" array.
type Item struct {
	ID        string     `json:"id"`
	Assignees []string   `json:"assignees"`
	Content   Content    `json:"content"`
	Priority  string     `json:"priority"`
	Status    string     `json:"status"`
	Module    string     `json:"module"`
	Category  string     `json:"category"`
	Labels    []string   `json:"labels"` // the issue's on GitHub
	EstHours  *float64   `json:"est. hours"`
	MoSCoW    string     `json:"moscow"`
	Milestone *Milestone `json:"milestone"`
	Sprint    *Sprint    `json:"sprint"`
	Title     string     `json:"title"`
	// Extra holds the Project's other fields, by column name as gh lists
	// them ("Risk": "High"): a field whose target is the Project reads
	// its raw value here. scalars only; strings as they come, numbers
	// stringified.
	Extra map[string]string `json:"-"`
}

// UnmarshalJSON decodes the known fields, then keeps the rest: gh
// flattens every Project field to the item's top level, by column name.
func (i *Item) UnmarshalJSON(data []byte) error {
	type plain Item
	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	*i = Item(p)
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	known := map[string]bool{"id": true, "assignees": true, "content": true, "priority": true, "status": true, "module": true,
		"category": true, "labels": true, "est. hours": true, "moscow": true, "milestone": true, "sprint": true, "title": true}
	for k, v := range raw {
		if known[k] {
			continue
		}
		switch x := v.(type) {
		case string:
			i.setExtra(k, x)
		case float64:
			i.setExtra(k, strconv.FormatFloat(x, 'f', -1, 64))
		case bool:
			i.setExtra(k, strconv.FormatBool(x))
		}
	}
	return nil
}

func (i *Item) setExtra(k, v string) {
	if i.Extra == nil {
		i.Extra = map[string]string{}
	}
	i.Extra[k] = v
}

// Milestone is the issue's milestone, if it has one: a deliverable.
type Milestone struct {
	Title string `json:"title"`
	DueOn string `json:"dueOn"`
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
	Added     []string // union of every field's Added
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

// planKind fills an issue's kind from the Project's Category, which is
// the same vocabulary as the kind labels: FEAT is feat, matched to
// [branches] kinds without case and written as kinds spells it, as the
// kind field stores it (cfg.Label: feat, or kind:feat). A kind
// already on the issue wins: labels are the truth, Category only fills a
// gap, and so does one GitHub has (forge) that git-bug hasn't pulled yet.
// It removes the bare label an earlier bridge wrote, Category as is
// (FEAT): a sync would send it to the forge as feat, beside a bug, or
// create FEAT there. A label GitHub has stays, whatever its case: the
// forge reads FEAT as its feat.
func planKind(cfg *config.Config, current, forge []string, category string) (FieldResult, string) {
	if category == "" {
		return FieldResult{}, ""
	}
	res := FieldResult{Field: "kind"}
	onForge := slices.ContainsFunc(forge, func(l string) bool { return strings.EqualFold(l, category) })
	// ponytail: only the current Category's bare label; one left by an
	// earlier value stays (none on co2)
	if _, isKind := cfg.Branches.Kinds[category]; !isKind && !onForge && slices.Contains(current, category) {
		res.Removed = []string{category}
	}
	var warning string
	if cfg.Branches.Kind(current) == "" && cfg.Branches.Kind(forge) == "" {
		warning = fmt.Sprintf("category value %q is none of [branches] kinds, skipped", category)
		for _, k := range slices.Sorted(maps.Keys(cfg.Branches.Kinds)) {
			if strings.EqualFold(k, category) {
				res.Value, res.Added, warning = k, []string{cfg.Label("kind", k)}, ""
				break
			}
		}
	}
	if len(res.Added)+len(res.Removed) == 0 {
		return FieldResult{}, warning
	}
	return res, warning
}

func sprintTitle(item Item) string {
	if item.Sprint == nil {
		return ""
	}
	return item.Sprint.Title
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

// LabelWriter is the subset of store.Backend a sync writes labels with.
type LabelWriter interface {
	ApplyLabelChange(idPrefix string, added, removed []string) ([]string, error)
}

var _ LabelWriter = (*store.Backend)(nil)
