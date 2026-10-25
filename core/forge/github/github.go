// Package github is a GitHub project as a forge.Forge, on github.com or a
// GitHub Enterprise host. It speaks GitHub's REST API, plus one GraphQL
// query for the change times REST doesn't give.
//
// GitHub has no confidential issues. A pull request is never synced: the
// listing leaves it out, and reading one alone is forge.ErrConfidential, as
// sync never reads it.
package github

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/guilbep/git-board/core/forge"
)

const (
	perPage = 100
	// margin, in seconds, lists the issues updated a minute before since:
	// since is the local clock's, and updated_at GitHub's.
	margin = 60
	// perHour is how many writes GitHub takes in an hour.
	perHour = 500
	// retries is how many times a request is tried again.
	retries = 3
)

// Options name a GitHub project, and how to reach it.
type Options struct {
	// Project is owner/repo.
	Project string
	// Host is github.com, the default, or a GitHub Enterprise host.
	Host string
	// Login is the posting account's, which git's credential helper is
	// asked for. Empty asks for any account on the host.
	Login string
	// API is the REST API's base URL: by default https://api.github.com,
	// or https://<host>/api/v3 for an Enterprise host. GraphQL is at its
	// /graphql, or at /api/graphql beside /api/v3.
	API string
	// Token gives an account's token. By default it is the environment's
	// GITHUB_TOKEN, GITHUB_ENTERPRISE_TOKEN for GITHUB_ENTERPRISE_HOST, when
	// set; else what git's credential helper holds for the host and the
	// account's login.
	Token func(ctx context.Context, a forge.Account) (string, error)
}

// Forge is a GitHub project, through one account's token.
type Forge struct {
	s     *shared
	token string

	mu sync.Mutex
	me *forge.Account
}

var _ forge.Forge = (*Forge)(nil)

// shared is what every account's view of the project shares.
type shared struct {
	host, project, repo, graphql, api string
	tokenOf                           func(context.Context, forge.Account) (string, error)
	client                            *http.Client
	now                               func() time.Time
	sleep                             func(context.Context, time.Duration) error

	mu      sync.Mutex
	numbers map[string]int  // issue id -> number, pull requests' too
	scanned bool            // numbers holds every issue's
	labels  map[string]bool // the project's labels known to exist, in lower case
	writes  []time.Time     // the last hour's writes, for pacing
}

// New is the project through the posting account's token.
func New(ctx context.Context, o Options) (*Forge, error) {
	host := cmp.Or(o.Host, "github.com")
	owner, repo, ok := strings.Cut(o.Project, "/")
	if !ok || owner == "" || repo == "" || strings.Contains(repo, "/") {
		return nil, fmt.Errorf("github: project %q isn't owner/repo", o.Project)
	}
	if strings.ContainsAny(host, "/?#@ \r\n") {
		return nil, fmt.Errorf("github: bad host %q", host)
	}
	api := o.API
	if api == "" {
		api = "https://api.github.com"
		if host != "github.com" {
			api = "https://" + host + "/api/v3"
		}
	}
	api = strings.TrimSuffix(api, "/")
	if err := checkURL(api, host, "api."+host); err != nil {
		return nil, err
	}
	graphql := api + "/graphql"
	if base, ok := strings.CutSuffix(api, "/v3"); ok {
		graphql = base + "/graphql"
	}
	tokenOf := o.Token
	if tokenOf == nil {
		tokenOf = func(ctx context.Context, a forge.Account) (string, error) { return lookup(ctx, host, a.Login) }
	}
	token, err := tokenOf(ctx, forge.Account{Login: o.Login})
	if err != nil {
		return nil, err
	}
	if token == "" {
		return nil, forge.ErrNoToken
	}
	s := &shared{
		host: host, project: o.Project, api: api, graphql: graphql, tokenOf: tokenOf,
		repo: api + "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo),
		client: &http.Client{Timeout: time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse // a moved issue is gone; and Go would turn a PATCH into a GET
		}},
		now: time.Now, sleep: sleep, numbers: map[string]int{}, labels: map[string]bool{},
	}
	return &Forge{s: s, token: token}, nil
}

// Key is the host and owner/repo, like github.com/guilbep/git-board-test.
func (f *Forge) Key() string { return f.s.host + "/" + f.s.project }

func (f *Forge) Server() string { return f.s.host }

func (f *Forge) Me(ctx context.Context) (forge.Account, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.me == nil {
		var u user
		if err := f.call(ctx, http.MethodGet, f.s.api+"/user", nil, &u); err != nil {
			return forge.Account{}, err
		}
		a := u.account()
		f.me = &a
	}
	return *f.me, nil
}

// Public is the repository's private flag, negated.
func (f *Forge) Public(ctx context.Context) (bool, error) {
	var r struct {
		Private bool `json:"private"`
	}
	if err := f.call(ctx, http.MethodGet, f.s.repo, nil, &r); err != nil {
		return false, err
	}
	return !r.Private, nil
}

// As looks up the account's token, and takes it only when GitHub says it is
// that account's: a login can be renamed, and a helper can answer with
// another account's token.
func (f *Forge) As(ctx context.Context, a forge.Account) (forge.Forge, error) {
	me, err := f.Me(ctx)
	if err != nil {
		return nil, err
	}
	if a.ID == me.ID {
		return f, nil
	}
	token, err := f.s.tokenOf(ctx, a)
	if err != nil || token == "" || token == f.token {
		return nil, forge.ErrNoToken
	}
	g := &Forge{s: f.s, token: token}
	who, err := g.Me(ctx)
	switch {
	case errors.Is(err, errUnauthorized):
		return nil, forge.ErrNoToken
	case err != nil:
		return nil, err
	case who.ID != a.ID:
		return nil, forge.ErrNoToken
	}
	return g, nil
}

// Issues lists the issues updated from a minute before since, oldest first,
// without pull requests.
func (f *Forge) Issues(ctx context.Context, since int64) ([]forge.Issue, error) {
	q := "/issues?state=all&sort=created&direction=asc"
	if since > 0 {
		q += "&since=" + time.Unix(max(since-margin, 0), 0).UTC().Format(time.RFC3339)
	}
	gis, err := f.issues(ctx, q)
	if err != nil {
		return nil, err
	}
	return f.read(ctx, gis)
}

func (f *Forge) Issue(ctx context.Context, id string) (forge.Issue, error) {
	n, err := f.number(ctx, id)
	if err != nil {
		return forge.Issue{}, err
	}
	var gi ghIssue
	if err := f.call(ctx, http.MethodGet, f.s.repo+"/issues/"+n, nil, &gi); err != nil {
		return forge.Issue{}, err
	}
	if gi.PullRequest != nil {
		return forge.Issue{}, fmt.Errorf("github: #%s is a pull request: %w", n, forge.ErrConfidential)
	}
	out, err := f.read(ctx, []ghIssue{gi})
	if err != nil {
		return forge.Issue{}, err
	}
	return out[0], nil
}

func (f *Forge) CreateIssue(ctx context.Context, title, body string) (forge.Issue, error) {
	var gi ghIssue
	if err := f.call(ctx, http.MethodPost, f.s.repo+"/issues", map[string]string{"title": title, "body": body}, &gi); err != nil {
		return forge.Issue{}, err
	}
	f.s.mu.Lock()
	f.s.numbers[id(gi.ID)] = gi.Number
	f.s.mu.Unlock()
	return gi.issue(nil, ghTimes{}), nil
}

func (f *Forge) CreateComment(ctx context.Context, issueID, body string) (forge.Comment, error) {
	n, err := f.number(ctx, issueID)
	if err != nil {
		return forge.Comment{}, err
	}
	var c ghComment
	if err := f.call(ctx, http.MethodPost, f.s.repo+"/issues/"+n+"/comments", map[string]string{"body": body}, &c); err != nil {
		return forge.Comment{}, err
	}
	return c.comment(), nil
}

// EditComment needs only the comment's id: GitHub names comments alone.
func (f *Forge) EditComment(ctx context.Context, _, commentID, body string) error {
	return f.call(ctx, http.MethodPatch, f.s.repo+"/issues/comments/"+url.PathEscape(commentID), map[string]string{"body": body}, nil)
}

func (f *Forge) SetTitle(ctx context.Context, issueID, title string) error {
	return f.patch(ctx, issueID, map[string]any{"title": title}, nil)
}

func (f *Forge) SetBody(ctx context.Context, issueID, body string) error {
	return f.patch(ctx, issueID, map[string]any{"body": body}, nil)
}

func (f *Forge) SetStatus(ctx context.Context, issueID, status string) error {
	return f.patch(ctx, issueID, map[string]any{"state": status}, nil)
}

// ChangeLabels makes each added label the project lacks, adds them in one
// write, then removes the others one by one. An added label GitHub leaves
// out, as it does without push access, is refused.
func (f *Forge) ChangeLabels(ctx context.Context, issueID string, add, remove []string) error {
	n, err := f.number(ctx, issueID)
	if err != nil {
		return err
	}
	path := f.s.repo + "/issues/" + n + "/labels"
	if len(add) > 0 {
		for _, l := range add {
			if err := f.label(ctx, l); err != nil {
				return err
			}
		}
		var got []label
		if err := f.call(ctx, http.MethodPost, path, map[string]any{"labels": add}, &got); err != nil {
			return err
		}
		for _, l := range add {
			if !slices.ContainsFunc(got, func(g label) bool { return strings.EqualFold(g.Name, l) }) {
				return fmt.Errorf("github: #%s: label %q left out: %w", n, l, forge.ErrRefused)
			}
		}
	}
	for _, l := range remove {
		// not found: the issue doesn't have it, already removed
		if err := f.call(ctx, http.MethodDelete, path+"/"+url.PathEscape(l), nil, nil); err != nil && !errors.Is(err, forge.ErrNotFound) {
			return err
		}
	}
	return nil
}

// SetAssignees sends the logins, as GitHub takes, and reads back what it
// kept: it leaves out an account it won't assign, as one without access.
func (f *Forge) SetAssignees(ctx context.Context, issueID string, users []forge.Account) ([]forge.Account, error) {
	logins := []string{}
	for _, u := range users {
		logins = append(logins, u.Login)
	}
	var gi ghIssue
	if err := f.patch(ctx, issueID, map[string]any{"assignees": logins}, &gi); err != nil {
		return nil, err
	}
	var kept []forge.Account
	for _, u := range gi.Assignees {
		kept = append(kept, u.account())
	}
	return kept, nil
}

func (f *Forge) patch(ctx context.Context, issueID string, fields map[string]any, out any) error {
	n, err := f.number(ctx, issueID)
	if err != nil {
		return err
	}
	return f.call(ctx, http.MethodPatch, f.s.repo+"/issues/"+n, fields, out)
}

// label makes a label the project lacks, in GitHub's grey.
func (f *Forge) label(ctx context.Context, name string) error {
	key := strings.ToLower(name)
	f.s.mu.Lock()
	known := f.s.labels[key]
	f.s.mu.Unlock()
	if known {
		return nil
	}
	err := f.call(ctx, http.MethodGet, f.s.repo+"/labels/"+url.PathEscape(name), nil, nil)
	if errors.Is(err, forge.ErrNotFound) {
		err = f.call(ctx, http.MethodPost, f.s.repo+"/labels", map[string]string{"name": name, "color": "ededed"}, nil)
	}
	if errors.Is(err, forge.ErrRefused) {
		return nil // made meanwhile, or not ours to make: adding it tells
	}
	if err != nil {
		return err
	}
	f.s.mu.Lock()
	f.s.labels[key] = true
	f.s.mu.Unlock()
	return nil
}

// number is an issue's number, which REST names issues by. An id not seen
// yet is looked for once, in the listing of every issue.
func (f *Forge) number(ctx context.Context, id string) (string, error) {
	f.s.mu.Lock()
	n, ok := f.s.numbers[id]
	scanned := f.s.scanned
	f.s.mu.Unlock()
	if !ok && !scanned {
		// ponytail: one listing of every issue per adapter finds the ids of
		// earlier runs; forge.Forge passing the link's number would save it
		if _, err := f.issues(ctx, "/issues?state=all"); err != nil {
			return "", err
		}
		f.s.mu.Lock()
		f.s.scanned = true
		n, ok = f.s.numbers[id]
		f.s.mu.Unlock()
	}
	if !ok {
		return "", fmt.Errorf("github: issue %s: %w", id, forge.ErrNotFound)
	}
	return strconv.Itoa(n), nil
}

// issues reads every page of an issue listing, each issue once, and
// remembers their numbers. Pull requests are left out.
func (f *Forge) issues(ctx context.Context, query string) ([]ghIssue, error) {
	all, err := pages(ctx, f, f.s.repo+query, func(gi ghIssue) int64 { return gi.ID })
	if err != nil {
		return nil, err
	}
	f.s.mu.Lock()
	for _, gi := range all {
		f.s.numbers[id(gi.ID)] = gi.Number
	}
	f.s.mu.Unlock()
	return slices.DeleteFunc(all, func(gi ghIssue) bool { return gi.PullRequest != nil }), nil
}

// read completes issues with their comments and change times. An issue
// whose comments or times fail to load is an error, never half an issue.
func (f *Forge) read(ctx context.Context, gis []ghIssue) ([]forge.Issue, error) {
	times := map[string]ghTimes{}
	for chunk := range slices.Chunk(gis, perPage) {
		if err := f.times(ctx, chunk, times); err != nil {
			return nil, err
		}
	}
	out := make([]forge.Issue, 0, len(gis))
	for _, gi := range gis {
		var cs []ghComment
		if gi.Comments > 0 {
			var err error
			path := f.s.repo + "/issues/" + strconv.Itoa(gi.Number) + "/comments"
			if cs, err = pages(ctx, f, path, func(c ghComment) int64 { return c.ID }); err != nil {
				return nil, err
			}
		}
		t, ok := times[gi.NodeID]
		if !ok {
			return nil, fmt.Errorf("github: #%d: no change times", gi.Number)
		}
		out = append(out, gi.issue(cs, t))
	}
	return out, nil
}

// timesQuery reads what REST lacks: when the body was last edited, and the
// last rename and reopening.
const timesQuery = `query($ids: [ID!]!) {
  nodes(ids: $ids) {
    ... on Issue {
      id
      lastEditedAt
      renamed: timelineItems(itemTypes: [RENAMED_TITLE_EVENT], last: 1) { nodes { ... on RenamedTitleEvent { createdAt } } }
      reopened: timelineItems(itemTypes: [REOPENED_EVENT], last: 1) { nodes { ... on ReopenedEvent { createdAt } } }
    }
  }
}`

// times reads up to a hundred issues' change times, in one query.
func (f *Forge) times(ctx context.Context, gis []ghIssue, into map[string]ghTimes) error {
	ids := make([]string, len(gis))
	for i, gi := range gis {
		ids[i] = gi.NodeID
	}
	var r struct {
		Data struct {
			Nodes []*ghTimes `json:"nodes"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	in := map[string]any{"query": timesQuery, "variables": map[string]any{"ids": ids}}
	if err := f.call(ctx, http.MethodPost, f.s.graphql, in, &r); err != nil {
		return err
	}
	if len(r.Errors) > 0 {
		return fmt.Errorf("github: graphql: %s", r.Errors[0].Message)
	}
	for _, t := range r.Data.Nodes {
		if t != nil {
			into[t.ID] = *t
		}
	}
	return nil
}

// pages reads every page of a listing, each item once by its id: paging
// can list one twice, as when an item comes in between two pages.
func pages[T any](ctx context.Context, f *Forge, u string, id func(T) int64) ([]T, error) {
	sep := "?"
	if strings.Contains(u, "?") {
		sep = "&"
	}
	var out []T
	seen := map[int64]bool{}
	for page := 1; ; page++ {
		var batch []T
		if err := f.call(ctx, http.MethodGet, fmt.Sprintf("%s%sper_page=%d&page=%d", u, sep, perPage, page), nil, &batch); err != nil {
			return nil, err
		}
		for _, x := range batch {
			if !seen[id(x)] {
				seen[id(x)] = true
				out = append(out, x)
			}
		}
		if len(batch) < perPage {
			return out, nil
		}
	}
}

var errUnauthorized = errors.New("github: bad credentials")

// call makes one request, and reads its answer into out. A write is paced.
// A rate limit is waited out and the request tried again, three times at
// most. A read is also tried again after a 5xx or a broken connection; a
// write never is, as GitHub may have made it.
func (f *Forge) call(ctx context.Context, method, u string, in, out any) error {
	var body []byte
	if in != nil {
		var err error
		if body, err = json.Marshal(in); err != nil {
			return err
		}
	}
	read := method == http.MethodGet || u == f.s.graphql
	for try := 0; ; try++ {
		if !read {
			if err := f.s.pace(ctx); err != nil {
				return err
			}
		}
		code, h, data, err := f.do(ctx, method, u, body)
		var wait time.Duration
		switch {
		case err != nil:
			if !read || try == retries {
				return err
			}
			wait = time.Second << try
		case code < 300:
			if out == nil || len(data) == 0 {
				return nil
			}
			if err := json.Unmarshal(data, out); err != nil {
				return fmt.Errorf("github: %s %s: %w", method, u, err)
			}
			return nil
		default:
			var e struct {
				Message string `json:"message"`
			}
			_ = json.Unmarshal(data, &e)
			limit, limited := rateLimited(code, h, e.Message, f.s.now())
			switch {
			case limited && try < retries:
				wait = limit
			case code >= 500 && read && try < retries:
				wait = time.Second << try
			default:
				return statusError(method, u, code, e.Message, limited)
			}
		}
		if err := f.s.sleep(ctx, wait); err != nil {
			return err
		}
	}
}

func (f *Forge) do(ctx context.Context, method, u string, body []byte) (int, http.Header, []byte, error) {
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, r)
	if err != nil {
		return 0, nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+f.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "git-board")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := f.s.client.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, data, err
}

// rateLimited says whether GitHub turned a request away for a rate limit,
// and how long to wait: Retry-After when given, else until the reset when
// none is left, else a minute.
func rateLimited(code int, h http.Header, msg string, now time.Time) (time.Duration, bool) {
	if code != http.StatusForbidden && code != http.StatusTooManyRequests {
		return 0, false
	}
	if s, err := strconv.Atoi(h.Get("Retry-After")); err == nil {
		return time.Duration(s) * time.Second, true
	}
	if h.Get("X-RateLimit-Remaining") == "0" {
		if reset, err := strconv.ParseInt(h.Get("X-RateLimit-Reset"), 10, 64); err == nil {
			return max(time.Unix(reset, 0).Sub(now), 0) + time.Second, true
		}
	}
	if code == http.StatusTooManyRequests || strings.Contains(strings.ToLower(msg), "rate limit") {
		return time.Minute, true
	}
	return 0, false
}

// statusError maps GitHub's answer to forge's errors. A rate limit is never
// a refusal.
func statusError(method, u string, code int, msg string, limited bool) error {
	var why error
	switch {
	case limited:
	case code == http.StatusUnauthorized:
		why = errUnauthorized
	case code == http.StatusNotFound || code == http.StatusGone || code/100 == 3:
		why = forge.ErrNotFound
	case code == http.StatusForbidden || code == http.StatusUnprocessableEntity:
		why = forge.ErrRefused
	}
	err := fmt.Sprintf("github: %s %s: %d %s", method, u, code, msg)
	if why == nil {
		return errors.New(err)
	}
	return fmt.Errorf("%s: %w", err, why)
}

// pace waits for a write's turn: a second after the last write, as GitHub
// asks, which keeps under its 80 a minute, and under 500 an hour.
// ponytail: one pace per New, shared by its As views; one per token if
// several projects on one host sync at once.
func (s *shared) pace(ctx context.Context) error {
	s.mu.Lock()
	now := s.now()
	s.writes = slices.DeleteFunc(s.writes, func(t time.Time) bool { return now.Sub(t) >= time.Hour })
	at := now
	if n := len(s.writes); n > 0 {
		at = later(at, s.writes[n-1].Add(time.Second))
		if n >= perHour {
			at = later(at, s.writes[n-perHour].Add(time.Hour))
		}
	}
	s.writes = append(s.writes, at)
	s.mu.Unlock()
	return s.sleep(ctx, at.Sub(now))
}

func later(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
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

// What GitHub answers.

type user struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
}

func (u user) account() forge.Account { return forge.Account{ID: id(u.ID), Login: u.Login} }

type label struct {
	Name string `json:"name"`
}

type ghIssue struct {
	ID          int64      `json:"id"`
	NodeID      string     `json:"node_id"`
	Number      int        `json:"number"`
	URL         string     `json:"html_url"`
	Title       string     `json:"title"`
	Body        string     `json:"body"`
	State       string     `json:"state"`
	User        user       `json:"user"`
	Labels      []label    `json:"labels"`
	Assignees   []user     `json:"assignees"`
	Comments    int        `json:"comments"`
	Created     time.Time  `json:"created_at"`
	Updated     time.Time  `json:"updated_at"`
	Closed      *time.Time `json:"closed_at"`
	PullRequest *struct{}  `json:"pull_request"`
}

type ghComment struct {
	ID      int64     `json:"id"`
	Body    string    `json:"body"`
	User    user      `json:"user"`
	Created time.Time `json:"created_at"`
	Updated time.Time `json:"updated_at"`
}

type ghTimes struct {
	ID           string     `json:"id"`
	LastEditedAt *time.Time `json:"lastEditedAt"`
	Renamed      events     `json:"renamed"`
	Reopened     events     `json:"reopened"`
}

type events struct {
	Nodes []struct {
		CreatedAt time.Time `json:"createdAt"`
	} `json:"nodes"`
}

// issue is the forge's issue. Its status is GitHub's state, whatever its
// state_reason: one closed as not planned is closed. Its title, body and
// status changed when GitHub last renamed it, edited its body, and closed
// or reopened it, else at its creation.
func (gi ghIssue) issue(cs []ghComment, t ghTimes) forge.Issue {
	created := gi.Created.Unix()
	is := forge.Issue{
		ID: id(gi.ID), Number: strconv.Itoa(gi.Number), URL: gi.URL, Author: gi.User.account(),
		Created: created, Updated: gi.Updated.Unix(), Title: lf(gi.Title), Body: lf(gi.Body), Status: gi.State,
		Changed: map[string]int64{"title": created, "body": created, "status": created},
	}
	for _, l := range gi.Labels {
		is.Labels = append(is.Labels, l.Name)
	}
	for _, u := range gi.Assignees {
		is.Assignees = append(is.Assignees, u.account())
	}
	for _, c := range cs {
		is.Comments = append(is.Comments, c.comment())
	}
	if t.LastEditedAt != nil {
		is.Changed["body"] = t.LastEditedAt.Unix()
	}
	if n := len(t.Renamed.Nodes); n > 0 {
		is.Changed["title"] = t.Renamed.Nodes[n-1].CreatedAt.Unix()
	}
	switch n := len(t.Reopened.Nodes); {
	case gi.State == "closed" && gi.Closed != nil:
		is.Changed["status"] = gi.Closed.Unix()
	case gi.State == "open" && n > 0:
		is.Changed["status"] = t.Reopened.Nodes[n-1].CreatedAt.Unix()
	}
	return is
}

// comment is the forge's comment. GitHub moves updated_at when its text is
// edited.
func (c ghComment) comment() forge.Comment {
	out := forge.Comment{ID: id(c.ID), Body: lf(c.Body), Author: c.User.account(), Time: c.Created.Unix()}
	if c.Updated.After(c.Created) {
		out.Edited = c.Updated.Unix()
	}
	return out
}

func id(n int64) string { return strconv.FormatInt(n, 10) }

// lf undoes a web edit's \r\n, which would break the blank lines around
// sync's header and marker.
func lf(s string) string { return strings.ReplaceAll(s, "\r\n", "\n") }
