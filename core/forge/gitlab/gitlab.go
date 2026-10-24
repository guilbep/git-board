// Package gitlab is the forge adapter for one GitLab project, over GitLab's
// REST API v4. spec/docs/forges.md says what it does.
package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/guilbep/git-board/core/forge"
)

const (
	tokenEnv = "GITLAB_TOKEN"
	margin   = 2 * 60 // seconds: GitLab moves updated_at late
	retries  = 3
	pace     = 250 * time.Millisecond // under GitLab's 300 issues or notes created per minute
)

// project is what every account's view of the project shares.
type project struct {
	host, path string // gitlab.epfl.ch, and the project's path or numeric id
	api        string // https://gitlab.epfl.ch/api/v4/projects/<path, escaped>
	root       string // https://gitlab.epfl.ch/api/v4
	scheme     string
	http       *http.Client
	cred       func(ctx context.Context, a forge.Account) (string, error)
	pace       time.Duration
	sleep      func(ctx context.Context, d time.Duration) error

	mu      sync.Mutex
	iids    map[string]int64 // issue id → iid, as GitLab's API takes an iid
	scanned bool
}

// Forge is a GitLab project, through one account's token.
type Forge struct {
	p     *project
	token string

	mu        sync.Mutex
	me        *forge.Account
	lastWrite time.Time
	notBefore time.Time // RateLimit-Reset, once RateLimit-Remaining is 0
}

var _ forge.Forge = (*Forge)(nil)

// New is the project on the GitLab at baseURL, like https://gitlab.epfl.ch,
// named by its path, like guilbep/git-board-test, or its numeric id. Its
// token is the posting account's: GITLAB_TOKEN when it is set, else git's
// credential helper's for the host, with login as the username.
func New(ctx context.Context, baseURL, project, login string) (*Forge, error) {
	p, err := newProject(baseURL, project)
	if err != nil {
		return nil, err
	}
	token := os.Getenv(tokenEnv)
	if token == "" {
		if token, err = p.cred(ctx, forge.Account{Login: login}); err != nil {
			return nil, err
		}
	}
	return &Forge{p: p, token: token}, nil
}

func newProject(baseURL, path string) (*project, error) {
	u, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return nil, fmt.Errorf("gitlab: bad base URL %q", baseURL)
	}
	path = strings.Trim(path, "/")
	if path == "" {
		return nil, errors.New("gitlab: no project")
	}
	root := u.String() + "/api/v4"
	p := &project{host: u.Host, path: path, scheme: u.Scheme, root: root, api: root + "/projects/" + url.PathEscape(path),
		http: &http.Client{Timeout: time.Minute}, pace: pace, sleep: sleep, iids: map[string]int64{}}
	p.cred = func(ctx context.Context, a forge.Account) (string, error) {
		return credential(ctx, p.scheme, p.host, a.Login)
	}
	return p, nil
}

// credential asks git's credential helper for the host's token, without
// ever prompting.
func credential(ctx context.Context, scheme, host, login string) (string, error) {
	if strings.ContainsAny(login, "\n\x00") {
		return "", forge.ErrNoToken
	}
	in := "protocol=" + scheme + "\nhost=" + host + "\n"
	if login != "" {
		in += "username=" + login + "\n"
	}
	cmd := exec.CommandContext(ctx, "git", "credential", "fill")
	cmd.Stdin = strings.NewReader(in + "\n")
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=", "GCM_INTERACTIVE=never")
	out, err := cmd.Output()
	if err != nil {
		return "", forge.ErrNoToken
	}
	for line := range strings.SplitSeq(string(out), "\n") {
		if v, ok := strings.CutPrefix(line, "password="); ok && v != "" {
			return v, nil
		}
	}
	return "", forge.ErrNoToken
}

func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// String is the key, so that printing a Forge never shows its token.
func (f *Forge) String() string { return f.Key() }

func (f *Forge) Key() string    { return f.p.host + "/" + f.p.path }
func (f *Forge) Server() string { return f.p.host }

func (f *Forge) Me(ctx context.Context) (forge.Account, error) {
	f.mu.Lock()
	me := f.me
	f.mu.Unlock()
	if me != nil {
		return *me, nil
	}
	var u user
	if _, err := f.call(ctx, http.MethodGet, f.p.root+"/user", nil, nil, &u); err != nil {
		return forge.Account{}, err
	}
	a := u.account()
	f.mu.Lock()
	f.me = &a
	f.mu.Unlock()
	return a, nil
}

// Public is true for a public project only: internal is seen by every
// user of the instance, not by anyone.
func (f *Forge) Public(ctx context.Context) (bool, error) {
	var p struct {
		Visibility string `json:"visibility"`
	}
	if _, err := f.call(ctx, http.MethodGet, f.p.api, nil, nil, &p); err != nil {
		return false, err
	}
	return p.Visibility == "public", nil
}

// As takes the account's token from git's credential helper, and checks
// that GitLab says it is theirs.
func (f *Forge) As(ctx context.Context, a forge.Account) (forge.Forge, error) {
	me, err := f.Me(ctx)
	if err != nil {
		return nil, err
	}
	if a.ID == me.ID {
		return f, nil
	}
	token, err := f.p.cred(ctx, a)
	if err != nil {
		return nil, forge.ErrNoToken
	}
	g := &Forge{p: f.p, token: token}
	if who, err := g.Me(ctx); err != nil || who.ID != a.ID {
		return nil, forge.ErrNoToken // ponytail: a token that fails, or is someone else's, counts as none
	}
	return g, nil
}

// What GitLab answers.

type user struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

func (u user) account() forge.Account {
	return forge.Account{ID: strconv.FormatInt(u.ID, 10), Login: u.Username}
}

type glIssue struct {
	ID           int64     `json:"id"`
	IID          int64     `json:"iid"`
	Title        string    `json:"title"`
	Description  string    `json:"description"`
	State        string    `json:"state"`
	Labels       []string  `json:"labels"`
	Assignees    []user    `json:"assignees"`
	Author       user      `json:"author"`
	Confidential bool      `json:"confidential"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	WebURL       string    `json:"web_url"`
}

func (g glIssue) key() int64 { return g.ID }

// issue is the issue without its comments and change times.
func (g glIssue) issue() forge.Issue {
	status := g.State
	if status == "opened" {
		status = "open"
	}
	is := forge.Issue{ID: strconv.FormatInt(g.ID, 10), Number: strconv.FormatInt(g.IID, 10), URL: g.WebURL,
		Author: g.Author.account(), Created: g.CreatedAt.Unix(), Updated: g.UpdatedAt.Unix(),
		Title: text(g.Title), Body: text(g.Description), Status: status, Labels: g.Labels}
	for _, u := range g.Assignees {
		is.Assignees = append(is.Assignees, u.account())
	}
	return is
}

type note struct {
	ID       int64  `json:"id"`
	Body     string `json:"body"`
	Author   user   `json:"author"`
	System   bool   `json:"system"`
	Internal bool   `json:"internal"`
	// Confidential is Internal's name before GitLab 15.0.
	Confidential bool      `json:"confidential"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (n note) key() int64 { return n.ID }

func (n note) comment() forge.Comment {
	c := forge.Comment{ID: strconv.FormatInt(n.ID, 10), Body: text(n.Body), Author: n.Author.account(), Time: n.CreatedAt.Unix()}
	if u := n.UpdatedAt.Unix(); u > c.Time {
		c.Edited = u
	}
	return c
}

// changes is the field a system note says changed, or "".
func (n note) changes() string {
	switch b := n.Body; {
	case strings.HasPrefix(b, "changed title from "):
		return "title"
	case strings.HasPrefix(b, "changed the description"):
		return "body"
	case b == "closed" || b == "reopened" || strings.HasPrefix(b, "closed via "):
		return "status" // GitLab before 13.2, without state events
	}
	return ""
}

type stateEvent struct {
	ID        int64     `json:"id"`
	CreatedAt time.Time `json:"created_at"`
}

func (e stateEvent) key() int64 { return e.ID }

// text is GitLab's text with its line endings as git-board's: a web edit
// can send \r\n.
func text(s string) string { return strings.ReplaceAll(s, "\r\n", "\n") }

// Reads.

func (p *project) issue(iid int64) string { return p.api + "/issues/" + strconv.FormatInt(iid, 10) }

func (p *project) remember(g glIssue) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.iids[strconv.FormatInt(g.ID, 10)] = g.IID
}

// Issues lists from 2 minutes before since, oldest first, without the
// confidential ones, which GitLab lists to its members.
func (f *Forge) Issues(ctx context.Context, since int64) ([]forge.Issue, error) {
	q := url.Values{"scope": {"all"}, "order_by": {"created_at"}, "sort": {"asc"}}
	if since > 0 {
		q.Set("updated_after", time.Unix(since-margin, 0).UTC().Format(time.RFC3339))
	}
	var out []forge.Issue
	err := each(ctx, f, f.p.api+"/issues", q, func(g glIssue) (bool, error) {
		f.p.remember(g)
		if g.Confidential {
			return true, nil
		}
		is, err := f.full(ctx, g)
		out = append(out, is)
		return err == nil, err
	})
	return out, err
}

func (f *Forge) Issue(ctx context.Context, id string) (forge.Issue, error) {
	g, err := f.read(ctx, id)
	if err != nil {
		return forge.Issue{}, err
	}
	return f.full(ctx, g)
}

// read reads the issue alone: ErrNotFound, or ErrConfidential.
func (f *Forge) read(ctx context.Context, id string) (glIssue, error) {
	var g glIssue
	iid, err := f.iid(ctx, id)
	if err != nil {
		return g, err
	}
	if _, err := f.call(ctx, http.MethodGet, f.p.issue(iid), nil, nil, &g); err != nil {
		return g, err
	}
	f.p.remember(g)
	if g.Confidential {
		return g, fmt.Errorf("gitlab: issue %s: %w", id, forge.ErrConfidential)
	}
	return g, nil
}

// iid is the issue's iid. GitLab's REST API reads an issue by its iid only,
// so an id not listed yet is looked for in the whole listing.
// ponytail: one full listing per adapter on a miss; the link's number, if the
// interface carried it, when projects get big.
func (f *Forge) iid(ctx context.Context, id string) (int64, error) {
	lookup := func() (int64, bool, bool) {
		f.p.mu.Lock()
		defer f.p.mu.Unlock()
		n, ok := f.p.iids[id]
		return n, ok, f.p.scanned
	}
	if n, ok, scanned := lookup(); ok {
		return n, nil
	} else if !scanned {
		done := true
		err := each(ctx, f, f.p.api+"/issues", url.Values{"scope": {"all"}, "order_by": {"created_at"}, "sort": {"asc"}},
			func(g glIssue) (bool, error) {
				f.p.remember(g)
				done = strconv.FormatInt(g.ID, 10) != id
				return done, nil
			})
		if err != nil {
			return 0, err
		}
		if done {
			f.p.mu.Lock()
			f.p.scanned = true
			f.p.mu.Unlock()
		}
		if n, ok, _ := lookup(); ok {
			return n, nil
		}
	}
	return 0, fmt.Errorf("gitlab: issue %s: %w", id, forge.ErrNotFound)
}

// full adds the issue's comments, without system and internal notes, and
// when its title, body and status last changed.
func (f *Forge) full(ctx context.Context, g glIssue) (forge.Issue, error) {
	is := g.issue()
	is.Changed = map[string]int64{}
	at := f.p.issue(g.IID)
	err := each(ctx, f, at+"/notes", url.Values{"sort": {"asc"}, "order_by": {"created_at"}}, func(n note) (bool, error) {
		switch {
		case n.System:
			if field := n.changes(); field != "" {
				is.Changed[field] = max(is.Changed[field], n.CreatedAt.Unix())
			}
		case !n.Internal && !n.Confidential: // an internal note is never synced
			is.Comments = append(is.Comments, n.comment())
		}
		return true, nil
	})
	if err != nil {
		return is, err
	}
	err = each(ctx, f, at+"/resource_state_events", url.Values{}, func(e stateEvent) (bool, error) {
		is.Changed["status"] = max(is.Changed["status"], e.CreatedAt.Unix())
		return true, nil
	})
	if errors.Is(err, forge.ErrNotFound) {
		err = nil // GitLab before 13.2: the system notes said it
	}
	return is, err
}

// each reads every page of a listing, in order, and gives each item once:
// a page read after a change can repeat the last one's items. do returns
// false to stop.
// ponytail: offset paging can skip an item deleted between two pages.
func each[T interface{ key() int64 }](ctx context.Context, f *Forge, at string, q url.Values, do func(T) (bool, error)) error {
	q = maps.Clone(q)
	if q == nil {
		q = url.Values{}
	}
	q.Set("per_page", "100")
	seen := map[int64]bool{}
	for page := 1; page > 0; {
		q.Set("page", strconv.Itoa(page))
		var items []T
		h, err := f.call(ctx, http.MethodGet, at, q, nil, &items)
		if err != nil {
			return err
		}
		for _, it := range items {
			if seen[it.key()] {
				continue
			}
			seen[it.key()] = true
			if more, err := do(it); err != nil || !more {
				return err
			}
		}
		next, _ := strconv.Atoi(h.Get("X-Next-Page"))
		if next <= page {
			next = 0
		}
		page = next
	}
	return nil
}

// Writes. Each reads the issue first, as GitLab takes writes to an issue
// that turned confidential.

func (f *Forge) CreateIssue(ctx context.Context, title, body string) (forge.Issue, error) {
	var g glIssue
	if _, err := f.call(ctx, http.MethodPost, f.p.api+"/issues", nil, map[string]string{"title": title, "description": body}, &g); err != nil {
		return forge.Issue{}, err
	}
	f.p.remember(g)
	return g.issue(), nil
}

func (f *Forge) CreateComment(ctx context.Context, issueID, body string) (forge.Comment, error) {
	g, err := f.read(ctx, issueID)
	if err != nil {
		return forge.Comment{}, err
	}
	var n note
	if _, err := f.call(ctx, http.MethodPost, f.p.issue(g.IID)+"/notes", nil, map[string]string{"body": body}, &n); err != nil {
		return forge.Comment{}, err
	}
	return n.comment(), nil
}

func (f *Forge) EditComment(ctx context.Context, issueID, commentID, body string) error {
	g, err := f.read(ctx, issueID)
	if err != nil {
		return err
	}
	if _, err := strconv.ParseInt(commentID, 10, 64); err != nil {
		return fmt.Errorf("gitlab: comment %s: %w", commentID, forge.ErrNotFound)
	}
	_, err = f.call(ctx, http.MethodPut, f.p.issue(g.IID)+"/notes/"+commentID, nil, map[string]string{"body": body}, nil)
	return err
}

func (f *Forge) SetTitle(ctx context.Context, issueID, title string) error {
	return f.update(ctx, issueID, map[string]any{"title": title}, nil)
}

func (f *Forge) SetBody(ctx context.Context, issueID, body string) error {
	return f.update(ctx, issueID, map[string]any{"description": body}, nil)
}

func (f *Forge) SetStatus(ctx context.Context, issueID, status string) error {
	event := map[string]string{"open": "reopen", "closed": "close"}[status]
	if event == "" {
		return fmt.Errorf("gitlab: bad status %q", status)
	}
	return f.update(ctx, issueID, map[string]any{"state_event": event}, nil)
}

// ChangeLabels sends the labels by name. GitLab splits them at commas, and
// its labels can't hold one, so a label with a comma is refused.
func (f *Forge) ChangeLabels(ctx context.Context, issueID string, add, remove []string) error {
	fields := map[string]any{}
	for name, l := range map[string][]string{"add_labels": add, "remove_labels": remove} {
		for _, x := range l {
			if strings.Contains(x, ",") {
				return fmt.Errorf("gitlab: label %q: %w", x, forge.ErrRefused)
			}
		}
		if len(l) > 0 {
			fields[name] = strings.Join(l, ",")
		}
	}
	if len(fields) == 0 {
		return nil
	}
	return f.update(ctx, issueID, fields, nil)
}

// SetAssignees sends the whole set, and returns the assignees GitLab kept:
// its free edition keeps the first.
func (f *Forge) SetAssignees(ctx context.Context, issueID string, users []forge.Account) ([]forge.Account, error) {
	ids := []int64{}
	for _, u := range users {
		id, err := strconv.ParseInt(u.ID, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("gitlab: user id %q: %w", u.ID, forge.ErrRefused)
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		ids = []int64{0} // GitLab's "no assignee"
	}
	var g glIssue
	if err := f.update(ctx, issueID, map[string]any{"assignee_ids": ids}, &g); err != nil {
		return nil, err
	}
	return g.issue().Assignees, nil
}

// update writes the fields, and decodes the issue GitLab answers into out,
// when it isn't nil.
func (f *Forge) update(ctx context.Context, issueID string, fields map[string]any, out any) error {
	g, err := f.read(ctx, issueID)
	if err != nil {
		return err
	}
	_, err = f.call(ctx, http.MethodPut, f.p.issue(g.IID), nil, fields, out)
	return err
}

// The requests.

// call sends one request with the token, and decodes GitLab's answer into
// out. It paces writes, waits out a spent rate limit, and retries a 429,
// and a 5xx for a read. A write is never retried after a 5xx: GitLab may
// have made it.
func (f *Forge) call(ctx context.Context, method, at string, q url.Values, in, out any) (http.Header, error) {
	var body []byte
	if in != nil {
		var err error
		if body, err = json.Marshal(in); err != nil {
			return nil, err
		}
	}
	if len(q) > 0 {
		at += "?" + q.Encode()
	}
	what := method + " " + strings.TrimPrefix(at, f.p.root)
	write := method != http.MethodGet
	for try := 0; ; try++ {
		if err := f.wait(ctx, write); err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, method, at, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("PRIVATE-TOKEN", f.token)
		if in != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := f.p.http.Do(req)
		if write {
			f.mu.Lock()
			f.lastWrite = time.Now()
			f.mu.Unlock()
		}
		if err != nil {
			return nil, fmt.Errorf("gitlab: %s: %w", what, err)
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("gitlab: %s: %w", what, err)
		}
		f.limits(resp.Header)
		code := resp.StatusCode
		if code/100 == 2 {
			if out != nil && len(data) > 0 {
				if err := json.Unmarshal(data, out); err != nil {
					return nil, fmt.Errorf("gitlab: %s: %w", what, err)
				}
			}
			return resp.Header, nil
		}
		if (code == http.StatusTooManyRequests || (code >= 500 && method == http.MethodGet)) && try < retries {
			if err := f.p.sleep(ctx, backoff(resp.Header, try)); err != nil {
				return nil, err
			}
			continue
		}
		return nil, refusal(what, code, data)
	}
}

// wait waits out a spent rate limit, and paces a write after the last one.
func (f *Forge) wait(ctx context.Context, write bool) error {
	f.mu.Lock()
	at := f.notBefore
	f.notBefore = time.Time{}
	if next := f.lastWrite.Add(f.p.pace); write && next.After(at) {
		at = next
	}
	f.mu.Unlock()
	return f.p.sleep(ctx, time.Until(at))
}

// limits notes when GitLab's rate limit is spent, until its reset.
func (f *Forge) limits(h http.Header) {
	if h.Get("RateLimit-Remaining") != "0" {
		return
	}
	if reset, err := strconv.ParseInt(h.Get("RateLimit-Reset"), 10, 64); err == nil {
		f.mu.Lock()
		f.notBefore = time.Unix(reset, 0)
		f.mu.Unlock()
	}
}

// backoff is how long to wait before trying again: Retry-After, else the
// rate limit's reset, else 1, 2, then 4 seconds.
func backoff(h http.Header, try int) time.Duration {
	if s := h.Get("Retry-After"); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			return time.Duration(n) * time.Second
		}
		if t, err := http.ParseTime(s); err == nil {
			return time.Until(t)
		}
	}
	if reset, err := strconv.ParseInt(h.Get("RateLimit-Reset"), 10, 64); err == nil {
		if d := time.Until(time.Unix(reset, 0)); d > 0 {
			return d
		}
	}
	return time.Second << try
}

// refusal maps GitLab's answer to the forge's errors.
func refusal(what string, code int, data []byte) error {
	msg := strings.TrimSpace(string(data))
	if len(msg) > 200 {
		msg = msg[:200]
	}
	err := fmt.Errorf("gitlab: %s: %d %s", what, code, msg)
	switch code {
	case http.StatusNotFound, http.StatusGone:
		return fmt.Errorf("%w: %w", err, forge.ErrNotFound)
	case http.StatusBadRequest, http.StatusForbidden, http.StatusConflict, http.StatusUnprocessableEntity:
		return fmt.Errorf("%w: %w", err, forge.ErrRefused)
	}
	return err
}
