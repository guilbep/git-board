package github

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/guilbep/git-board/next/forge"
)

// The pull requests the cycle's gate reads and moves (forge.Puller):
// v1's internal/ghpr, on the adapter's client, token and pacing.

var _ forge.Puller = (*Forge)(nil)

// findQuery is a branch's pull requests, newest first. Verified against
// GitHub 2026-10-17 (v1), with a token of the repo scope alone: a team's
// fields need read:org and fail the whole query, so requested reviewers
// are people only.
const findQuery = `query($owner: String!, $name: String!, $head: String!) {
  repository(owner: $owner, name: $name) {
    pullRequests(headRefName: $head, first: 10, orderBy: {field: CREATED_AT, direction: DESC}) {
      nodes {
        number url state isDraft title body baseRefName headRefOid
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

type prLogin struct{ Login string }

type prNode struct {
	Number                   int
	URL                      string
	State                    string
	IsDraft                  bool
	Title                    string
	Body                     string
	BaseRefName              string
	HeadRefOid               string
	MergeCommit              *struct{ Oid string }
	HeadRepository           *struct{ NameWithOwner string }
	Author                   *prLogin
	ReviewDecision           string
	Mergeable                string
	MergeStateStatus         string
	LatestOpinionatedReviews struct {
		Nodes []struct {
			Author *prLogin
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

// FindPR is the pull request whose head is branch here: the open one,
// else the newest; nil for none. An open one's gate reads the base's
// required checks.
func (f *Forge) FindPR(ctx context.Context, branch string) (*forge.PR, error) {
	owner, name, _ := strings.Cut(f.s.project, "/")
	var res struct {
		Data struct {
			Repository struct {
				PullRequests struct{ Nodes []prNode }
			}
		}
		Errors []gqlError
	}
	vars := map[string]string{"owner": owner, "name": name, "head": branch}
	if err := f.call(ctx, http.MethodPost, f.s.graphql, map[string]any{"query": findQuery, "variables": vars}, &res); err != nil {
		return nil, err
	}
	if err := gqlErr(res.Errors); err != nil {
		return nil, err
	}
	var found *prNode
	for i, n := range res.Data.Repository.PullRequests.Nodes {
		if n.HeadRepository == nil || !strings.EqualFold(n.HeadRepository.NameWithOwner, f.s.project) {
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
	pr := found.pr()
	var required []string
	if pr.State == "OPEN" {
		var err error
		if required, err = f.required(ctx, pr.Base); err != nil {
			return nil, err
		}
	}
	forge.Gate(pr, required)
	return pr, nil
}

func (n *prNode) pr() *forge.PR {
	p := &forge.PR{Number: n.Number, URL: n.URL, State: n.State, Draft: n.IsDraft, Title: n.Title, Body: lf(n.Body), Base: n.BaseRefName,
		Head: n.HeadRefOid, Decision: n.ReviewDecision, Mergeable: n.Mergeable, MergeState: n.MergeStateStatus}
	if n.Author != nil {
		p.Author = n.Author.Login
	}
	if n.MergeCommit != nil {
		p.MergeCommit = n.MergeCommit.Oid
	}
	for _, r := range n.LatestOpinionatedReviews.Nodes {
		rv := forge.Review{State: r.State}
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
func (n contextNode) check() forge.Check {
	if n.Typename == "StatusContext" {
		state := "failure"
		switch n.State {
		case "SUCCESS":
			state = "success"
		case "PENDING", "EXPECTED":
			state = "pending"
		}
		return forge.Check{Name: n.Context, State: state, URL: n.TargetURL}
	}
	state := "failure"
	switch {
	case n.Status != "COMPLETED":
		state = "pending"
	case n.Conclusion == "SUCCESS" || n.Conclusion == "NEUTRAL" || n.Conclusion == "SKIPPED":
		state = "success"
	}
	return forge.Check{Name: n.Name, State: state, URL: n.DetailsURL}
}

// required names the checks the base's rules require, in their order:
// the rulesets' required_status_checks.
func (f *Forge) required(ctx context.Context, base string) ([]string, error) {
	var rules []struct {
		Type       string
		Parameters struct {
			RequiredStatusChecks []struct{ Context string } `json:"required_status_checks"`
		}
	}
	if err := f.call(ctx, http.MethodGet, f.s.repo+"/rules/branches/"+url.PathEscape(base), nil, &rules); err != nil {
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

// OpenPR opens a pull request from head into base.
func (f *Forge) OpenPR(ctx context.Context, head, base, title, body string) (*forge.PR, error) {
	in := map[string]string{"title": title, "head": head, "base": base, "body": body}
	var res struct {
		Number  int    `json:"number"`
		HTMLURL string `json:"html_url"`
	}
	if err := f.call(ctx, http.MethodPost, f.s.repo+"/pulls", in, &res); err != nil {
		return nil, err
	}
	pr := &forge.PR{Number: res.Number, URL: res.HTMLURL, State: "OPEN", Title: title, Body: body, Base: base}
	forge.Gate(pr, nil)
	return pr, nil
}

// EditPR sets a pull request's title and description.
func (f *Forge) EditPR(ctx context.Context, number int, title, body string) error {
	return f.call(ctx, http.MethodPatch, fmt.Sprintf("%s/pulls/%d", f.s.repo, number), map[string]string{"title": title, "body": body}, nil)
}

// MergePR merges with a merge commit, if the head is still sha: GitHub
// refuses otherwise.
func (f *Forge) MergePR(ctx context.Context, number int, sha string) error {
	return f.call(ctx, http.MethodPut, fmt.Sprintf("%s/pulls/%d/merge", f.s.repo, number), map[string]string{"merge_method": "merge", "sha": sha}, nil)
}
