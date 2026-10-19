// Package cycle is an issue's step in the development cycle, computed from
// its status (the floor) and the evidence: the risk field, the plan file,
// the tests in the branch's diff, the PR, and comments with metadata. The
// step is never stored (docs/prd/dev-cycle.md).
package cycle

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/ghpr"
)

// Steps names the seven steps, 1 to 7.
var Steps = [...]string{"", "Intent", "Risk class", "Plan", "Tests first", "Code", "Review", "Verify"}

// Plan is the issue's plan file, found by its front matter's issue:.
type Plan struct {
	Path   string `json:"path,omitempty"`
	Ref    string `json:"ref,omitempty"`    // the branch it was read from
	Status string `json:"status,omitempty"` // its front matter status
	HTML   string `json:"html,omitempty"`   // rendered, below its front matter
	By     string `json:"by,omitempty"`     // who first committed it on the branch, "" when it came with the base
}

// Approval is a plan:approve comment.
type Approval struct {
	By     string    `json:"by"`
	Commit string    `json:"commit,omitempty"`
	At     time.Time `json:"at"`
}

// TestFile is a test the branch changed.
type TestFile struct {
	Path   string `json:"path"`
	Commit string `json:"commit,omitempty"` // the commit that first added it
}

// Signoff is a signoff:status comment.
type Signoff struct {
	Status string    `json:"status"`
	By     string    `json:"by"`
	Commit string    `json:"commit,omitempty"`
	Why    string    `json:"why,omitempty"` // an override's reason
	At     time.Time `json:"at"`
}

// Run is a verify:run comment: someone ran it at a commit.
type Run struct {
	By     string    `json:"by"`
	Commit string    `json:"commit"`
	At     time.Time `json:"at"`
}

// Check is a verify:criterion comment: one acceptance criterion checked.
type Check struct {
	Index  int       `json:"index"`
	By     string    `json:"by"`
	Commit string    `json:"commit,omitempty"`
	At     time.Time `json:"at"`
}

// Input is everything Compute needs, gathered once per issue.
type Input struct {
	Stages    config.Stages `json:"stages"`
	Status    string        `json:"status"` // the status field's value
	Closed    bool          `json:"closed"` // git-bug's closed
	Criteria  []string      `json:"criteria,omitempty"`
	Risk      string        `json:"risk,omitempty"`
	Author    string        `json:"author,omitempty"`
	Assignees []string      `json:"assignees,omitempty"`
	// People is the login holding each gate for this issue, "" for
	// nobody: "risk", "plan", "review", and "signoff:<status>".
	People map[string]string `json:"people,omitempty"`
	Me     string            `json:"-"` // the viewer's login, "" unknown

	Branch       string     `json:"branch,omitempty"` // the evidence branch
	Head         string     `json:"head,omitempty"`
	Base         string     `json:"base,omitempty"`
	PlanBranch   string     `json:"plan_branch,omitempty"`
	PlanHead     string     `json:"plan_head,omitempty"`
	Plan         *Plan      `json:"plan,omitempty"`
	PlanApproval *Approval  `json:"plan_approval,omitempty"` // by the plan gate's person
	PlanPR       *ghpr.PR   `json:"plan_pr,omitempty"`       // the plan branch's PR, structural
	Tests        []TestFile `json:"tests,omitempty"`
	PR           *ghpr.PR   `json:"pr,omitempty"`
	PRAt         time.Time  `json:"-"`
	Signoffs     []Signoff  `json:"signoffs,omitempty"`
	Runs         []Run      `json:"runs,omitempty"`
	Checks       []Check    `json:"checks,omitempty"`
}

// Waits is whose turn it is, and to do what.
type Waits struct {
	Role   string `json:"role,omitempty"`  // risk, plan, review, assignee, author, signoff
	Login  string `json:"login,omitempty"` // "" with Anyone
	What   string `json:"what"`
	Anyone bool   `json:"anyone"`
}

// Cycle is an issue's step.
type Cycle struct {
	Step     int      `json:"step"` // 1 to 7
	Of       int      `json:"of"`
	Name     string   `json:"name"`
	Stage    string   `json:"stage"`
	Done     bool     `json:"done"`
	Gate     string   `json:"gate"`
	Risk     string   `json:"risk,omitempty"`
	Skipped  []int    `json:"skipped,omitempty"` // [3] for low risk
	Note     string   `json:"note,omitempty"`    // evidence that disagrees with the status
	Waits    Waits    `json:"waits"`
	Mine     bool     `json:"mine"`              // the viewer's turn, or anyone's
	Actions  []string `json:"actions,omitempty"` // set-risk, approve-plan, review, verify, check, signoff
	Evidence Input    `json:"evidence"`
}

func gate(step int, risk string) string {
	switch {
	case step == 3 && risk == "structural":
		return "Structural: plan PR approved first"
	case step == 3 && risk == "low":
		return "Skipped, low risk"
	case step == 6 && risk == "structural":
		return "Structural: a second human"
	}
	return [...]string{"", "No issue, no agent", "Class written on the issue", "Plan approved by the plan role",
		"Every bug fix ships its test", "A human commits and opens the PR", "A human who can explain the diff", "Green codes are not proof"}[step]
}

// approvedPlan: the plan file is accepted or past it, the plan role
// approved it in a comment (not structural), or its PR is approved.
func approvedPlan(in Input) bool {
	switch {
	case in.Plan != nil && slices.Contains([]string{"accepted", "in-progress", "delivered"}, in.Plan.Status):
		return true
	case in.Risk == "structural":
		return in.PlanPR != nil && (in.PlanPR.Decision == "APPROVED" || in.PlanPR.State == "MERGED")
	}
	return in.PlanApproval != nil
}

func first(statuses []string, fallback string) string {
	if len(statuses) > 0 {
		return statuses[0]
	}
	return fallback
}

// Compute is the issue's step: the status's stage is the floor, the
// evidence moves the step within it, and evidence that disagrees is a note.
func Compute(in Input) Cycle {
	c := Cycle{Of: 7, Risk: in.Risk, Stage: in.Stages.StageOf(in.Status), Evidence: in}
	if in.Closed || c.Stage == "done" {
		c.Step, c.Name, c.Done, c.Gate = 7, "Done", true, gate(7, in.Risk)
		return c
	}
	assignee := ""
	if len(in.Assignees) > 0 {
		assignee = in.Assignees[0]
	}
	approved := approvedPlan(in)
	inProgress := first(in.Stages.InProgress, "in-progress")
	w := Waits{Role: "author", Login: in.Author, What: "add acceptance criteria"}
	switch c.Stage {
	case "ready":
		switch {
		case len(in.Criteria) == 0:
			c.Step = 1
		case in.Risk == "":
			c.Step, w = 2, Waits{Role: "risk", Login: in.People["risk"], What: "set the risk class"}
		case in.Risk == "low" || approved:
			// ponytail: the plan doesn't need anyone; whoever is assigned starts
			c.Step, w = 3, Waits{Role: "assignee", Login: assignee, What: "move the status to " + inProgress}
			c.Note = "plan approved, waits for the status " + inProgress
			if in.Risk == "low" {
				c.Skipped, c.Note = []int{3}, "low risk, waits for the status "+inProgress
			}
		default:
			c.Step, w = 3, Waits{Role: "plan", Login: in.People["plan"], What: "approve the plan"}
			if in.Risk == "structural" {
				w.What = "approve the plan PR on GitHub"
			}
		}
	case "in-progress":
		c.Step, w = 4, Waits{Role: "assignee", Login: assignee, What: "write the failing test"}
		if len(in.Tests) > 0 {
			c.Step, w.What = 5, "commit and open the PR"
		}
		switch {
		case in.Risk == "":
			c.Note = "risk not set"
		case in.Risk != "low" && !approved:
			c.Note = "plan not approved"
		case in.PR != nil && in.PR.State == "OPEN":
			c.Note = fmt.Sprintf("PR #%d open, waits for the status %s", in.PR.Number, first(in.Stages.InReview, "in-review"))
		}
	case "in-review":
		c.Step, w = 6, Waits{Role: "review", Login: in.People["review"], What: "review the PR"}
		if in.PR != nil && in.PR.State == "MERGED" {
			c.Note = "merged, waits for the status"
		}
	case "validation":
		c.Step, w = 7, Waits{Role: "signoff", Login: in.People["signoff:"+in.Status], What: "sign off " + in.Status}
		switch {
		case in.Plan != nil && in.Plan.Status != "delivered":
			c.Note = "plan not marked delivered"
		case in.Plan == nil && in.Risk != "low":
			c.Note = "no plan file"
		}
	default: // before ready, or a status no option names
		c.Step = 1
		if len(in.Criteria) > 0 && in.Stages.Ready != "" {
			w.What = "move the status to " + in.Stages.Ready
		}
	}
	c.Name, c.Gate = Steps[c.Step], gate(c.Step, in.Risk)
	w.Anyone = w.Login == ""
	c.Waits = w
	c.Mine = w.Anyone || (in.Me != "" && strings.EqualFold(w.Login, in.Me))
	if c.Mine {
		switch {
		case c.Step == 2:
			c.Actions = []string{"set-risk"}
		case c.Step == 3 && w.Role == "plan" && in.Risk != "structural":
			c.Actions = []string{"approve-plan"}
		case c.Step == 6:
			c.Actions = []string{"review"}
		case c.Step == 7:
			c.Actions = []string{"verify", "check", "signoff"}
		}
	}
	return c
}

var (
	criteriaHeading = regexp.MustCompile(`(?i)^\s*(#{1,6}\s*|\*\*)acceptance criteria:?(\*\*)?:?\s*$`)
	criteriaItem    = regexp.MustCompile(`^\s*([-*+]|\d+[.)])\s+(\[[ xX]\]\s*)?(.+)$`)
)

// Criteria lists the items of the list under an "Acceptance criteria"
// heading (any level, or a bold line) in a Markdown description.
func Criteria(description string) []string {
	var res []string
	in := false
	for _, l := range strings.Split(description, "\n") {
		switch {
		case !in:
			in = criteriaHeading.MatchString(l)
		case strings.TrimSpace(l) == "":
		case criteriaItem.MatchString(l):
			res = append(res, strings.TrimSpace(criteriaItem.FindStringSubmatch(l)[3]))
		default:
			return res
		}
	}
	return res
}
