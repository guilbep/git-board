package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/guilbep/git-board/internal/branches"
	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/cycle"
	"github.com/guilbep/git-board/internal/fields"
	"github.com/guilbep/git-board/internal/gitbugio"
	"github.com/guilbep/git-board/internal/wtx"
)

// Why wtx can't run from here, by code.
var wtxRefusals = map[string]struct {
	status int
	reason string
}{
	"exposed": {http.StatusForbidden, "The server listens beyond this machine: agents start only from a server on localhost."},
	"no-wtx":  {http.StatusConflict, "wtx is not on PATH where git board serve runs."},
	"origin":  {http.StatusForbidden, "wtx runs only on a request from this board's own page."},
}

// wtxAllowed is the one gate every wtx run passes: loopback only, wtx
// on PATH, and for a write a same-host Origin (withGuard lets a POST
// without Origin through; this doesn't). code is "" when allowed.
func (s *apiServer) wtxAllowed(r *http.Request, write bool) (code, reason string) {
	switch origin := r.Header.Get("Origin"); {
	case len(s.exposed) > 0:
		code = "exposed"
	case !wtx.Available():
		code = "no-wtx"
	case write && (origin == "" || !sameHost(origin, r.Host) && !devOrigins[origin]):
		code = "origin"
	default:
		return "", ""
	}
	return code, wtxRefusals[code].reason
}

// refuseWtx answers the refusal and tells whether it did.
func (s *apiServer) refuseWtx(w http.ResponseWriter, r *http.Request) bool {
	code, reason := s.wtxAllowed(r, true)
	if code != "" {
		writeCoded(w, wtxRefusals[code].status, code, errors.New(reason))
	}
	return code != ""
}

type worktreesResponse struct {
	Local   bool            `json:"local"`             // the server listens on loopback alone
	Wtx     bool            `json:"wtx"`               // on PATH
	Reason  string          `json:"reason,omitempty"`  // why nothing can start from here; "" when it can
	Code    string          `json:"code,omitempty"`    // exposed, no-wtx, no-config
	Doctor  *wtx.Doctor     `json:"doctor,omitempty"`  // wtx doctor --json
	Config  bool            `json:"config"`            // wtx.toml in the repository
	Answers json.RawMessage `json:"answers,omitempty"` // wtx init --edit --no-notes
}

func (s *apiServer) handleWorktrees(w http.ResponseWriter, r *http.Request) {
	out := worktreesResponse{Local: len(s.exposed) == 0, Wtx: wtx.Available()}
	if out.Code, out.Reason = s.wtxAllowed(r, false); out.Code != "" {
		writeJSON(w, http.StatusOK, out)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Minute)
	defer cancel()
	// doctor exits 1 on a hard failure, its report still on stdout
	report, _ := wtx.Run(ctx, s.root, "doctor", "--json")
	var d wtx.Doctor
	if err := json.Unmarshal(report, &d); err != nil {
		writeError(w, http.StatusBadGateway, errors.New("wtx doctor --json: "+err.Error()))
		return
	}
	out.Doctor, out.Config = &d, d.Has("wtx.toml")
	answers, err := wtx.Run(ctx, s.root, "init", "--edit", "--no-notes")
	if err == nil && !json.Valid(answers) {
		err = errors.New("wtx init --edit: not JSON")
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	out.Answers = answers
	if !out.Config {
		out.Code, out.Reason = "no-config", "No wtx.toml in this repository yet"
	}
	writeJSON(w, http.StatusOK, out)
}

type wtxFiles struct {
	Files []wtxFile `json:"files"` // wtx.toml, .wt.toml
}

type wtxFile struct {
	Path string `json:"path"`
	Text string `json:"text"`
}

// handleWtxPreview runs wtx init --from-json in a scratch repository and
// answers what it wrote: the bytes Write puts in the checkout.
func (s *apiServer) handleWtxPreview(w http.ResponseWriter, r *http.Request) {
	s.wtxInit(w, r, false)
}

// handleWtxInit writes wtx.toml and .wt.toml in the checkout, uncommitted.
func (s *apiServer) handleWtxInit(w http.ResponseWriter, r *http.Request) {
	s.wtxInit(w, r, true)
}

func (s *apiServer) wtxInit(w http.ResponseWriter, r *http.Request, write bool) {
	if s.refuseWtx(w, r) {
		return
	}
	var body struct {
		Answers json.RawMessage `json:"answers"`
	}
	if !decode(w, r, &body) {
		return
	}
	if len(body.Answers) == 0 || string(body.Answers) == "null" {
		writeError(w, http.StatusBadRequest, errors.New("no answers"))
		return
	}
	file, err := tempFile("git-board-wtx-*.json", body.Answers)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer os.Remove(file)
	ctx, cancel := context.WithTimeout(r.Context(), time.Minute)
	defer cancel()

	dir, args := s.root, []string{"init", "--from-json", file}
	if write {
		// ponytail: --force always; the preview is what replaces the
		// file, and wtx's --edit carried its values.
		args = append(args, "--force")
		if !s.lockWtx(w) {
			return
		}
		defer s.wtxMu.Unlock()
	} else {
		if dir, err = os.MkdirTemp("", "git-board-wtx-preview-"); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		defer os.RemoveAll(dir)
		if out, err := exec.CommandContext(ctx, "git", "init", "-q", dir).CombinedOutput(); err != nil {
			writeError(w, http.StatusInternalServerError, errors.New("git init: "+string(out)))
			return
		}
	}
	if _, err := wtx.Run(ctx, dir, args...); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	// ponytail: read back from the cwd; wtx writes in the main checkout,
	// which is where git board serve runs (git-bug's lock is there)
	var out wtxFiles
	for _, name := range []string{"wtx.toml", ".wt.toml"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		out.Files = append(out.Files, wtxFile{Path: name, Text: string(data)})
	}
	writeJSON(w, http.StatusOK, out)
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

// issueAgent is an issue's agent: its branch, the worktree on it, and
// wtx's session there.
type issueAgent struct {
	Reason   string             `json:"reason,omitempty"` // why Start is refused here; "" when it can start
	Code     string             `json:"code,omitempty"`
	Branch   string             `json:"branch"`             // the evidence branch, else the [branches] name for the issue
	Worktree *gitbugio.Worktree `json:"worktree,omitempty"` // the linked worktree on that branch
	Session  *wtx.Session       `json:"session,omitempty"`  // wtx status for it, when wtx answers
	Output   string             `json:"output,omitempty"`   // a write: wtx's last lines
}

// agentOf reads the issue's agent: the branch and worktree from git, and
// with session the session from wtx when it may run.
func (s *apiServer) agentOf(ctx context.Context, r *http.Request, th gitbugio.Thread, cyc cycleResponse, session bool) (issueAgent, error) {
	a := issueAgent{Branch: cyc.Evidence.Branch}
	a.Code, a.Reason = s.wtxAllowed(r, false)
	if a.Branch == "" {
		sug, err := branches.Suggest(s.b, s.config().Branches, th.Issue)
		if err != nil {
			return a, err
		}
		a.Branch = sug.Suggested
	}
	wts, err := s.b.Worktrees()
	if err != nil {
		return a, err
	}
	if i := slices.IndexFunc(wts, func(wt gitbugio.Worktree) bool { return wt.Branch == a.Branch }); i >= 0 {
		a.Worktree = &wts[i]
	}
	if session && a.Code == "" {
		// ponytail: a failing wtx status (no wtx.toml yet) reads as no
		// session; the Worktrees tab says why
		if out, err := wtx.Run(ctx, s.root, "status", a.Branch, "--json"); err == nil {
			if rows, err := wtx.ParseStatus(out); err == nil && len(rows) > 0 {
				a.Session = &rows[0]
			}
		}
	}
	return a, nil
}

// GET /api/issues/{id}/agent
func (s *apiServer) handleIssueAgent(w http.ResponseWriter, r *http.Request) {
	th, err := s.b.Thread(r.PathValue("id"))
	if err != nil {
		writeIssueError(w, err)
		return
	}
	cyc, err := s.cycleOf(r.Context(), th)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	a, err := s.agentOf(r.Context(), r, th, cyc, true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

// lockWtx takes the one wtx write, or answers 409 busy: queued, a second
// Start would wait in silence behind a wtx go of up to ten minutes.
func (s *apiServer) lockWtx(w http.ResponseWriter) bool {
	if s.wtxMu.TryLock() {
		return true
	}
	writeCoded(w, http.StatusConflict, "busy", errors.New("wtx is busy starting an agent or writing wtx.toml: try again when it's done."))
	return false
}

// buildBrief: wtx brief --build briefs the agent in its build phase,
// where today's wtx always restarts the pane in plan mode.
// ponytail: off until EPFL-ENAC/wtx has the flag (argparse refuses it)
const buildBrief = false

// POST /api/issues/{id}/agent/{action}, action go, brief or approve, body
// {"brief": "...", "comment": "..."}: wtx go or wtx brief on the issue's
// branch, with a prompt file of git-board's words and the issue quoted as
// data. comment, brief only, is a line comment's id: its thread is quoted
// too, and the agent is asked to answer it, never to approve. The
// agent's permissions are wtx.toml's: no --agent, --llm or permission
// flag is passed.
//
// go at step 3 starts the agent on the plan, or, at low risk or with the
// plan approved, on step 4 and moves the status to in-progress. approve
// is a brief in the server's words, after approve-plan: the plan is
// approved, on to step 4.
func (s *apiServer) handleIssueAgentAction(w http.ResponseWriter, r *http.Request) {
	action := r.PathValue("action")
	if !slices.Contains([]string{"go", "brief", "approve"}, action) {
		writeError(w, http.StatusNotFound, fmt.Errorf("no agent action %q", action))
		return
	}
	if s.refuseWtx(w, r) {
		return
	}
	var req struct {
		Brief   string `json:"brief"`
		Comment string `json:"comment"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.Comment != "" && action != "brief" {
		writeError(w, http.StatusBadRequest, errors.New("a review thread goes with brief only"))
		return
	}
	if !s.lockWtx(w) {
		return
	}
	defer s.wtxMu.Unlock()
	th, err := s.b.Thread(r.PathValue("id"))
	if err != nil {
		writeIssueError(w, err)
		return
	}
	cyc, err := s.cycleOf(r.Context(), th)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	a, err := s.agentOf(r.Context(), r, th, cyc, false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	switch {
	case action == "go" && a.Worktree != nil:
		writeCoded(w, http.StatusConflict, "has-worktree", errors.New("This issue has a worktree: brief the agent instead."))
		return
	case action == "go" && a.Branch == "all":
		writeCoded(w, http.StatusConflict, "branch-all", errors.New("wtx reads the branch all as every worktree: give the issue another branch."))
		return
	case action != "go" && a.Worktree == nil:
		writeCoded(w, http.StatusConflict, "no-worktree", errors.New("No worktree yet: start an agent first."))
		return
	case action == "approve" && cyc.Evidence.Plan == nil:
		writeCoded(w, http.StatusConflict, "no-plan", errors.New("No plan file on the branch: nothing to approve to the agent."))
		return
	case action == "approve" && cyc.Evidence.PlanApproval == nil:
		writeCoded(w, http.StatusConflict, "not-approved", errors.New("The plan isn't approved on the board yet: approve it first."))
		return
	}
	// at ready, low risk or the plan approved: the branch starts at step
	// 4, the status moved once wtx go has worked
	start := action == "go" && cyc.Stage == "ready" && len(cyc.Stages.InProgress) > 0 &&
		(cyc.Risk == "low" || cycle.ApprovedPlan(cyc.Evidence))
	if start {
		if cyc, err = s.cycleOf(r.Context(), withStatus(th, cyc.Stages.InProgress[0])); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	in := wtx.PromptInput{Issue: issueName(th.Issue), Title: th.Title, Step: cyc.Step, Of: cyc.Of, Name: cyc.Name,
		Gate: cyc.Gate, What: cyc.Waits.What, Branch: a.Branch, Brief: req.Brief}
	switch {
	case action == "approve":
		in.Brief = ""
		by := "" // who approved, when the board knows their login
		if l := cyc.Evidence.PlanApproval.By; l != "" {
			by = " by " + l
		}
		in.Then = fmt.Sprintf("The plan is approved%s on the board. Set `status: accepted` in the plan file's front matter, commit and push it, then go on with step %d.", by, cyc.Step)
	case action == "go" && cyc.Step == 3 && cyc.Stage == "ready":
		in.What, in.Then = "write the plan, in plan mode", planThen(cyc.Stages, th.Issue, a.Branch)
	}
	if i := slices.IndexFunc(th.Events, func(e gitbugio.Event) bool { return e.Kind == "created" }); i >= 0 {
		in.Description = th.Events[i].Body
	}
	if cyc.Evidence.Plan != nil {
		in.Plan = cyc.Evidence.Plan.Path
	}
	if req.Comment != "" {
		what, quoted, ok := s.reviewThread(&th, req.Comment)
		if !ok {
			writeError(w, http.StatusBadRequest, errors.New("send a line comment's thread"))
			return
		}
		in.What, in.Thread = what, quoted
	}
	file, err := tempFile("git-board-brief-*.md", []byte(wtx.Prompt(in)))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer os.Remove(file) // wtx copied it into the worktree

	// ponytail: the request waits for wtx go (npm ci included); a job
	// with a progress stream when a repo's setup takes longer than a
	// browser waits.
	args, timeout := []string{"go", a.Branch, "--prompt", file, "--no-attach"}, 10*time.Minute
	if action != "go" {
		args, timeout = []string{"brief", a.Branch, "--prompt", file}, time.Minute
		if buildBrief {
			args = append(args, "--build")
		}
	}
	// a closed tab doesn't kill wtx halfway: that leaves a worktree
	// without a session, which Start then refuses
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), timeout)
	defer cancel()
	out, err := wtx.Run(ctx, s.root, args...)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if start {
		if _, err := fields.Apply(s.b, s.config(), th.ID, "status", cyc.Stages.InProgress[0], s.me, time.Now()); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	if a, err = s.agentOf(r.Context(), r, th, cyc, true); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	a.Output = wtx.Tail(string(out), 8)
	writeJSON(w, http.StatusOK, a)
}

// withStatus is th with its status label set to status: the step the
// issue will be at once it moves.
func withStatus(th gitbugio.Thread, status string) gitbugio.Thread {
	labels := slices.DeleteFunc(slices.Clone(th.Issue.Labels), func(l string) bool { return strings.HasPrefix(l, "status:") })
	th.Issue.Labels = append(labels, "status:"+status)
	return th
}

// planThen is what the agent does with its plan once the person accepts
// it in the pane, where plan mode ends: commit it where gather finds it,
// push, and open the draft PR that holds it, unless plans-in-pr = false.
// The plan is approved on the board, not by the agent.
func planThen(stages config.Stages, is gitbugio.Issue, branch string) string {
	key := cycle.PlanIssue(is)
	file := path.Join(stages.Plans, path.Base(branch)+".md")
	pr := "open a draft PR that holds only the plan (`gh pr create --draft`)"
	if !stages.PlansInPR {
		pr = "open no PR at this step: the PR comes at step 5"
	}
	return fmt.Sprintf("Once the person accepts the plan in this pane, commit it as `%s`, its front matter `issue: %s` and `status: draft` (git-board finds the plan by that `issue:` line), push the branch, %s, and stop. The plan is approved on the board, and you are briefed then: write no code before.", file, key, pr)
}

// reviewThread is the line comment comment, or the thread it replies to,
// as the agent reads it: what is asked (git-board's words) and the thread
// written out to be quoted (the commenters' words). false: no such line
// comment on the issue.
func (s *apiServer) reviewThread(th *gitbugio.Thread, comment string) (what, quoted string, ok bool) {
	s.annotate(th)
	root := comment
	if i := slices.IndexFunc(th.Events, func(e gitbugio.Event) bool { return e.ID == comment }); i < 0 || th.Events[i].Review == nil || th.Events[i].Review.Path == "" {
		return "", "", false
	} else if to := th.Events[i].Review.ReplyTo; to != "" {
		root = to
	}
	var b strings.Builder
	var rv *gitbugio.Review
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

// issueName names the issue as the Work page does: its project number,
// else its first forge number, else its short id.
func issueName(is gitbugio.Issue) string {
	if is.Number != "" {
		return is.Number
	}
	for _, f := range is.Forges {
		if f.Number != "" {
			return "#" + f.Number
		}
	}
	return is.ID[:min(7, len(is.ID))]
}
