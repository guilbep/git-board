// Package agent starts, briefs and stops a coding agent on a git-board
// issue, in the issue's own worktree, through wtx: what git-board v1's
// server did behind the issue page's Agent panel and the Work page
// (internal/server/worktrees.go, work.go), as a client of the board.
//
// The board says what the issue is and where it stands (package board);
// wtx makes the worktree, the tmux session and the agent. The prompt is
// our words first, the issue's last, quoted as data (wtx.Prompt).
package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/guilbep/board-agents/internal/board"
	"github.com/guilbep/board-agents/internal/wtx"
)

// Board is what the agents read and write on the board: board.Client,
// or a fake in the tests.
type Board interface {
	Thread(ctx context.Context, ref string) (board.Thread, error)
	// Cycle is nil-safe: an error when no server runs or the cycle is off,
	// and the agent then starts with no step.
	Cycle(ctx context.Context, id string) (*board.Cycle, error)
	Branch(ctx context.Context, id string) (string, error)
	SetStatus(ctx context.Context, id, status string) error
	ReviewThread(ctx context.Context, id string) (board.Thread, error)
	Comment(ctx context.Context, id, body, step string) error
	Agents(ctx context.Context) ([]board.WorkAgent, error)
}

// Refusal is an action refused before wtx runs, by code: has-worktree,
// branch-all, no-worktree, no-plan, not-approved, no-cycle, no-wtx.
type Refusal struct {
	Code, Reason string
}

func (r *Refusal) Error() string { return r.Reason }

func refuse(code, reason string) error { return &Refusal{code, reason} }

var errNoWtx = refuse("no-wtx", "wtx is not on PATH: https://github.com/EPFL-ENAC/wtx")

// Service runs the agents of the repository whose main checkout is Root:
// wtx runs there, as it did where git board serve ran.
type Service struct {
	Board Board
	Root  string
}

// Worktree is a linked worktree.
type Worktree struct {
	Path   string `json:"path"`
	Head   string `json:"head"`
	Branch string `json:"branch,omitempty"` // "" when detached
}

// Agent is an issue's agent: its branch, the worktree on it, and wtx's
// session there.
type Agent struct {
	Issue    string       `json:"issue"` // CO2-421, #421, or the short id
	ID       string       `json:"id"`
	Title    string       `json:"title"`
	Step     string       `json:"step,omitempty"` // "4 of 7: Tests", "" without a cycle
	Branch   string       `json:"branch"`         // the evidence branch, else [branches]' name for the issue
	Worktree *Worktree    `json:"worktree,omitempty"`
	Session  *wtx.Session `json:"session,omitempty"` // wtx status for it, when wtx answers
	Output   string       `json:"output,omitempty"`  // a write: wtx's last lines
	Moved    string       `json:"moved,omitempty"`   // the status a start moved the issue to
}

// Request is what the person adds to an action: a brief, and for brief a
// line comment whose thread the agent is asked to answer.
type Request struct {
	Brief   string
	Comment string
}

// Worktrees lists the linked worktrees of root, the main one left out:
// git worktree list --porcelain.
func Worktrees(ctx context.Context, root string) ([]Worktree, error) {
	out, err := exec.CommandContext(ctx, "git", "-C", root, "worktree", "list", "--porcelain").Output()
	if err != nil {
		return nil, fmt.Errorf("git worktree list: %w", err)
	}
	var res []Worktree
	var cur Worktree
	first, bare := true, false
	flush := func() {
		if cur.Path != "" && !first && !bare {
			res = append(res, cur)
		}
		if cur.Path != "" {
			first = false
		}
		cur, bare = Worktree{}, false
	}
	for _, l := range strings.Split(string(out), "\n") {
		switch k, v, _ := strings.Cut(l, " "); k {
		case "worktree":
			cur.Path = v
		case "HEAD":
			cur.Head = v
		case "branch":
			cur.Branch = strings.TrimPrefix(v, "refs/heads/")
		case "bare":
			bare = true
		case "":
			flush()
		}
	}
	flush()
	return res, nil
}

// look is the issue, its cycle (nil without one) and its agent, with
// wtx's session when session.
func (s *Service) look(ctx context.Context, ref string, session bool) (board.Thread, *board.Cycle, Agent, error) {
	th, err := s.Board.Thread(ctx, ref)
	if err != nil {
		return th, nil, Agent{}, err
	}
	cyc, _ := s.Board.Cycle(ctx, th.ID) // ponytail: no server or the cycle off reads as no cycle
	a, err := s.of(ctx, th, cyc, session)
	return th, cyc, a, err
}

// of is the issue's agent: the branch from the cycle's evidence, else the
// board's name for it; the worktree from git; with session, the session
// from wtx.
func (s *Service) of(ctx context.Context, th board.Thread, cyc *board.Cycle, session bool) (Agent, error) {
	a := Agent{Issue: th.Name(), ID: th.ID, Title: th.Title}
	if cyc != nil {
		a.Branch = cyc.Evidence.Branch
		if cyc.Of > 0 && cyc.Position() > 0 {
			a.Step = fmt.Sprintf("%d of %d: %s", cyc.Position(), cyc.Of, cyc.Name)
		}
	}
	if a.Branch == "" {
		b, err := s.Board.Branch(ctx, th.ID)
		if err != nil {
			return a, err
		}
		a.Branch = b
	}
	wts, err := Worktrees(ctx, s.Root)
	if err != nil {
		return a, err
	}
	if i := slices.IndexFunc(wts, func(wt Worktree) bool { return wt.Branch == a.Branch }); i >= 0 {
		a.Worktree = &wts[i]
	}
	if session {
		a.Session = s.session(ctx, a.Branch)
	}
	return a, nil
}

// session is wtx's session on branch, nil when wtx doesn't answer.
// ponytail: a failing wtx status (no wtx.toml yet) reads as no session;
// doctor says why.
func (s *Service) session(ctx context.Context, branch string) *wtx.Session {
	if !wtx.Available() {
		return nil
	}
	out, err := wtx.Run(ctx, s.Root, "status", branch, "--json")
	if err != nil {
		return nil
	}
	if rows, err := wtx.ParseStatus(out); err == nil && len(rows) > 0 {
		return &rows[0]
	}
	return nil
}

// Show is the issue's agent.
func (s *Service) Show(ctx context.Context, ref string) (Agent, error) {
	_, _, a, err := s.look(ctx, ref, true)
	return a, err
}

// Act runs action on the issue's agent: go (wtx go: a worktree, a
// session, the agent started on a prompt), brief (wtx brief: a prompt to
// the running agent, with a review thread when req.Comment names a line
// comment), approve (a brief in our words once the plan is approved on
// the board). The agent's permissions are wtx.toml's: no --agent, --llm
// or permission flag is passed.
//
// go at step 3 starts the agent on the plan, or, at low risk or with the
// plan approved, on step 4 and moves the status to in-progress.
func (s *Service) Act(ctx context.Context, action, ref string, req Request) (Agent, error) {
	if !slices.Contains([]string{"go", "brief", "approve"}, action) {
		return Agent{}, fmt.Errorf("no agent action %q", action)
	}
	if req.Comment != "" && action != "brief" {
		return Agent{}, errors.New("a review thread goes with brief only")
	}
	if !wtx.Available() {
		return Agent{}, errNoWtx
	}
	th, cyc, a, err := s.look(ctx, ref, false)
	if err != nil {
		return a, err
	}
	switch {
	case action == "go" && a.Worktree != nil:
		return a, refuse("has-worktree", "This issue has a worktree: brief the agent instead.")
	case action == "go" && a.Branch == "all":
		return a, refuse("branch-all", "wtx reads the branch all as every worktree: give the issue another branch.")
	case action != "go" && a.Worktree == nil:
		return a, refuse("no-worktree", "No worktree yet: start an agent first.")
	case action == "approve" && cyc == nil:
		return a, refuse("no-cycle", "Approve reads the plan from the board's cycle: run git board here, with the cycle on.")
	case action == "approve" && cyc.Evidence.Plan == nil:
		return a, refuse("no-plan", "No plan file on the branch: nothing to approve to the agent.")
	case action == "approve" && cyc.Evidence.PlanApproval == nil:
		return a, refuse("not-approved", "The plan isn't approved on the board yet: approve it first.")
	}
	// at ready, low risk or the plan approved: the branch starts at step 4.
	// The board computes the step from the status, so the status moves
	// first, and moves back if wtx go fails (v1 computed the step as if
	// moved, and moved it after).
	start := action == "go" && cyc != nil && cyc.Stage == "ready" && len(cyc.Stages.InProgress) > 0 &&
		(cyc.Risk == "low" || board.ApprovedPlan(cyc.Evidence))
	was := th.Field("status")
	if start {
		if err := s.Board.SetStatus(ctx, th.ID, cyc.Stages.InProgress[0]); err != nil {
			return a, err
		}
		if cyc, err = s.Board.Cycle(ctx, th.ID); err != nil {
			return a, errors.Join(err, s.restore(th.ID, was))
		}
	}
	in := wtx.PromptInput{Issue: th.Name(), Title: th.Title, Branch: a.Branch, Brief: req.Brief, Description: th.Description()}
	if cyc != nil {
		in.Step, in.Of, in.Name, in.Gate, in.What = cyc.Position(), cyc.Of, cyc.Name, cyc.Gate, cyc.Waits.What
		if cyc.Evidence.Plan != nil {
			in.Plan = cyc.Evidence.Plan.Path
		}
	}
	switch {
	case action == "approve":
		in.Brief = ""
		by := "" // who approved, when the board knows their login
		if l := cyc.Evidence.PlanApproval.By; l != "" {
			by = " by " + l
		}
		in.Then = fmt.Sprintf("The plan is approved%s on the board. Set `status: accepted` in the plan file's front matter, commit and push it, then go on with step %d.", by, in.Step)
	case action == "go" && cyc != nil && cyc.Step == 3 && cyc.Stage == "ready":
		in.What, in.Then = "write the plan, in plan mode", planThen(cyc.Stages, th, a.Branch)
	}
	if req.Comment != "" {
		rt, err := s.Board.ReviewThread(ctx, th.ID)
		if err != nil {
			return a, err
		}
		what, quoted, ok := reviewThread(rt, req.Comment)
		if !ok {
			return a, errors.New("send a line comment's thread: " + req.Comment + " is no line comment on the issue")
		}
		in.What, in.Thread = what, quoted
	}
	file, err := tempFile("board-agents-brief-*.md", []byte(wtx.Prompt(in)))
	if err != nil {
		return a, errors.Join(err, s.restoreIf(start, th.ID, was))
	}
	defer os.Remove(file) // wtx copied it into the worktree

	args, timeout := []string{"go", a.Branch, "--prompt", file, "--no-attach"}, 10*time.Minute
	if action != "go" {
		args, timeout = []string{"brief", a.Branch, "--prompt", file}, time.Minute
		// past the plan, the agent carries on its conversation in its build
		// phase; Revise at step 3 restarts the planner in plan mode
		if action == "approve" || cyc != nil && cyc.Step > 3 {
			args = append(args, "--build")
		}
	}
	// wtx go is not cut halfway by the caller going away: that leaves a
	// worktree without a session, which go then refuses
	wctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer cancel()
	out, err := wtx.Run(wctx, s.Root, args...)
	if err != nil {
		return a, errors.Join(err, s.restoreIf(start, th.ID, was))
	}
	if a, err = s.of(context.WithoutCancel(ctx), th, cyc, true); err != nil {
		return a, err
	}
	if start {
		a.Moved = cyc.Stages.InProgress[0]
	}
	a.Output = wtx.Tail(string(out), 8)
	return a, nil
}

func (s *Service) restoreIf(start bool, id, was string) error {
	if !start {
		return nil
	}
	return s.restore(id, was)
}

// restore sets the status back to was, after a start that failed.
func (s *Service) restore(id, was string) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := s.Board.SetStatus(ctx, id, was); err != nil {
		return fmt.Errorf("the status moved and could not move back to %q: %w", was, err)
	}
	return nil
}

// Stop ends the agent's session and frees its ports, the worktree kept:
// wtx teardown --path <worktree>.
func (s *Service) Stop(ctx context.Context, ref string) (Agent, error) {
	if !wtx.Available() {
		return Agent{}, errNoWtx
	}
	_, _, a, err := s.look(ctx, ref, false)
	if err != nil {
		return a, err
	}
	if a.Worktree == nil {
		return a, refuse("no-worktree", "No worktree for "+a.Branch+": no agent to stop.")
	}
	out, err := wtx.Run(ctx, s.Root, "teardown", "--path", a.Worktree.Path)
	if err != nil {
		return a, err
	}
	a.Session = s.session(ctx, a.Branch)
	a.Output = wtx.Tail(string(out), 8)
	return a, nil
}

// Done closes the agent's worktree, the branch kept: wtx done <branch>,
// --force discarding what is uncommitted.
func (s *Service) Done(ctx context.Context, ref string, force bool) (Agent, error) {
	if !wtx.Available() {
		return Agent{}, errNoWtx
	}
	_, _, a, err := s.look(ctx, ref, false)
	if err != nil {
		return a, err
	}
	if a.Worktree == nil {
		return a, refuse("no-worktree", "No worktree for "+a.Branch+": nothing to close.")
	}
	args := []string{"done", a.Branch}
	if force {
		args = append(args, "--force")
	}
	out, err := wtx.Run(ctx, s.Root, args...)
	if err != nil {
		return a, err
	}
	a.Worktree, a.Session = nil, nil
	a.Output = wtx.Tail(string(out), 8)
	return a, nil
}

// Row is one linked worktree: its branch, wtx's session there, and the
// issue the board says its branch names.
type Row struct {
	Path    string       `json:"path"`
	Branch  string       `json:"branch"` // the head's short sha when detached
	Issue   string       `json:"issue,omitempty"`
	Title   string       `json:"title,omitempty"`
	Step    string       `json:"step,omitempty"`
	State   string       `json:"state,omitempty"` // the board's: "Waits for you", "No issue"
	Session *wtx.Session `json:"session,omitempty"`
}

// List is every linked worktree with its session (wtx status --json) and,
// when the board's server runs with the cycle on, its issue (the Work
// page's agents). note says what is missing and why, "" when nothing is.
func (s *Service) List(ctx context.Context) (rows []Row, note string, err error) {
	wts, err := Worktrees(ctx, s.Root)
	if err != nil {
		return nil, "", err
	}
	var notes []string
	var sessions []wtx.Session
	if !wtx.Available() {
		notes = append(notes, errNoWtx.Error())
	} else if out, err := wtx.Run(ctx, s.Root, "status", "--json"); err != nil {
		notes = append(notes, err.Error())
	} else if sessions, err = wtx.ParseStatus(out); err != nil {
		notes = append(notes, err.Error())
	}
	work, err := s.Board.Agents(ctx)
	if err != nil {
		notes = append(notes, "no issues: "+err.Error())
	}
	for _, wt := range wts {
		r := Row{Path: wt.Path, Branch: wt.Branch}
		if r.Branch == "" {
			r.Branch = wt.Head[:min(7, len(wt.Head))]
		}
		if i := slices.IndexFunc(sessions, func(x wtx.Session) bool { return x.Branch == wt.Branch }); i >= 0 {
			r.Session = &sessions[i]
		}
		if i := slices.IndexFunc(work, func(w board.WorkAgent) bool { return w.Path == wt.Path || w.Branch == r.Branch }); i >= 0 {
			w := work[i]
			r.State = w.State
			if w.Issue != nil {
				r.Issue, r.Title = w.Issue.Name(), w.Issue.Title
			}
			if w.Step > 0 {
				r.Step = fmt.Sprintf("%d: %s", w.Step, w.Name)
			}
		}
		rows = append(rows, r)
	}
	return rows, strings.Join(notes, "; "), nil
}

// planThen is what the agent does with its plan once the person accepts
// it in the pane, where plan mode ends: commit it where the board finds
// it, push, and open the draft PR that holds it, unless plans-in-pr =
// false. The plan is approved on the board, not by the agent.
func planThen(stages board.Stages, th board.Thread, branch string) string {
	file := path.Join(stages.Plans, path.Base(branch)+".md")
	pr := "open a draft PR that holds only the plan (`gh pr create --draft`)"
	if !stages.PlansInPR {
		pr = "open no PR at this step: the PR comes at step 5"
	}
	return fmt.Sprintf("Once the person accepts the plan in this pane, commit it as `%s`, its front matter `issue: %s` and `status: draft` (git-board finds the plan by that `issue:` line), push the branch, %s, and stop. The plan is approved on the board, and you are briefed then: write no code before.", file, th.PlanKey(), pr)
}

// reviewThread is the line comment comment, or the thread it replies to,
// as the agent reads it: what is asked (our words) and the thread
// written out to be quoted (the commenters' words). false: no such line
// comment on the issue.
func reviewThread(th board.Thread, comment string) (what, quoted string, ok bool) {
	comment = commentID(th, comment)
	root := comment
	if i := slices.IndexFunc(th.Events, func(e board.Event) bool { return e.ID == comment }); i < 0 || th.Events[i].Review == nil || th.Events[i].Review.Path == "" {
		return "", "", false
	} else if to := th.Events[i].Review.ReplyTo; to != "" {
		root = to
	}
	var b strings.Builder
	var rv *board.Review
	for _, e := range th.Events {
		if e.Review == nil || e.ID != root && e.Review.ReplyTo != root {
			continue
		}
		if rv == nil {
			if e.ID != root {
				return "", "", false // the reply's thread starts elsewhere
			}
			rv = e.Review
			fmt.Fprintf(&b, "%s line %d (%s side), at %.7s", rv.Path, rv.Line, rv.Side, rv.Commit)
			switch {
			case rv.Outdated:
				b.WriteString(", outdated: the line changed since")
			case rv.HeadLine > 0 && rv.HeadLine != rv.Line:
				fmt.Fprintf(&b, ", now line %d at the branch's head", rv.HeadLine)
			}
			if rv.Text != "" {
				fmt.Fprintf(&b, "\n%s", rv.Text)
			}
		}
		fmt.Fprintf(&b, "\n\n%s, %s:\n%s", e.Author.Name, e.At.UTC().Format(time.RFC3339), strings.TrimSpace(e.Body))
	}
	if rv == nil {
		return "", "", false
	}
	what = fmt.Sprintf("answer the review thread on %s line %d: change the code in the worktree as it asks, or say why not. Don't resolve the thread, approve or request changes: a person does", rv.Path, rv.Line)
	return what, b.String(), true
}

// commentID is the event ref names: its whole id (hash:n), or the ref git
// board show prints, a prefix of the hash with its :n (9c8b7a6:1), when
// it names one event. ref itself otherwise.
func commentID(th board.Thread, ref string) string {
	hash, n, _ := strings.Cut(ref, ":")
	found := ""
	for _, e := range th.Events {
		if e.ID == ref {
			return ref
		}
		h, m, _ := strings.Cut(e.ID, ":")
		if hash != "" && strings.HasPrefix(h, hash) && m == n {
			if found != "" {
				return ref // several: none
			}
			found = e.ID
		}
	}
	if found == "" {
		return ref
	}
	return found
}

// tempFile writes data to a new temp file and returns its path.
func tempFile(pattern string, data []byte) (string, error) {
	f, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", err
	}
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}
