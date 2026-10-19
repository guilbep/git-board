// Package ghpr is the gate into a GitHub base branch: the pull request
// whose head is a branch, found by its name (never stored), its checks,
// reviews and merge state, and opening and merging it. Plain HTTP with
// one person's token: git-board acts on GitHub as whoever uses it.
package ghpr

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"
)

// Client is GitHub's API as one person: their token.
type Client struct {
	API   string // "" is https://api.github.com; tests: an httptest server
	Token string
	HTTP  *http.Client // nil: a client with a 30 s timeout
}

var defaultHTTP = &http.Client{Timeout: 30 * time.Second}

// Check is one of a PR's checks: a check run or a commit status.
type Check struct {
	Name     string `json:"name"`
	State    string `json:"state"` // success, failure, pending, expected (required, never reported)
	Required bool   `json:"required"`
	URL      string `json:"url,omitempty"`
}

// Review is a person's latest approval or request for changes, or a
// review GitHub dismissed.
type Review struct {
	Login  string `json:"login"`
	State  string `json:"state"`  // APPROVED, CHANGES_REQUESTED, DISMISSED
	Commit string `json:"commit"` // the commit it was made at
}

// PR is a pull request as the gate shows it.
type PR struct {
	Number      int      `json:"number"`
	URL         string   `json:"url"`
	State       string   `json:"state"` // OPEN, MERGED, CLOSED
	Draft       bool     `json:"draft"`
	Title       string   `json:"title"`
	Base        string   `json:"base"`
	Head        string   `json:"head"`                   // its head commit
	MergeCommit string   `json:"merge_commit,omitempty"` // on the base, once merged
	Author      string   `json:"author"`
	Decision    string   `json:"decision"`    // reviewDecision: APPROVED, REVIEW_REQUIRED, CHANGES_REQUESTED, "" (no review rule)
	Mergeable   string   `json:"mergeable"`   // MERGEABLE, CONFLICTING, UNKNOWN
	MergeState  string   `json:"merge_state"` // mergeStateStatus
	Checks      []Check  `json:"checks"`
	Reviews     []Review `json:"reviews"`   // latestOpinionatedReviews: a COMMENTED review hides no approval
	Requested   []string `json:"requested"` // the people asked to review
	// set by Gate
	CanMerge bool     `json:"can_merge"`
	Checking bool     `json:"checking"` // GitHub is still computing mergeability
	Waits    []string `json:"waits"`
}

// findQuery is a branch's pull requests, newest first. Verified against
// GitHub 2026-10-17, without preview headers, with a token of the repo
// scope alone (git-bug's): a team's fields need read:org and fail the
// whole query, so requested reviewers are people only.
// ponytail: a team asked to review (a CODEOWNERS team) reads "an
// approval"; REST's requested_reviewers if its name matters
const findQuery = `query($owner: String!, $name: String!, $head: String!) {
  repository(owner: $owner, name: $name) {
    pullRequests(headRefName: $head, first: 10, orderBy: {field: CREATED_AT, direction: DESC}) {
      nodes {
        number url state isDraft title baseRefName headRefOid
        mergeCommit { oid }
        headRepository { nameWithOwner }
        author { login }
        reviewDecision mergeable mergeStateStatus
        latestOpinionatedReviews(first: 20) { nodes { author { login } state commit { oid } } }
        reviewRequests(first: 10) { nodes { requestedReviewer { ... on User { login } } } }
        commits(last: 1) { nodes { commit { statusCheckRollup { contexts(first: 100) { nodes {
          __typename
          ... on CheckRun { name status conclusion detailsUrl }
          ... on StatusContext { context state targetUrl }
        } } } } } }
      }
    }
  }
}`

type login struct{ Login string }

type prNode struct {
	Number                   int
	URL                      string
	State                    string
	IsDraft                  bool
	Title                    string
	BaseRefName              string
	HeadRefOid               string
	MergeCommit              *struct{ Oid string }
	HeadRepository           *struct{ NameWithOwner string }
	Author                   *login
	ReviewDecision           string
	Mergeable                string
	MergeStateStatus         string
	LatestOpinionatedReviews struct {
		Nodes []struct {
			Author *login
			State  string
			Commit *struct{ Oid string }
		}
	}
	ReviewRequests struct {
		Nodes []struct {
			RequestedReviewer struct{ Login string }
		}
	}
	Commits struct {
		Nodes []struct {
			Commit struct {
				StatusCheckRollup *struct {
					Contexts struct{ Nodes []contextNode }
				}
			}
		}
	}
}

type contextNode struct {
	Typename                             string `json:"__typename"`
	Name, Status, Conclusion, DetailsURL string // a check run
	Context, State, TargetURL            string // a commit status
}

// Find is the pull request whose head is branch in repo ("owner/name"):
// the open one, else the newest. A fork's branch of the same name is
// another PR. nil when there's none.
func (c *Client) Find(ctx context.Context, repo, branch string) (*PR, error) {
	owner, name, _ := strings.Cut(repo, "/")
	var res struct {
		Data struct {
			Repository struct {
				PullRequests struct{ Nodes []prNode }
			}
		}
		Errors []struct{ Message string }
	}
	vars := map[string]string{"owner": owner, "name": name, "head": branch}
	if err := c.do(ctx, http.MethodPost, "/graphql", map[string]any{"query": findQuery, "variables": vars}, &res); err != nil {
		return nil, err
	}
	if len(res.Errors) > 0 {
		return nil, fmt.Errorf("GitHub: %s", res.Errors[0].Message)
	}
	var found *prNode
	for i, n := range res.Data.Repository.PullRequests.Nodes {
		if n.HeadRepository == nil || !strings.EqualFold(n.HeadRepository.NameWithOwner, repo) {
			continue
		}
		if n.State == "OPEN" {
			found = &res.Data.Repository.PullRequests.Nodes[i]
			break
		}
		if found == nil {
			found = &res.Data.Repository.PullRequests.Nodes[i]
		}
	}
	if found == nil {
		return nil, nil
	}
	return found.pr(), nil
}

func (n *prNode) pr() *PR {
	p := &PR{Number: n.Number, URL: n.URL, State: n.State, Draft: n.IsDraft, Title: n.Title, Base: n.BaseRefName,
		Head: n.HeadRefOid, Decision: n.ReviewDecision, Mergeable: n.Mergeable, MergeState: n.MergeStateStatus,
		Checks: []Check{}, Reviews: []Review{}, Requested: []string{}}
	if n.Author != nil {
		p.Author = n.Author.Login
	}
	if n.MergeCommit != nil {
		p.MergeCommit = n.MergeCommit.Oid
	}
	for _, r := range n.LatestOpinionatedReviews.Nodes {
		rv := Review{State: r.State}
		if r.Author != nil {
			rv.Login = r.Author.Login
		}
		if r.Commit != nil {
			rv.Commit = r.Commit.Oid
		}
		p.Reviews = append(p.Reviews, rv)
	}
	for _, r := range n.ReviewRequests.Nodes {
		if who := r.RequestedReviewer.Login; who != "" {
			p.Requested = append(p.Requested, who)
		}
	}
	for _, cm := range n.Commits.Nodes {
		if cm.Commit.StatusCheckRollup == nil {
			continue
		}
		for _, cx := range cm.Commit.StatusCheckRollup.Contexts.Nodes {
			p.Checks = append(p.Checks, cx.check())
		}
	}
	return p
}

// check is a context as the gate says it. GitHub lets a neutral or
// skipped run pass a required check.
func (n contextNode) check() Check {
	if n.Typename == "StatusContext" {
		state := "failure"
		switch n.State {
		case "SUCCESS":
			state = "success"
		case "PENDING", "EXPECTED":
			state = "pending"
		}
		return Check{Name: n.Context, State: state, URL: n.TargetURL}
	}
	state := "failure"
	switch {
	case n.Status != "COMPLETED":
		state = "pending"
	case n.Conclusion == "SUCCESS" || n.Conclusion == "NEUTRAL" || n.Conclusion == "SKIPPED":
		state = "success"
	}
	return Check{Name: n.Name, State: state, URL: n.DetailsURL}
}

// Required names the checks the base's rules require, in their order:
// the rulesets' required_status_checks.
func (c *Client) Required(ctx context.Context, repo, base string) ([]string, error) {
	var rules []struct {
		Type       string
		Parameters struct {
			RequiredStatusChecks []struct{ Context string } `json:"required_status_checks"`
		}
	}
	if err := c.do(ctx, http.MethodGet, "/repos/"+repo+"/rules/branches/"+base, nil, &rules); err != nil {
		return nil, err
	}
	var names []string
	for _, r := range rules {
		for _, rc := range r.Parameters.RequiredStatusChecks {
			if r.Type == "required_status_checks" && !slices.Contains(names, rc.Context) {
				names = append(names, rc.Context)
			}
		}
	}
	return names, nil
}

// Open opens a pull request from head into base.
func (c *Client) Open(ctx context.Context, repo, head, base, title, body string) (*PR, error) {
	in := struct {
		Title string `json:"title"`
		Head  string `json:"head"`
		Base  string `json:"base"`
		Body  string `json:"body"`
	}{title, head, base, body}
	var res struct {
		Number  int
		HTMLURL string `json:"html_url"`
	}
	if err := c.do(ctx, http.MethodPost, "/repos/"+repo+"/pulls", in, &res); err != nil {
		return nil, err
	}
	return &PR{Number: res.Number, URL: res.HTMLURL, State: "OPEN", Title: title, Base: base}, nil
}

// Merge merges a pull request with a merge commit, if its head is still
// sha: GitHub refuses otherwise, so nothing the page didn't show merges.
func (c *Client) Merge(ctx context.Context, repo string, number int, sha string) error {
	in := struct {
		Method string `json:"merge_method"`
		SHA    string `json:"sha"`
	}{"merge", sha}
	return c.do(ctx, http.MethodPut, fmt.Sprintf("/repos/%s/pulls/%d/merge", repo, number), in, nil)
}

// do sends in as JSON and decodes the answer into out; an answer of 300
// or more is an error with GitHub's message.
func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		data, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, cmp.Or(c.API, "https://api.github.com")+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := cmp.Or(c.HTTP, defaultHTTP).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		var e struct{ Message string }
		_ = json.Unmarshal(data, &e)
		return fmt.Errorf("GitHub: %s", cmp.Or(e.Message, resp.Status))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(data, out)
}

// Gate puts the base's required checks first, in the rules' order,
// adding those that never reported as expected, then says whether the PR
// can merge (as GitHub says it) and, if not, what it waits for.
func Gate(p *PR, required []string) {
	var checks []Check
	for _, name := range required {
		c := Check{Name: name, State: "expected"}
		if i := slices.IndexFunc(p.Checks, func(c Check) bool { return c.Name == name }); i >= 0 {
			c = p.Checks[i]
		}
		c.Required = true
		checks = append(checks, c)
	}
	for _, c := range p.Checks {
		if !slices.Contains(required, c.Name) {
			checks = append(checks, c)
		}
	}
	p.Checks, p.CanMerge, p.Checking, p.Waits = checks, false, false, []string{}
	if p.State != "OPEN" {
		return
	}
	p.Checking = p.Mergeable == "UNKNOWN"
	p.CanMerge = !p.Draft && p.Mergeable == "MERGEABLE" && slices.Contains([]string{"CLEAN", "HAS_HOOKS", "UNSTABLE"}, p.MergeState)
	if p.Draft {
		p.Waits = append(p.Waits, "ready for review (a draft)")
	}
	for _, c := range checks {
		if c.Required && c.State != "success" {
			p.Waits = append(p.Waits, c.Name)
		}
	}
	switch p.Decision {
	case "REVIEW_REQUIRED":
		w := "an approval"
		if len(p.Requested) > 0 {
			w += " from " + strings.Join(p.Requested, " or ")
		}
		p.Waits = append(p.Waits, w)
	case "CHANGES_REQUESTED":
		var who []string
		for _, r := range p.Reviews {
			if r.State == "CHANGES_REQUESTED" {
				who = append(who, r.Login)
			}
		}
		w := "the requested changes"
		if len(who) > 0 {
			w = "changes " + strings.Join(who, " and ") + " requested"
		}
		p.Waits = append(p.Waits, w)
	}
	if p.Mergeable == "CONFLICTING" {
		p.Waits = append(p.Waits, "conflicts with "+p.Base+" resolved")
	}
	if p.MergeState == "BEHIND" {
		p.Waits = append(p.Waits, "an update from "+p.Base)
	}
	if len(p.Waits) == 0 && p.MergeState == "BLOCKED" {
		p.Waits = append(p.Waits, "GitHub's rules")
	}
}
