package store

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

// Whole is the whole sync, as git-board sync and Settings › Sync run it:
// a git pull from Remote, the forge sync, the GitHub Project's fields,
// then a git push to Remote. Its check (DryRun) is a fetch and compare,
// the forges' dry run and the Project's check: nothing written.
type Whole struct {
	Remote string
	DryRun bool
	// Forges runs the forge sync, its dry run with DryRun: the caller's,
	// who opens the forges and hears their events; nil, none has sync on.
	// A forge that can't be read is told, and the run goes on.
	Forges func(ctx context.Context) error
	// Project syncs the GitHub Project's fields, or checks them with
	// DryRun; nil, no [github-project].
	Project func(ctx context.Context) error
	// Step hears each step start (running) and end: done, failed, or
	// skipped, with a note: why it failed or was skipped, what it did.
	Step func(name, state, note string)
}

// The steps of a whole sync, in order: Steps.
const (
	StepPull    = "pull" // the check's fetch
	StepForges  = "forges"
	StepProject = "project"
	StepPush    = "push"
)

var Steps = []string{StepPull, StepForges, StepProject, StepPush}

// SyncWhole runs the whole sync's steps in order. A step that fails stops
// the steps after it, and the error names it. Without the remote, the git
// steps are skipped: the forges still sync.
func (b *Backend) SyncWhole(ctx context.Context, w Whole) error {
	step := w.Step
	if step == nil {
		step = func(string, string, string) {}
	}
	has, err := b.hasRemote(w.Remote)
	if err != nil {
		return err
	}
	type stage struct {
		name, skip string // skip: why, when do is nil
		do         func(context.Context) (string, error)
	}
	steps := []stage{
		{StepPull, "no git remote " + w.Remote, nil},
		{StepForges, "no forge has its sync on", nil},
		{StepProject, "no [github-project] in .git-board.toml", nil},
		{StepPush, "no git remote " + w.Remote, nil},
	}
	if has {
		steps[0].do = func(context.Context) (string, error) { return "", b.pullStep(w.Remote, w.DryRun) }
		steps[3].do = func(context.Context) (string, error) { return b.pushStep(w.Remote) }
		if w.DryRun {
			steps[3].do, steps[3].skip = nil, "a check pushes nothing: the fetch compared"
		}
	}
	if w.Forges != nil {
		steps[1].do = func(ctx context.Context) (string, error) { return "", w.Forges(ctx) }
	}
	if w.Project != nil {
		steps[2].do = func(ctx context.Context) (string, error) { return "", w.Project(ctx) }
	}
	for i, s := range steps {
		if s.do == nil {
			step(s.name, "skipped", s.skip)
			continue
		}
		step(s.name, "running", "")
		note, err := s.do(ctx)
		if err != nil {
			step(s.name, "failed", NoCredentials(err.Error()))
			for _, rest := range steps[i+1:] {
				step(rest.name, "skipped", s.name+" failed")
			}
			return fmt.Errorf("%s: %w", s.name, err)
		}
		step(s.name, "done", note)
	}
	return nil
}

// pullStep merges the remote's issues and people in, or with dry only
// fetches their copies here to compare. Either refuses a repository the
// switch didn't mark, and a counter holding numbers the remote hasn't.
func (b *Backend) pullStep(remote string, dry bool) error {
	do, numbers := b.Pull, b.FetchNumbers
	if dry {
		do, numbers = b.Fetch, b.numbersBehind
	}
	if err := do(remote); err != nil {
		return err
	}
	if err := b.Switched(); err != nil {
		return err
	}
	if remote != NumbersRemote {
		return nil
	}
	return numbers(remote)
}

// pushStep pushes the issues and people, then numbers what the remote
// now has (NumberIssues), on origin.
func (b *Backend) pushStep(remote string) (string, error) {
	if _, err := b.Push(remote); err != nil {
		if strings.Contains(err.Error(), "non-fast-forward") {
			return "", fmt.Errorf("%s has changes this repository doesn't have: sync again", remote)
		}
		return "", err
	}
	if remote != NumbersRemote {
		return "", nil
	}
	n, err := b.NumberIssues(remote)
	switch {
	case err != nil || n == 0:
		return "", err
	case n == 1:
		return "numbered 1 issue", nil
	}
	return fmt.Sprintf("numbered %d issues", n), nil
}

// hasRemote says whether the repository has a git remote of that name.
func (b *Backend) hasRemote(remote string) (bool, error) {
	if err := validRemote(remote); err != nil {
		return false, err
	}
	out, err := b.git("remote")
	if err != nil {
		return false, err
	}
	return slices.Contains(lines(out), remote), nil
}
