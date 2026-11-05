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
//   - the HTTP API, when a server runs: the issue's cycle (GET
//     /api/issues/{id}/cycle), its thread with the review comments
//     located on the branch (GET /api/issues/{id}/thread), the Work page's
//     agents (GET /api/work), a comment in a cycle step (POST
//     /api/issues/{id}/comments).
//
// The routes are git-board v1's (v0.11). v2 reshapes the API (ADR 0021):
// they are all in this file, so a v2 client changes here alone.
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

// Review is a line comment's anchor; the server fills Branch, Outdated,
// HeadLine and Text from the issue's branches.
type Review struct {
	Path     string `json:"path,omitempty"`
	Line     int    `json:"line,omitempty"`
	Side     string `json:"side,omitempty"`
	Commit   string `json:"commit,omitempty"`
	ReplyTo  string `json:"reply_to,omitempty"`
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

// Comment writes a comment on the issue. With a server it goes through
// the API, in the cycle's step when step isn't "" (the comment's
// cycle:step); without one, through git-board comment, in no step.
func (c *Client) Comment(ctx context.Context, id, body, step string) error {
	if c.URL == "" {
		_, err := c.cli(ctx, "comment", id, "-b", body)
		return err
	}
	req := map[string]any{"body": body}
	if step != "" {
		req["step"] = step
	}
	return c.send(ctx, http.MethodPost, "/api/issues/"+url.PathEscape(id)+"/comments", req, nil)
}

// Cycle is the issue's cycle, as the server computes it. ErrOff when the
// cycle is off on the board; an error when no server runs.
func (c *Client) Cycle(ctx context.Context, id string) (*Cycle, error) {
	var cyc Cycle
	if err := c.get(ctx, "/api/issues/"+url.PathEscape(id)+"/cycle", &cyc); err != nil {
		return nil, err
	}
	return &cyc, nil
}

// ReviewThread is the issue's thread with its line comments located on
// the issue's branch: the server's, else show --json's, whose line
// comments say where they were written only.
func (c *Client) ReviewThread(ctx context.Context, id string) (Thread, error) {
	if c.URL == "" {
		return c.Thread(ctx, id)
	}
	var th Thread
	err := c.get(ctx, "/api/issues/"+url.PathEscape(id)+"/thread", &th)
	return th, err
}

// WorkAgent is a linked worktree as the Work page lists it: the issue its
// branch names, the step, and what it waits for.
type WorkAgent struct {
	Path   string  `json:"path"`
	Branch string  `json:"branch"`
	Issue  *Thread `json:"issue,omitempty"`
	Step   int     `json:"step,omitempty"`
	Name   string  `json:"name,omitempty"`
	State  string  `json:"state"`
}

// Agents are the Work page's agents (GET /api/work?who=team): the cycle
// must be on, and a server running.
func (c *Client) Agents(ctx context.Context) ([]WorkAgent, error) {
	var res struct {
		Agents []WorkAgent `json:"agents"`
	}
	err := c.get(ctx, "/api/work?who=team", &res)
	return res.Agents, err
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	return c.send(ctx, http.MethodGet, path, nil, out)
}

// send is one API request: JSON in, JSON out, the server's error shape
// ({"error", "code"}) read back.
func (c *Client) send(ctx context.Context, method, path string, in, out any) error {
	if c.URL == "" {
		return errors.New("no board server: run git board in the repository, or pass --board")
	}
	var body io.Reader
	if in != nil {
		data, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.URL+path, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json") // the server refuses a write without it
	}
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 2 * time.Minute}
	}
	res, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 32<<20))
	if err != nil {
		return err
	}
	if res.StatusCode/100 != 2 {
		var e struct{ Error, Code string }
		_ = json.Unmarshal(raw, &e)
		switch {
		case res.StatusCode == http.StatusNotFound && e.Code == "experimental":
			return fmt.Errorf("%w: %s", ErrOff, e.Error)
		case res.StatusCode == http.StatusNotFound:
			return fmt.Errorf("%w: %s", ErrNotFound, cmpOr(e.Error, path))
		}
		return fmt.Errorf("%s %s: %s: %s", method, path, res.Status, cmpOr(e.Error, strings.TrimSpace(string(raw[:min(len(raw), 300)]))))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	return nil
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// Discover is the URL of the board's server for the repository at dir:
// GIT_BOARD_URL when set, else the address a running git-board server
// wrote in the main checkout's git directory (git-board/server.json, as
// git board and show --web find it), when the server there answers. ""
// when none runs.
func Discover(ctx context.Context, dir string) string {
	if u := os.Getenv("GIT_BOARD_URL"); u != "" {
		return strings.TrimRight(u, "/")
	}
	for _, flag := range []string{"--git-common-dir", "--absolute-git-dir"} {
		out, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--path-format=absolute", flag).Output()
		if err != nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join(strings.TrimSpace(string(out)), "git-board", "server.json"))
		if err != nil {
			continue
		}
		var inst struct {
			URL string `json:"url"`
		}
		if json.Unmarshal(data, &inst) != nil || inst.URL == "" {
			continue
		}
		if answers(ctx, inst.URL) {
			return strings.TrimRight(inst.URL, "/")
		}
	}
	return ""
}

// answers tells whether a git-board server answers at u: a file left by
// a server that died names nothing.
func answers(ctx context.Context, u string) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(u, "/")+"/api/repository", nil)
	if err != nil {
		return false
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	res.Body.Close()
	return res.StatusCode == http.StatusOK
}
