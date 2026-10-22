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
var Steps = [...]string{"", "Intent", "Risk", "Plan", "Tests", "Code", "Review", "Verify"}

// Plan is the issue's plan file, found by its front matter's issue:.
type Plan struct {
	Path   string `json:"path,omitempty"`
	Ref    string `json:"ref,omitempty"`    // the branch it was read from
	Status string `json:"status,omitempty"` // its front matter status
	Issue  string `json:"issue,omitempty"`  // its front matter issue
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
	Stages   config.Stages `json:"stages"`
	Status   string        `json:"status"` // the status field's value
	Closed   bool          `json:"closed"` // git-bug's closed
	Criteria []string      `json:"criteria,omitempty"`
	// CriteriaProblems are the description's warnings (Criteria)
	CriteriaProblems []string `json:"criteria_problems,omitempty"`
	Risk             string   `json:"risk,omitempty"`
	Kind             string   `json:"kind,omitempty"` // [branches]' kind: step 4's rule (Stages.KindRule)
	Author           string   `json:"author,omitempty"`
	Assignees        []string `json:"assignees,omitempty"`
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
	Step    int    `json:"step"` // 1 to 7
	Of      int    `json:"of"`
	Name    string `json:"name"`
	Stage   string `json:"stage"`
	Done    bool   `json:"done"`
	Gate    string `json:"gate"`
	Risk    string `json:"risk,omitempty"`
	Skipped []int  `json:"skipped,omitempty"` // 3 for low risk, 4 for a kind with no tests
	Note    string `json:"note,omitempty"`    // evidence that disagrees with the status
	Waits   Waits  `json:"waits"`
	Mine    bool   `json:"mine"` // the viewer's turn, or anyone's
	// Actions the viewer can take: set-risk, approve-plan, review, verify,
	// check, signoff, and the status moves move-ready, start,
	// ready-for-review, to-validation.
	Actions  []string `json:"actions,omitempty"`
	Evidence Input    `json:"evidence"`
}

// gate is the step's rule, by the risk and step 4's rule for the kind.
func gate(step int, risk, rule string) string {
	switch {
	case step == 3 && risk == "structural":
		return "Structural: plan PR approved first"
	case step == 3 && risk == "low":
		return "Skipped, low risk"
	case step == 4 && rule == config.NoTests:
		return "Skipped: no tests for this kind"
	case step == 4 && rule == config.FailingTest:
		return "A test that fails before the fix"
	case step == 4:
		return "The touched suites green, each criterion names its test"
	case step == 6 && risk == "structural":
		return "Structural: a second human"
	}
	return [...]string{"", "No issue, no agent", "Class written on the issue", "Plan approved by the plan role",
		"", "A human commits and opens the PR", "A human who can explain the diff", "Green codes are not proof"}[step]
}

// ApprovedPlan: the plan file is accepted or past it, the plan role
// approved it in a comment (not structural), or its PR is approved.
func ApprovedPlan(in Input) bool {
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
	rule := in.Stages.KindRule(in.Kind)
	c := Cycle{Of: 7, Risk: in.Risk, Stage: in.Stages.StageOf(in.Status), Evidence: in}
	if in.Risk == "low" {
		c.Skipped = append(c.Skipped, 3)
	}
	if rule == config.NoTests {
		c.Skipped = append(c.Skipped, 4)
	}
	if in.Closed || c.Stage == "done" {
		c.Step, c.Name, c.Done, c.Gate = 7, "Done", true, gate(7, in.Risk, rule)
		return c
	}
	assignee := ""
	if len(in.Assignees) > 0 {
		assignee = in.Assignees[0]
	}
	approved := ApprovedPlan(in)
	inProgress := first(in.Stages.InProgress, "in-progress")
	merged := in.PR != nil && in.PR.State == "MERGED"
	draft := in.PR != nil && in.PR.State == "OPEN" && in.PR.Draft
	stage := c.Stage
	if stage == "in-review" && merged && len(in.Stages.Validation) == 0 {
		stage = "validation" // no validation status: Verify happens in in-review once merged
	}
	w := Waits{Role: "author", Login: in.Author, What: "add acceptance criteria"}
	act := "" // what the step's person presses, when it is theirs
	switch stage {
	case "ready":
		switch {
		case len(in.Criteria) == 0:
			c.Step = 1
		case in.Risk == "":
			c.Step, w, act = 2, Waits{Role: "risk", Login: in.People["risk"], What: "set the risk class"}, "set-risk"
		case in.Risk == "low" || approved:
			// ponytail: the plan doesn't need anyone; whoever is assigned starts
			c.Step, w, act = 3, Waits{Role: "assignee", Login: assignee, What: "move the status to " + inProgress}, "start"
			c.Note = "plan approved, waits for the status " + inProgress
			if in.Risk == "low" {
				c.Note = "low risk, waits for the status " + inProgress
			}
		default:
			c.Step, w, act = 3, Waits{Role: "plan", Login: in.People["plan"], What: "approve the plan"}, "approve-plan"
			if in.Risk == "structural" {
				w.What, act = "approve the plan PR on GitHub", ""
			}
		}
	case "in-progress":
		c.Step, w = 4, Waits{Role: "assignee", Login: assignee, What: "write the tests"}
		if rule == config.FailingTest {
			w.What = "write the failing test"
		}
		if len(in.Tests) > 0 || rule == config.NoTests {
			c.Step, w.What, act = 5, "commit and open the PR", "ready-for-review"
		}
		switch {
		case in.Risk == "":
			c.Note = "risk not set"
		case in.Risk != "low" && !approved:
			c.Note = "plan not approved"
		case draft && c.Step == 4:
			// the draft PR holding the plan: step 4's own state (C4)
		case draft:
			// Review opens when the draft PR is marked ready, not before
			c.Note, act = fmt.Sprintf("draft PR #%d, waits for it to be marked ready for review", in.PR.Number), ""
		case in.PR != nil && in.PR.State == "OPEN":
			c.Note = fmt.Sprintf("PR #%d open, waits for the status %s", in.PR.Number, first(in.Stages.InReview, "in-review"))
		}
	case "in-review":
		c.Step, w, act = 6, Waits{Role: "review", Login: in.People["review"], What: "review the PR"}, "review"
		if draft {
			c.Note, act = fmt.Sprintf("PR #%d is still a draft on GitHub", in.PR.Number), ""
		}
		if merged { // with a validation status: the stage above is false otherwise
			next := in.Stages.Validation[0]
			w, act = Waits{Role: "assignee", Login: assignee, What: "move the status to " + next}, "to-validation"
			c.Note = "merged, waits for the status " + next
		}
	case "validation":
		c.Step, w, act = 7, Waits{Role: "signoff", Login: in.People["signoff:"+in.Status], What: "sign off " + in.Status}, "verify"
		switch {
		case in.Plan != nil && in.Plan.Status != "delivered":
			c.Note = "plan not marked delivered"
		case in.Plan == nil && in.Risk != "low":
			c.Note = "no plan file"
		}
	default: // before ready, or a status no option names
		c.Step = 1
		if len(in.Criteria) > 0 && in.Stages.Ready != "" {
			w.What, act = "move the status to "+in.Stages.Ready, "move-ready"
		}
	}
	c.Name, c.Gate = Steps[c.Step], gate(c.Step, in.Risk, rule)
	w.Anyone = w.Login == ""
	c.Waits = w
	c.Mine = w.Anyone || (in.Me != "" && strings.EqualFold(w.Login, in.Me))
	switch {
	case !c.Mine || act == "":
	case act == "verify":
		c.Actions = []string{"verify", "check", "signoff"}
	default:
		c.Actions = []string{act}
	}
	return c
}

var (
	criteriaItem = regexp.MustCompile(`^(\s*)([-*+]|\d+[.)])\s+(\[[ xX]\]\s*)?(.+)$`)
	anyHeading   = regexp.MustCompile(`^\s*(#{1,6}\s|\*\*.+\*\*:?\s*$)`)
)

// Criteria lists the items of the list under the first of the headings
// ([cycle] criteria) a Markdown description has: a heading of any level,
// or a bold line, in any case, with or without a colon. The list runs to
// its first line that is neither blank nor an item. Problems are the
// warnings Intent shows, each naming its line: where the description
// reads differently from how it looks.
func Criteria(description string, headings []string) (items, problems []string) {
	if len(headings) == 0 {
		return nil, nil
	}
	q := make([]string, len(headings))
	for i, h := range headings {
		q[i] = regexp.QuoteMeta(strings.TrimSpace(h))
	}
	heading := regexp.MustCompile(`(?i)^\s*(#{1,6}\s*|\*\*)(` + strings.Join(q, "|") + `):?(\*\*)?:?\s*$`)
	lines := strings.Split(description, "\n")
	at, ended, indent := -1, false, -1
	for i, l := range lines {
		switch m := criteriaItem.FindStringSubmatch(l); {
		case heading.MatchString(l):
			if at < 0 {
				at = i
			} else {
				problems = append(problems, fmt.Sprintf("line %d: a second criteria heading, ignored: only the first list counts", i+1))
				ended = true
			}
		case at < 0 || ended || strings.TrimSpace(l) == "":
		case m != nil:
			if indent < 0 {
				indent = len(m[1])
			} else if len(m[1]) > indent {
				problems = append(problems, fmt.Sprintf("line %d: a sub-item, counted as a criterion of its own", i+1))
			}
			items = append(items, strings.TrimSpace(m[4]))
		default:
			ended = true
			if len(items) > 0 && !anyHeading.MatchString(l) && itemBeforeHeading(lines[i+1:]) {
				problems = append(problems, fmt.Sprintf("line %d is not a list item: the list stops there, and the items after it don't count", i+1))
			}
		}
	}
	if at >= 0 && len(items) == 0 {
		problems = append(problems, fmt.Sprintf("line %d: no list right under “%s”", at+1, strings.Trim(strings.TrimSpace(lines[at]), "#* :")))
	}
	return items, problems
}

// itemBeforeHeading: a list item comes before the next heading.
func itemBeforeHeading(lines []string) bool {
	for _, l := range lines {
		if anyHeading.MatchString(l) {
			return false
		}
		if criteriaItem.MatchString(l) {
			return true
		}
	}
	return false
}
