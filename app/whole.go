package app

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"

	"github.com/guilbep/git-board/tools/ghproject"
)

// The whole sync (git board sync): a git pull from the remote, the forges'
// sync, then a git push to the remote, which numbers what origin has, as
// v1's internal/store SyncWhole. A failed pull stops the steps after it;
// a failed forge step is told, and the push still runs.
// The GitHub Project's step runs the bridge both ways when
// [github-project] names one (ghproject).
// ponytail: the whole sync's dry run (sync --check: a fetch, the forges'
// check over the fetched copies) is v1's still; forge … --check and
// project … --check run.

// The steps.
const (
	StepPull    = "pull"
	StepForges  = "forges"
	StepProject = "project"
	StepPush    = "push"
)

// Whole is one whole sync: its remote, its forge step (nil: none has its
// sync on), and who hears each step: running, done, skipped or failed,
// with a note.
type Whole struct {
	Remote string
	// DryRun is the check: a fetch and compare, the forges' dry run (the
	// caller's Forges) and the Project's check; nothing written.
	DryRun bool
	Forges func(context.Context) error
	// NoProject skips the GitHub Project's step (sync --git).
	NoProject bool
	Step      func(name, state, note string)
}

// hasRemote says the repository has the remote.
func (b *Board) hasRemote(ctx context.Context, remote string) (bool, error) {
	rs, err := b.Remotes(ctx)
	return slices.Contains(rs, remote), err
}

// SyncWhole runs the whole sync's steps in turn, each told.
func (b *Board) SyncWhole(ctx context.Context, w Whole) error {
	step := w.Step
	if step == nil {
		step = func(string, string, string) {}
	}
	has := false
	if b.inGit() {
		var err error
		if has, err = b.hasRemote(ctx, w.Remote); err != nil {
			return err
		}
		gp, _ := b.Config.Tools.GithubProject.Get()
		if !has && w.Forges == nil && (gp.Number == 0 || w.NoProject) {
			return fmt.Errorf("nothing to sync: no git remote %s, no forge with its sync on, no [github-project]", w.Remote)
		}
	}
	type stage struct {
		name, skip string // skip: why, when do is nil
		do         func(context.Context) (string, error)
	}
	noRemote := "no git remote " + w.Remote
	if !b.inGit() {
		noRemote = "the board is kept in " + b.DatastoreName() + ", which every clone shares"
	}
	steps := []stage{
		{StepPull, noRemote, nil},
		{StepForges, "no forge has its sync on", nil},
		{StepProject, "no [github-project] in .git-board.toml", nil},
		{StepPush, noRemote, nil},
	}
	if gp, _ := b.Config.Tools.GithubProject.Get(); gp.Owner != "" && gp.Number != 0 && !w.NoProject {
		steps[2].do = func(ctx context.Context) (string, error) {
			r, err := b.ProjectRun(ctx, ghproject.Both, !w.DryRun, 0)
			return ProjectNote(r), err
		}
	}
	if has {
		steps[0].do = func(ctx context.Context) (string, error) { return b.Pull(ctx, w.Remote) }
		steps[3].do = func(ctx context.Context) (string, error) { return b.Push(ctx, w.Remote) }
		if w.DryRun {
			steps[0].do = func(ctx context.Context) (string, error) { return b.pullCheck(ctx, w.Remote) }
			steps[3].do, steps[3].skip = nil, "a check pushes nothing: the fetch compared"
		}
	}
	if w.Forges != nil {
		steps[1].do = func(ctx context.Context) (string, error) { return "", w.Forges(ctx) }
	}
	var errs []error
	for i, s := range steps {
		if s.do == nil {
			step(s.name, "skipped", s.skip)
			continue
		}
		step(s.name, "running", "")
		note, err := s.do(ctx)
		if err == nil {
			step(s.name, "done", note)
			continue
		}
		step(s.name, "failed", NoCredentials(err.Error()))
		errs = append(errs, fmt.Errorf("%s: %w", s.name, err))
		if s.name == StepPull || ctx.Err() != nil {
			for _, rest := range steps[i+1:] {
				step(rest.name, "skipped", s.name+" failed")
			}
			break
		}
	}
	return errors.Join(errs...)
}

var credentials = regexp.MustCompile(`://[^/\s@]+@`)

// NoCredentials is a text with the user and password of every URL in it
// taken out: a git URL may carry a token.
func NoCredentials(s string) string { return credentials.ReplaceAllString(s, "://") }
