package app

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// The whole sync (git board sync): a git pull from the remote, the forges'
// sync, then a git push to the remote, which numbers what origin has, as
// v1's internal/store SyncWhole. A failed pull stops the steps after it;
// a failed forge step is told, and the push still runs.
// ponytail: the GitHub Project's step and the dry run are v1's still.

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
	Forges func(context.Context) error
	Step   func(name, state, note string)
}

// hasRemote says the repository has the remote.
func (b *Board) hasRemote(ctx context.Context, remote string) (bool, error) {
	out, err := gitRun(ctx, b.Dir, "remote")
	if err != nil {
		return false, err
	}
	return slices.Contains(strings.Fields(out), remote), nil
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
		if !has && w.Forges == nil {
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
	if gp, _ := b.Config.Tools.GithubProject.Get(); gp.Owner != "" && gp.Number != 0 {
		steps[2].skip = "git-board-next doesn't sync the GitHub Project yet: git board project sync does"
	}
	if has {
		steps[0].do = func(ctx context.Context) (string, error) { return b.Pull(ctx, w.Remote) }
		steps[3].do = func(ctx context.Context) (string, error) { return b.Push(ctx, w.Remote) }
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
