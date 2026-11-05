package forge

import (
	"context"
	"slices"
	"strings"
)

// PR is a pull request as the gate shows it: the code's way into a base
// branch, found by its head branch's name and never stored. The words are
// GitHub's (internal/ghpr reads them); a forge naming them otherwise maps
// to them. The cycle reads it as evidence (core/cycle).
type PR struct {
	Number      int      `json:"number"`
	URL         string   `json:"url"`
	State       string   `json:"state"` // OPEN, MERGED, CLOSED
	Draft       bool     `json:"draft"`
	Title       string   `json:"title"`
	Body        string   `json:"body"` // the description, Markdown
	Base        string   `json:"base"`
	Head        string   `json:"head"`                   // its head commit
	MergeCommit string   `json:"merge_commit,omitempty"` // on the base, once merged
	Author      string   `json:"author"`
	Decision    string   `json:"decision"`    // reviewDecision: APPROVED, REVIEW_REQUIRED, CHANGES_REQUESTED, "" (no review rule)
	Mergeable   string   `json:"mergeable"`   // MERGEABLE, CONFLICTING, UNKNOWN
	MergeState  string   `json:"merge_state"` // mergeStateStatus
	Checks      []Check  `json:"checks"`
	Reviews     []Review `json:"reviews"`   // latestOpinionatedReviews: a COMMENTED review hides no approval
	Requested   []string `json:"requested"` // the people asked to review
	// set by the gate (ghpr.Gate)
	CanMerge bool     `json:"can_merge"`
	Checking bool     `json:"checking"` // the forge is still computing mergeability
	Waits    []string `json:"waits"`
}

// Check is one of a PR's checks: a check run or a commit status.
type Check struct {
	Name     string `json:"name"`
	State    string `json:"state"` // success, failure, pending, expected (required, never reported)
	Required bool   `json:"required"`
	URL      string `json:"url,omitempty"`
}

// Review is a person's latest approval or request for changes on a PR,
// or a review the forge dismissed.
type Review struct {
	Login  string `json:"login"`
	State  string `json:"state"`  // APPROVED, CHANGES_REQUESTED, DISMISSED
	Commit string `json:"commit"` // the commit it was made at
}

// Puller is a forge project whose pull requests (GitLab's merge requests)
// the cycle reads and moves: its gate's evidence, and Open PR, Edit and
// Merge on the issue page. GitHub and GitLab are; the fake is, for tests.
type Puller interface {
	// FindPR is the pull request whose head is branch, in this project
	// (a fork's branch of the same name is another): the open one, else
	// the newest; nil when there is none. Its gate is set (Gate).
	FindPR(ctx context.Context, branch string) (*PR, error)
	// OpenPR opens a pull request from head into base.
	OpenPR(ctx context.Context, head, base, title, body string) (*PR, error)
	// EditPR sets a pull request's title and description.
	EditPR(ctx context.Context, number int, title, body string) error
	// MergePR merges a pull request with a merge commit if its head is
	// still sha: nothing the page didn't show merges.
	MergePR(ctx context.Context, number int, sha string) error
}

// Gate puts the base's required checks first, in the rules' order,
// adding those that never reported as expected, then says whether the PR
// can merge (as the forge says it) and, if not, what it waits for. v1's
// internal/ghpr.Gate.
func Gate(p *PR, required []string) {
	var checks []Check
	for _, name := range required {
		c := Check{Name: name, State: "expected"}
		if i := slices.IndexFunc(p.Checks, func(c Check) bool { return c.Name == name }); i >= 0 {
			c = p.Checks[i]
		}
		c.Required = true
		checks = append(checks, c)
	}
	for _, c := range p.Checks {
		if !slices.Contains(required, c.Name) {
			checks = append(checks, c)
		}
	}
	if checks == nil {
		checks = []Check{}
	}
	p.Checks, p.CanMerge, p.Checking, p.Waits = checks, false, false, []string{}
	if p.Reviews == nil {
		p.Reviews = []Review{}
	}
	if p.Requested == nil {
		p.Requested = []string{}
	}
	if p.State != "OPEN" {
		return
	}
	p.Checking = p.Mergeable == "UNKNOWN"
	p.CanMerge = !p.Draft && p.Mergeable == "MERGEABLE" && slices.Contains([]string{"CLEAN", "HAS_HOOKS", "UNSTABLE"}, p.MergeState)
	if p.Draft {
		p.Waits = append(p.Waits, "ready for review (a draft)")
	}
	for _, c := range checks {
		if c.Required && c.State != "success" {
			p.Waits = append(p.Waits, c.Name)
		}
	}
	switch p.Decision {
	case "REVIEW_REQUIRED":
		w := "an approval"
		if len(p.Requested) > 0 {
			w += " from " + strings.Join(p.Requested, " or ")
		}
		p.Waits = append(p.Waits, w)
	case "CHANGES_REQUESTED":
		var who []string
		for _, r := range p.Reviews {
			if r.State == "CHANGES_REQUESTED" {
				who = append(who, r.Login)
			}
		}
		w := "the requested changes"
		if len(who) > 0 {
			w = "changes " + strings.Join(who, " and ") + " requested"
		}
		p.Waits = append(p.Waits, w)
	}
	if p.Mergeable == "CONFLICTING" {
		p.Waits = append(p.Waits, "conflicts with "+p.Base+" resolved")
	}
	if p.MergeState == "BEHIND" {
		p.Waits = append(p.Waits, "an update from "+p.Base)
	}
	if len(p.Waits) == 0 && p.MergeState == "BLOCKED" {
		p.Waits = append(p.Waits, "the forge's rules")
	}
}
