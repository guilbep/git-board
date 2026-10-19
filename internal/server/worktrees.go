package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"time"

	"github.com/guilbep/git-board/internal/branches"
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
		s.wtxMu.Lock()
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

// POST /api/issues/{id}/agent/{action}, action go or brief, body
// {"brief": "..."}: wtx go or wtx brief on the issue's branch, with a
// prompt file of git-board's words and the issue quoted as data. The
// agent's permissions are wtx.toml's: no --agent, --llm or permission
// flag is passed.
func (s *apiServer) handleIssueAgentAction(w http.ResponseWriter, r *http.Request) {
	action := r.PathValue("action")
	if action != "go" && action != "brief" {
		writeError(w, http.StatusNotFound, fmt.Errorf("no agent action %q", action))
		return
	}
	if s.refuseWtx(w, r) {
		return
	}
	var req struct {
		Brief string `json:"brief"`
	}
	if !decode(w, r, &req) {
		return
	}
	s.wtxMu.Lock()
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
	case action == "brief" && a.Worktree == nil:
		writeCoded(w, http.StatusConflict, "no-worktree", errors.New("No worktree yet: start an agent first."))
		return
	}
	in := wtx.PromptInput{Issue: issueName(th.Issue), Title: th.Title, Step: cyc.Step, Of: cyc.Of, Name: cyc.Name,
		Gate: cyc.Gate, What: cyc.Waits.What, Branch: a.Branch, Brief: req.Brief}
	if i := slices.IndexFunc(th.Events, func(e gitbugio.Event) bool { return e.Kind == "created" }); i >= 0 {
		in.Description = th.Events[i].Body
	}
	if cyc.Evidence.Plan != nil {
		in.Plan = cyc.Evidence.Plan.Path
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
	if action == "brief" {
		args, timeout = []string{"brief", a.Branch, "--prompt", file}, time.Minute
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	out, err := wtx.Run(ctx, s.root, args...)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if a, err = s.agentOf(r.Context(), r, th, cyc, true); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	a.Output = wtx.Tail(string(out), 8)
	writeJSON(w, http.StatusOK, a)
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
