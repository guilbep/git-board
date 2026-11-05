// Package board talks to a git-board board the way any client does:
// through the git-board CLI (its --json outputs and exit codes, the
// contract git-board keeps, docs/guide/cli.md) and, for what only the
// server computes, through its HTTP API. It never reads the board's
// storage.
//
// What goes where:
//
//   - the CLI: an issue's thread (show --json), its branch name (branch),
//     its status (set), a comment without a server (comment -b);
//   - the HTTP API, when a server runs: the board it serves (GET
//     /api/board), the issue's cycle (GET /api/issues/{id}/cycle), its
//     thread (GET /api/issues/{id}), the Work page's rows (GET
//     /api/cycle?who=team), a comment (POST /api/issues/{id}/comments,
//     If-Match the version the caller read).
//
// The routes are git-board v2's (docs/rewrite/api.md): the error shape
// {"code", "message", "field"}, versions in ETag and If-Match. They are
// all in this file. What v2 doesn't answer yet (a comment's cycle step,
// line comments located on the branch, the worktrees' paths) is said
// where it is used: the client writes and reads without it, and says so.
package board

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// ErrNotFound is the CLI's exit code 3: no such issue or comment.
var ErrNotFound = errors.New("no such issue")

// ErrOff is a route the board answers 404 {"code": "experimental"}: the
// feature is off in .git-board.toml (ADR 0018).
var ErrOff = errors.New("off on this board")

// ErrStale is 412 {"code": "stale"}: the issue moved from the version an
// If-Match named, and nothing was written.
var ErrStale = errors.New("the issue changed since it was read")

// Client is a board: the git-board binary run in Dir, and the board's
// server at URL ("" when none runs).
type Client struct {
	Bin  string // "git-board", or a path
	Dir  string // the repository the CLI runs in
	URL  string // http://127.0.0.1:8080, no trailing slash; "" for none
	HTTP *http.Client
}

// Person is a person as the CLI prints one.
type Person struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Login string `json:"login,omitempty"`
}

// Forge is where an issue is on a forge.
type Forge struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Number string `json:"number"`
}

// Review is a line comment's anchor; v1's server filled Branch,
// Outdated, HeadLine and Text from the issue's branches (Located).
type Review struct {
	Path     string `json:"path,omitempty"`
	Line     int    `json:"line,omitempty"`
	Side     string `json:"side,omitempty"`
	Commit   string `json:"commit,omitempty"`
	ReplyTo  string `json:"reply_to,omitempty"`
	Branch   string `json:"branch,omitempty"`
	Outdated bool   `json:"outdated,omitempty"`
	HeadLine int    `json:"head_line,omitempty"`
	Text     string `json:"text,omitempty"`
}

// Event is one entry of a thread: created, comment, labels, status, title.
type Event struct {
	Kind   string            `json:"kind"`
	ID     string            `json:"id"`
	Author Person            `json:"author"`
	At     time.Time         `json:"at"`
	Body   string            `json:"body,omitempty"`
	Meta   map[string]string `json:"meta,omitempty"`
	Review *Review           `json:"review,omitempty"`
}

// Thread is an issue with its events, as show --json prints it.
type Thread struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Status  string   `json:"status"` // open, closed
	Labels  []string `json:"labels"`
	Number  string   `json:"number,omitempty"`
	Counted bool     `json:"counted,omitempty"`
	Forges  []Forge  `json:"forges,omitempty"`
	Events  []Event  `json:"events"`
	// Version is the issue's version, as the API answers it (its ETag):
	// what a write sends back in If-Match. "" from the CLI.
	Version string `json:"version,omitempty"`
}

// Field is the value of a single field, from its label: status:ready
// gives ready for status. "" when it has none.
func (th Thread) Field(name string) string {
	for _, l := range th.Labels {
		if v, ok := strings.CutPrefix(l, name+":"); ok {
			return v
		}
	}
	return ""
}

// Description is the issue's text: its created event's body.
func (th Thread) Description() string {
	if i := slices.IndexFunc(th.Events, func(e Event) bool { return e.Kind == "created" }); i >= 0 {
		return th.Events[i].Body
	}
	return ""
}

// Name names the issue as the Work page does: its project number, else
// its first forge number, else its short id.
func (th Thread) Name() string {
	if th.Number != "" {
		return th.Number
	}
	for _, f := range th.Forges {
		if f.Number != "" {
			return "#" + f.Number
		}
	}
	return th.ID[:min(7, len(th.ID))]
}

// PlanKey is what a plan file's issue: line holds for the issue
// (git-board's cycle.PlanIssue): the bare project number when the project
// numbers its issues, else its first forge number, else its short id.
func (th Thread) PlanKey() string {
	if th.Counted && th.Number != "" {
		return th.Number[strings.LastIndexByte(th.Number, '-')+1:]
	}
	for _, f := range th.Forges {
		if f.Number != "" {
			return f.Number
		}
	}
	return th.ID[:min(7, len(th.ID))]
}

// cli runs git-board with args in Dir and returns its stdout. Exit code 3
// is ErrNotFound; any other failure carries git-board's stderr.
func (c *Client) cli(ctx context.Context, args ...string) ([]byte, error) {
	bin := c.Bin
	if bin == "" {
		bin = "git-board"
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = c.Dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(stderr.String()), "error: "))
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 3 {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, msg)
		}
		return nil, fmt.Errorf("git-board %s: %w: %s", args[0], err, msg)
	}
	return stdout.Bytes(), nil
}

// Thread is the issue ref names (a short id, CO2-3075, #2528), with its
// comments: git-board show <ref> --json.
func (c *Client) Thread(ctx context.Context, ref string) (Thread, error) {
	var th Thread
	out, err := c.cli(ctx, "show", ref, "--json")
	if err != nil {
		return th, err
	}
	if err := json.Unmarshal(out, &th); err != nil {
		return th, fmt.Errorf("git-board show --json: %w", err)
	}
	return th, nil
}

// Branch is the issue's branch name as [branches] makes it: the first
// line git-board branch <id> prints.
func (c *Client) Branch(ctx context.Context, id string) (string, error) {
	out, err := c.cli(ctx, "branch", id)
	if err != nil {
		return "", err
	}
	name, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	if name == "" {
		return "", errors.New("git-board branch printed no name")
	}
	return name, nil
}

// SetStatus sets the issue's status: git-board set <id> status=<status>,
// status= to clear it.
func (c *Client) SetStatus(ctx context.Context, id, status string) error {
	_, err := c.cli(ctx, "set", id, "status="+status)
	return err
}

// NoStep is the note of a comment asked in a cycle step that the board
// can't record: git-board v2's comments take {body} alone (api.md, routes
// 9 to 11: a comment's meta comes with its domain), so the comment is
// written in no step, as git-board comment writes it.
const NoStep = "written in no cycle step: git-board's API takes no comment metadata (cycle:step) yet"

// Written is what Comment wrote: Note says what was asked and not
// written, "" when nothing was left out.
type Written struct {
	ID   string `json:"id,omitempty"` // the comment's id, when the server names it
	Note string `json:"note,omitempty"`
}

// Comment writes a comment on the issue, through the API when a server
// runs, else through git-board comment. step is the cycle step it answers
// ("" none): no board records it yet (NoStep). version is the issue's
// version the caller read it at, sent as If-Match: ErrStale when the
// issue moved since, and nothing written; "" writes whatever the issue is
// now.
func (c *Client) Comment(ctx context.Context, id, body, step, version string) (Written, error) {
	var w Written
	if step != "" {
		w.Note = NoStep
	}
	if c.URL == "" {
		_, err := c.cli(ctx, "comment", id, "-b", body)
		return w, err
	}
	res, err := c.do(ctx, http.MethodPost, "/api/issues/"+url.PathEscape(id)+"/comments", version, map[string]string{"body": body}, nil)
	if err != nil {
		return Written{}, err
	}
	if loc := res.Header.Get("Location"); loc != "" {
		w.ID = loc[strings.LastIndexByte(loc, '/')+1:]
	}
	return w, nil
}

// Cycle is the issue's cycle, as the server computes it, with the issue's
// version. ErrOff when the cycle is off on the board; an error when no
// server runs.
func (c *Client) Cycle(ctx context.Context, id string) (*Cycle, error) {
	var cyc Cycle
	if err := c.get(ctx, "/api/issues/"+url.PathEscape(id)+"/cycle", &cyc); err != nil {
		return nil, err
	}
	return &cyc, nil
}

// ReviewThread is the issue's thread for answering a line comment: the
// server's (GET /api/issues/{id}, with its version), else show --json's.
// Neither locates a line comment on the branch's head: v1's server filled
// Review's Branch, Outdated, HeadLine and Text, v2's doesn't yet, so they
// stay empty and the comment says where it was written only
// (Review.Located).
func (c *Client) ReviewThread(ctx context.Context, id string) (Thread, error) {
	if c.URL == "" {
		return c.Thread(ctx, id)
	}
	var th Thread
	err := c.get(ctx, "/api/issues/"+url.PathEscape(id), &th)
	return th, err
}

// Located tells whether the server located the comment on the issue's
// branch: false, it says where it was written only.
func (r Review) Located() bool {
	return r.Branch != "" || r.Outdated || r.HeadLine > 0 || r.Text != ""
}

// WorkAgent is a branch the Work page shows an issue's step for: the
// issue whose evidence branch it is, its step, and who it waits for.
// Path is "": v2's Work page knows branches, not worktrees, and the
// caller matches a worktree by its branch.
type WorkAgent struct {
	Path   string  `json:"path,omitempty"`
	Branch string  `json:"branch"`
	Issue  *Thread `json:"issue,omitempty"`
	Step   int     `json:"step,omitempty"`
	Name   string  `json:"name,omitempty"`
	State  string  `json:"state"`
}

// workRow is a row of GET /api/cycle (next/domain/cycle/work.Row): the
// fields read here.
type workRow struct {
	Issue  Thread `json:"issue"`
	Step   int    `json:"step"`
	Name   string `json:"name"`
	Done   bool   `json:"done"`
	Mine   bool   `json:"mine"`
	Branch string `json:"branch"`
	Waits  struct {
		Login  string `json:"login"`
		What   string `json:"what"`
		Anyone bool   `json:"anyone"`
	} `json:"waits"`
}

func (r workRow) state() string {
	switch {
	case r.Done:
		return "Done"
	case r.Mine:
		return "Waits for you"
	case r.Waits.Anyone:
		return "Waits for anyone"
	case r.Waits.Login != "":
		return "Waits for @" + r.Waits.Login
	}
	return r.Waits.What
}

// Agents are the Work page's rows that name a branch (GET
// /api/cycle?who=team, every open issue, unpaged): the cycle must be on,
// and a server running.
func (c *Client) Agents(ctx context.Context) ([]WorkAgent, error) {
	var res struct {
		Needs   []workRow `json:"needs"`
		Waiting []workRow `json:"waiting"`
	}
	if err := c.get(ctx, "/api/cycle?who=team", &res); err != nil {
		return nil, err
	}
	var out []WorkAgent
	for _, r := range append(res.Needs, res.Waiting...) {
		if r.Branch == "" {
			continue
		}
		issue := r.Issue
		out = append(out, WorkAgent{Branch: r.Branch, Issue: &issue, Step: r.Step, Name: r.Name, State: r.state()})
	}
	return out, nil
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	_, err := c.do(ctx, http.MethodGet, path, "", nil, out)
	return err
}

// APIError is the API's error shape (api.md, One error shape), with the
// request's status.
type APIError struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
}

func (e *APIError) Error() string {
	s := e.Code
	if e.Message != "" {
		s += ": " + e.Message
	}
	if e.Field != "" {
		s += " (" + e.Field + ")"
	}
	return s
}

// do is one API request: JSON in, JSON out, ifMatch as If-Match when not
// "", the server's error shape read back: 404 experimental is ErrOff,
// another 404 ErrNotFound, 412 ErrStale, each wrapping the *APIError.
func (c *Client) do(ctx context.Context, method, path, ifMatch string, in, out any) (*http.Response, error) {
	if c.URL == "" {
		return nil, errors.New("no board server: run git board in the repository, or pass --board")
	}
	var body io.Reader
	if in != nil {
		data, err := json.Marshal(in)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.URL+path, body)
	if err != nil {
		return nil, err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json") // the server answers 415 without it
	}
	if ifMatch != "" {
		req.Header.Set("If-Match", `"`+strings.Trim(ifMatch, `"`)+`"`)
	}
	req.Header.Set("X-Git-Board-Client", "git-board-agents")
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 2 * time.Minute}
	}
	res, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode/100 != 2 {
		e := &APIError{Status: res.StatusCode}
		if json.Unmarshal(raw, e) != nil || e.Code == "" {
			e.Code, e.Message = "http", fmt.Sprintf("%s: %s", res.Status, strings.TrimSpace(string(raw[:min(len(raw), 300)])))
		}
		switch {
		case res.StatusCode == http.StatusNotFound && e.Code == "experimental":
			return nil, fmt.Errorf("%w: %w", ErrOff, e)
		case res.StatusCode == http.StatusNotFound:
			return nil, fmt.Errorf("%w: %s %s: %w", ErrNotFound, method, path, e)
		case res.StatusCode == http.StatusPreconditionFailed:
			return nil, fmt.Errorf("%w: %w", ErrStale, e)
		}
		return nil, fmt.Errorf("%s %s: %w", method, path, e)
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return nil, fmt.Errorf("%s %s: %w", method, path, err)
		}
	}
	return res, nil
}

// Discover is the URL of the board's server for the repository at dir:
// GIT_BOARD_URL when set, else the address a running git-board server
// wrote in the main checkout's git directory, or in this worktree's
// (git-board/server.json, as git board finds it), when the server there
// answers GET /api/board for that git directory. "" when none runs.
func Discover(ctx context.Context, dir string) string {
	if u := os.Getenv("GIT_BOARD_URL"); u != "" {
		return strings.TrimRight(u, "/")
	}
	for _, flag := range []string{"--git-common-dir", "--absolute-git-dir"} {
		out, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--path-format=absolute", flag).Output()
		if err != nil {
			continue
		}
		gitDir := strings.TrimSpace(string(out))
		data, err := os.ReadFile(filepath.Join(gitDir, "git-board", "server.json"))
		if err != nil {
			continue
		}
		var inst struct {
			URL string `json:"url"`
		}
		if json.Unmarshal(data, &inst) != nil || inst.URL == "" {
			continue
		}
		if answers(ctx, inst.URL, gitDir) {
			return strings.TrimRight(inst.URL, "/")
		}
	}
	return ""
}

// answers tells whether a git-board server answers at u for the
// repository at gitDir: a file left by a server that died names nothing,
// nor one whose port another board took since.
func answers(ctx context.Context, u, gitDir string) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	c := &Client{URL: strings.TrimRight(u, "/"), HTTP: http.DefaultClient}
	var b struct {
		GitDir string `json:"git_dir"`
	}
	if err := c.get(ctx, "/api/board", &b); err != nil {
		return false
	}
	return sameDir(b.GitDir, gitDir)
}

func sameDir(a, b string) bool {
	a, errA := filepath.EvalSymlinks(a)
	b, errB := filepath.EvalSymlinks(b)
	return errA == nil && errB == nil && a == b
}
