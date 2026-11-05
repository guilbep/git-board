package board

import (
	"slices"
	"time"
)

// Cycle is an issue's step, as GET /api/issues/{id}/cycle answers it
// (git-board v2's app.CycleView, next/domain/cycle.Cycle): the fields an
// agent's prompt and its refusals read. The project's stages are in
// Evidence.Stages (v1 also answered them beside, as stages).
type Cycle struct {
	Step  int    `json:"step"` // the kind's, 1 to 7
	At    string `json:"at"`   // the list's entry reached, "" before the first
	Of    int    `json:"of"`   // the list's length
	Name  string `json:"name"` // the entry's name
	Stage string `json:"stage"`
	Done  bool   `json:"done"`
	Gate  string `json:"gate"`
	Risk  string `json:"risk,omitempty"`
	Waits struct {
		What string `json:"what"`
	} `json:"waits"`
	Evidence Evidence `json:"evidence"`
	ID       string   `json:"id"`      // the issue's whole id
	Version  string   `json:"version"` // the issue's version: a write's If-Match
}

// Evidence is what the cycle was computed from.
type Evidence struct {
	Stages       Stages    `json:"stages"`
	Risk         string    `json:"risk,omitempty"`
	Branch       string    `json:"branch,omitempty"` // the evidence branch
	Plan         *Plan     `json:"plan,omitempty"`
	PlanApproval *Approval `json:"plan_approval,omitempty"`
	PlanPR       *PR       `json:"plan_pr,omitempty"`
}

// Stages are the project's statuses by stage, and its steps.
type Stages struct {
	Ready      string   `json:"ready"`
	InProgress []string `json:"in_progress"`
	Plans      string   `json:"plans"`
	PlansInPR  bool     `json:"plans_in_pr"`
	Steps      []Step   `json:"steps"`
}

// Step is one entry of the project's list of steps.
type Step struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// Plan is the plan file found on the branch.
type Plan struct {
	Path   string `json:"path,omitempty"`
	Status string `json:"status,omitempty"` // its front matter's
}

// Approval is the plan approved on the board.
type Approval struct {
	By string    `json:"by"`
	At time.Time `json:"at"`
}

// PR is a pull request, as far as the plan's approval reads it.
type PR struct {
	State    string `json:"state"`    // OPEN, MERGED, CLOSED
	Decision string `json:"decision"` // APPROVED, ...
}

// defaultSteps are the seven of a project with no [cycle] steps.
var defaultSteps = []Step{
	{ID: "intent", Kind: "intent"}, {ID: "risk", Kind: "risk"}, {ID: "plan", Kind: "plan"},
	{ID: "tests", Kind: "tests"}, {ID: "code", Kind: "code"}, {ID: "review", Kind: "review"},
	{ID: "verify", Kind: "signoff"},
}

func steps(s Stages) []Step {
	if len(s.Steps) == 0 {
		return defaultSteps
	}
	return s.Steps
}

// Position is At's place in the list, from 1; 0 when At is ""
// (core/cycle's Cycle.Position).
func (c Cycle) Position() int {
	return slices.IndexFunc(steps(c.Evidence.Stages), func(s Step) bool { return s.ID == c.At }) + 1
}

// ApprovedPlan tells whether the plan holds no start back (core/cycle's
// ApprovedPlan): no plan step in the list, the plan file accepted, a
// structural plan's PR approved or merged, else the plan approved on the
// board.
func ApprovedPlan(in Evidence) bool {
	switch {
	case !slices.ContainsFunc(steps(in.Stages), func(s Step) bool { return s.Kind == "plan" }):
		return true
	case in.Plan != nil && slices.Contains([]string{"accepted", "in-progress", "delivered"}, in.Plan.Status):
		return true
	case in.Risk == "structural":
		return in.PlanPR != nil && (in.PlanPR.Decision == "APPROVED" || in.PlanPR.State == "MERGED")
	}
	return in.PlanApproval != nil
}
