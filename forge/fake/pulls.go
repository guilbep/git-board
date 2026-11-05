package fake

import (
	"context"
	"slices"
	"strconv"

	"github.com/guilbep/git-board/forge"
)

// The fake's pull requests (forge.Puller): AddPR puts one on the forge's
// side, as its users would; the forge.Puller methods are what the cycle
// sees. A PR's Head is its head branch's name here, and Gate is set on
// each read.

var _ forge.Puller = (*Forge)(nil)

// AddPR adds a pull request whose head branch is branch, and answers its
// number. pr.Head is its head commit.
func (f *Forge) AddPR(branch string, pr forge.PR) int {
	f.p.mu.Lock()
	defer f.p.mu.Unlock()
	pr.Number = len(f.p.pulls) + 1
	if pr.URL == "" {
		pr.URL = "https://" + f.p.key + "/pull/" + strconv.Itoa(pr.Number)
	}
	if pr.State == "" {
		pr.State = "OPEN"
	}
	f.p.pulls = append(f.p.pulls, &pr)
	f.p.heads = append(f.p.heads, branch)
	return pr.Number
}

// SetPR changes a pull request as the forge's side would: merges it,
// approves it, closes it.
func (f *Forge) SetPR(number int, change func(*forge.PR)) {
	f.p.mu.Lock()
	defer f.p.mu.Unlock()
	change(f.p.pulls[number-1])
}

// PRReads counts FindPR's calls: what a cache saves.
func (f *Forge) PRReads() int {
	f.p.mu.Lock()
	defer f.p.mu.Unlock()
	return f.p.prReads
}

func clonePR(p *forge.PR) *forge.PR {
	c := *p
	c.Checks, c.Reviews, c.Requested = slices.Clone(p.Checks), slices.Clone(p.Reviews), slices.Clone(p.Requested)
	forge.Gate(&c, nil)
	return &c
}

// FindPR is branch's open PR, else its newest.
func (f *Forge) FindPR(_ context.Context, branch string) (*forge.PR, error) {
	f.p.mu.Lock()
	defer f.p.mu.Unlock()
	f.p.prReads++
	var found *forge.PR
	for i := len(f.p.pulls) - 1; i >= 0; i-- {
		if f.p.heads[i] != branch {
			continue
		}
		if p := f.p.pulls[i]; p.State == "OPEN" {
			return clonePR(p), nil
		} else if found == nil {
			found = p
		}
	}
	if found == nil {
		return nil, nil
	}
	return clonePR(found), nil
}

// OpenPR opens a pull request from head into base.
func (f *Forge) OpenPR(_ context.Context, head, base, title, body string) (*forge.PR, error) {
	var out *forge.PR
	err := f.write(Write{Op: "open-pr", Args: []string{head, base, title, body}}, func(p *project, _ int64) error {
		pr := &forge.PR{Number: len(p.pulls) + 1, State: "OPEN", Title: title, Body: body, Base: base, Mergeable: "MERGEABLE", MergeState: "CLEAN"}
		pr.URL = "https://" + p.key + "/pull/" + strconv.Itoa(pr.Number)
		p.pulls, p.heads = append(p.pulls, pr), append(p.heads, head)
		out = clonePR(pr)
		return nil
	})
	return out, err
}

func (p *project) pull(number int) *forge.PR {
	if number < 1 || number > len(p.pulls) {
		return nil
	}
	return p.pulls[number-1]
}

// EditPR sets a pull request's title and description.
func (f *Forge) EditPR(_ context.Context, number int, title, body string) error {
	return f.write(Write{Op: "edit-pr", Issue: strconv.Itoa(number), Args: []string{title, body}}, func(p *project, _ int64) error {
		pr := p.pull(number)
		if pr == nil {
			return forge.ErrNotFound
		}
		pr.Title, pr.Body = title, body
		return nil
	})
}

// MergePR merges an open pull request whose head is still sha.
func (f *Forge) MergePR(_ context.Context, number int, sha string) error {
	return f.write(Write{Op: "merge-pr", Issue: strconv.Itoa(number), Args: []string{sha}}, func(p *project, _ int64) error {
		pr := p.pull(number)
		switch {
		case pr == nil:
			return forge.ErrNotFound
		case pr.State != "OPEN" || pr.Head != sha:
			return &forge.Refused{Values: []string{sha}, Why: "the head moved, or the pull request isn't open"}
		}
		pr.State, pr.MergeCommit = "MERGED", sha
		return nil
	})
}
