package gitlab

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/guilbep/git-board/next/forge"
)

// The merge requests the cycle's gate reads and moves (forge.Puller), in
// forge.PR's words, which are GitHub's: opened is OPEN, merged MERGED,
// closed and locked CLOSED; GitLab's merge status says MERGEABLE,
// CONFLICTING or UNKNOWN, and its head pipeline is the one check.

var _ forge.Puller = (*Forge)(nil)

type glMR struct {
	IID          int    `json:"iid"`
	WebURL       string `json:"web_url"`
	State        string `json:"state"`
	Draft        bool   `json:"draft"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	TargetBranch string `json:"target_branch"`
	SHA          string `json:"sha"`
	MergeCommit  string `json:"merge_commit_sha"`
	Author       user   `json:"author"`
	Status       string `json:"detailed_merge_status"`
	Conflicts    bool   `json:"has_conflicts"`
	Pipeline     *struct {
		Status string `json:"status"`
		WebURL string `json:"web_url"`
	} `json:"head_pipeline"`
}

func (m glMR) pr() *forge.PR {
	p := &forge.PR{Number: m.IID, URL: m.WebURL, Draft: m.Draft, Title: m.Title, Body: text(m.Description), Base: m.TargetBranch,
		Head: m.SHA, MergeCommit: m.MergeCommit, Author: m.Author.Username}
	switch m.State {
	case "opened":
		p.State = "OPEN"
	case "merged":
		p.State = "MERGED"
	default:
		p.State = "CLOSED"
	}
	switch {
	case m.Conflicts || m.Status == "conflict":
		p.Mergeable, p.MergeState = "CONFLICTING", "DIRTY"
	case m.Status == "mergeable":
		p.Mergeable, p.MergeState = "MERGEABLE", "CLEAN"
	case m.Status == "checking" || m.Status == "unchecked" || m.Status == "preparing" || m.Status == "":
		p.Mergeable, p.MergeState = "UNKNOWN", "UNKNOWN"
	case m.Status == "need_rebase":
		p.Mergeable, p.MergeState = "MERGEABLE", "BEHIND"
	case m.Status == "not_approved":
		p.Mergeable, p.MergeState, p.Decision = "MERGEABLE", "BLOCKED", "REVIEW_REQUIRED"
	default:
		p.Mergeable, p.MergeState = "MERGEABLE", "BLOCKED"
	}
	if m.Pipeline != nil {
		state := "failure"
		switch m.Pipeline.Status {
		case "success", "skipped", "manual":
			state = "success"
		case "created", "waiting_for_resource", "preparing", "pending", "running", "scheduled":
			state = "pending"
		}
		p.Checks = append(p.Checks, forge.Check{Name: "pipeline", State: state, URL: m.Pipeline.WebURL})
	}
	return p
}

// FindPR is the merge request whose source is branch, in this project:
// the open one, else the newest; nil for none.
func (f *Forge) FindPR(ctx context.Context, branch string) (*forge.PR, error) {
	var list []glMR
	q := url.Values{"source_branch": {branch}, "order_by": {"created_at"}, "sort": {"desc"}, "per_page": {"20"}}
	if _, err := f.call(ctx, http.MethodGet, f.p.api+"/merge_requests", q, nil, &list); err != nil {
		return nil, err
	}
	pick := -1
	for i, m := range list {
		if m.State == "opened" {
			pick = i
			break
		}
		if pick < 0 {
			pick = i
		}
	}
	if pick < 0 {
		return nil, nil
	}
	m := list[pick]
	// the list leaves out the head pipeline: one read of the open one
	if m.State == "opened" {
		if _, err := f.call(ctx, http.MethodGet, fmt.Sprintf("%s/merge_requests/%d", f.p.api, m.IID), nil, nil, &m); err != nil {
			return nil, err
		}
	}
	pr := m.pr()
	forge.Gate(pr, nil)
	return pr, nil
}

// OpenPR opens a merge request from head into base.
func (f *Forge) OpenPR(ctx context.Context, head, base, title, body string) (*forge.PR, error) {
	var m glMR
	in := map[string]string{"source_branch": head, "target_branch": base, "title": title, "description": body}
	if _, err := f.call(ctx, http.MethodPost, f.p.api+"/merge_requests", nil, in, &m); err != nil {
		return nil, err
	}
	pr := m.pr()
	forge.Gate(pr, nil)
	return pr, nil
}

// EditPR sets a merge request's title and description.
func (f *Forge) EditPR(ctx context.Context, number int, title, body string) error {
	_, err := f.call(ctx, http.MethodPut, fmt.Sprintf("%s/merge_requests/%d", f.p.api, number), nil, map[string]string{"title": title, "description": body}, nil)
	return err
}

// MergePR merges a merge request if its head is still sha: GitLab
// refuses otherwise.
func (f *Forge) MergePR(ctx context.Context, number int, sha string) error {
	_, err := f.call(ctx, http.MethodPut, fmt.Sprintf("%s/merge_requests/%d/merge", f.p.api, number), nil, map[string]string{"sha": sha}, nil)
	return err
}
