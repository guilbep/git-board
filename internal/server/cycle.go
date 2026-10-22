package server

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/guilbep/git-board/internal/board"
	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/cycle"
	"github.com/guilbep/git-board/internal/fields"
	"github.com/guilbep/git-board/internal/ghpr"
	"github.com/guilbep/git-board/internal/gitbugio"
)

// The development cycle (docs/prd/dev-cycle.md, M1): an issue's step,
// computed from its status and the evidence at every read, never stored.

// pullsEvery is how often the cycle asks GitHub for a branch's PR.
const pullsEvery = 5 * time.Minute

// cachedPull is a branch's PR as last read (at, zero: never), and when
// GitHub was last asked.
type cachedPull struct {
	pr        *ghpr.PR
	at, tried time.Time
}

// pullCache is the PR of each branch on origin, by name: the cycle reads
// it, and offline the last answer stands (cycle.Pulls).
type pullCache struct {
	mu sync.Mutex
	m  map[string]cachedPull
}

func (c *pullCache) PR(branch string) (*ghpr.PR, time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e := c.m[branch]
	return e.pr, e.at
}

func (c *pullCache) stale(branch string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return time.Since(c.m[branch].tried) >= pullsEvery
}

// put records an answer; err keeps the last PR.
func (c *pullCache) put(branch string, pr *ghpr.PR, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = map[string]cachedPull{}
	}
	e := c.m[branch]
	e.tried = time.Now()
	if err == nil {
		e.pr, e.at = pr, e.tried
	}
	c.m[branch] = e
}

// refreshPulls asks GitHub for the PR of each branch whose answer is
// older than pullsEvery, as handlePulls does, within 10 seconds. Without
// GitHub or a token it asks nothing: the cycle computes without the PR.
// ponytail: one branch after the other, as handlePulls.
func (s *apiServer) refreshPulls(ctx context.Context, names []string) {
	names = slices.DeleteFunc(names, func(n string) bool { return !s.pulls.stale(n) })
	repo := s.b.GitHubRepo("origin")
	if repo == "" || len(names) == 0 {
		return
	}
	c, _, err := s.github()
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	for _, name := range names {
		pr, err := c.Find(ctx, repo, name)
		if err == nil && pr != nil && pr.State == "OPEN" {
			var required []string
			if required, err = c.Required(ctx, repo, pr.Base); err == nil {
				ghpr.Gate(pr, required)
			}
		}
		s.pulls.put(name, pr, err)
	}
}

// cycleResponse is the step, the stages it was computed with, and when
// the PR was last read from GitHub.
type cycleResponse struct {
	cycle.Cycle
	Stages      config.Stages `json:"stages"`
	PRCheckedAt time.Time     `json:"pr_checked_at,omitzero"`
}

// pullNames are the branches on origin whose PR the step needs: the
// issue's from in-review on, its plan branches when structural, none
// before (the Work page asks for every issue's at once).
func (s *apiServer) pullNames(th gitbugio.Thread, stages config.Stages) ([]string, error) {
	names, err := onOrigin(s.b, th.Issue)
	if err != nil {
		return nil, err
	}
	switch stage := stages.StageOf(board.FieldValue(th.Issue, "status")); {
	case slices.Contains([]string{"in-review", "validation", "done"}, stage):
	case board.FieldValue(th.Issue, "risk") == "structural":
		names = slices.DeleteFunc(names, func(n string) bool { return !strings.HasPrefix(n, "plan/") })
	default:
		names = nil
	}
	return names, nil
}

// cycleOf is the issue's step for the viewer. GitHub is asked for the PR
// only where the step reads it: from review on, and a structural plan's PR.
func (s *apiServer) cycleOf(ctx context.Context, th gitbugio.Thread) (cycleResponse, error) {
	cfg := s.config()
	stages := cfg.Stages()
	names, err := s.pullNames(th, stages)
	if err != nil {
		return cycleResponse{}, err
	}
	s.refreshPulls(ctx, names)
	in, err := cycle.Gather(s.b, cfg, th, s.viewer(), &s.pulls)
	if err != nil {
		return cycleResponse{}, err
	}
	return cycleResponse{Cycle: cycle.Compute(in), Stages: stages, PRCheckedAt: in.PRAt}, nil
}

func (s *apiServer) handleCycle(w http.ResponseWriter, r *http.Request) {
	th, err := s.b.Thread(r.PathValue("id"))
	if err != nil {
		writeIssueError(w, err)
		return
	}
	res, err := s.cycleOf(r.Context(), th)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

var errNotAtVerify = errors.New("not at Verify")

// handleCycleAction writes what a gate needs as a comment with metadata,
// in its step's discussion (cycle:step): approve-plan (which then moves a
// ready issue to in-progress), verify (a run's output), check (one
// criterion), signoff (which then moves the status on, closing the issue
// after the last validation status). The risk and the other status moves
// are fields: PATCH the issue.
func (s *apiServer) handleCycleAction(w http.ResponseWriter, r *http.Request) {
	action := r.PathValue("action")
	if !slices.Contains([]string{"approve-plan", "verify", "check", "signoff"}, action) {
		writeError(w, http.StatusNotFound, fmt.Errorf("no cycle action %q", action))
		return
	}
	var req struct {
		Output string `json:"output"`
		Commit string `json:"commit"`
		Index  *int   `json:"index"`
		Why    string `json:"why"`
	}
	if !decode(w, r, &req) {
		return
	}
	done, ok := s.beginWrite(w)
	if !ok {
		return
	}
	defer done()
	id := r.PathValue("id")
	th, err := s.b.Thread(id)
	if err != nil {
		writeIssueError(w, err)
		return
	}
	cur, err := s.cycleOf(r.Context(), th)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	ev, cfg := cur.Evidence, s.config()
	// waitsFor refuses when the gate has a person and it isn't the viewer.
	waitsFor := func(role, login string) bool {
		if login == "" || strings.EqualFold(login, s.viewer()) {
			return false
		}
		writeError(w, http.StatusForbidden, fmt.Errorf("waits for the %s role: %s", role, login))
		return true
	}
	atVerify := cur.Step == 7 && !cur.Done
	commit := cmp.Or(req.Commit, ev.Head)
	meta := map[string]string{"cycle:step": "7"}
	var body string
	switch action {
	case "approve-plan":
		if waitsFor("plan", ev.People["plan"]) {
			return
		}
		if ev.Risk == "structural" {
			writeError(w, http.StatusBadRequest, errors.New("the plan PR is approved on GitHub"))
			return
		}
		body, meta["plan:approve"], meta["cycle:step"] = "Plan approved", "1", "3"
		if c := cmp.Or(ev.Head, ev.PlanHead); c != "" {
			meta["plan:commit"] = c
		}
	case "verify":
		if !atVerify {
			writeError(w, http.StatusBadRequest, errNotAtVerify)
			return
		}
		if strings.TrimSpace(req.Output) == "" {
			writeError(w, http.StatusBadRequest, errors.New("a run needs its output"))
			return
		}
		fence := "```"
		for strings.Contains(req.Output, fence) {
			fence += "`"
		}
		body, meta["verify:run"] = "Run"+at(commit)+"\n\n"+fence+"\n"+strings.TrimRight(req.Output, "\n")+"\n"+fence, "1"
	case "check":
		if !atVerify {
			writeError(w, http.StatusBadRequest, errNotAtVerify)
			return
		}
		if req.Index == nil || *req.Index < 0 || *req.Index >= len(ev.Criteria) {
			writeError(w, http.StatusBadRequest, fmt.Errorf("index: one of the %d acceptance criteria, from 0", len(ev.Criteria)))
			return
		}
		body, meta["verify:criterion"] = "Checked: "+ev.Criteria[*req.Index], strconv.Itoa(*req.Index)
	case "signoff":
		if waitsFor(cfg.Cycle.Signoffs[ev.Status], ev.People["signoff:"+ev.Status]) {
			return
		}
		if !atVerify {
			writeError(w, http.StatusBadRequest, errNotAtVerify)
			return
		}
		// the run is checked at the branch's head, or at the commit merged
		// on the base once the PR is merged (the PR cache knows it)
		merged := ""
		if ev.PR != nil && ev.PR.State == "MERGED" {
			merged = ev.PR.MergeCommit
		}
		ran := slices.ContainsFunc(ev.Runs, func(run cycle.Run) bool {
			return ev.Head == "" || run.Commit == ev.Head || (merged != "" && run.Commit == merged)
		})
		if !ran && strings.TrimSpace(req.Why) == "" {
			writeError(w, http.StatusBadRequest, fmt.Errorf("no run%s; sign off with a reason to override", at(ev.Head)))
			return
		}
		body, meta["signoff:status"] = "Signed off "+ev.Status+at(ev.Head), ev.Status
		if ev.Head != "" {
			meta["signoff:commit"] = ev.Head
		}
		if why := strings.TrimSpace(req.Why); why != "" {
			body, meta["signoff:why"] = body+"\n\n"+why, why
		}
	}
	if (action == "verify" || action == "check") && commit != "" {
		meta["verify:commit"] = commit
	}
	if _, err := s.b.AddCommentMeta(id, body, meta); err != nil {
		writeIssueError(w, err)
		return
	}
	if action == "approve-plan" && cur.Stage == "ready" && len(cur.Stages.InProgress) > 0 {
		if _, err := fields.Apply(s.b, cfg, id, "status", cur.Stages.InProgress[0], s.me, time.Now()); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	if action == "signoff" {
		if err := s.signedOff(id, cur.Stages, ev.Status); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	if th, err = s.b.Thread(id); err == nil {
		cur, err = s.cycleOf(r.Context(), th)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, cur)
}

// signedOff moves the status to the next validation status, or after the
// last one to the closing status, and closes the issue.
func (s *apiServer) signedOff(id string, stages config.Stages, status string) error {
	i := slices.Index(stages.Validation, status)
	next := stages.Closing
	if i+1 < len(stages.Validation) {
		next = stages.Validation[i+1]
	}
	if next == "" {
		return nil
	}
	if _, err := fields.Apply(s.b, s.config(), id, "status", next, s.me, time.Now()); err != nil {
		return err
	}
	if next == stages.Closing {
		return s.b.CloseBug(id)
	}
	return nil
}

// at is " at " and a commit's first 7 characters, as git shows it; "" for none.
func at(sha string) string {
	if sha == "" {
		return ""
	}
	return " at " + sha[:min(7, len(sha))]
}
