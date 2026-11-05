// Package app opens a board and wires its domains: the one API every
// surface calls (docs/rewrite/design.md, The application). A surface
// decodes a request, calls a service here, and encodes the answer; it
// holds no rule. Errors carry their class: the exit code and the HTTP
// status, decided once.
package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/guilbep/git-board/next/conf"
	"github.com/guilbep/git-board/next/domain/issues"
	"github.com/guilbep/git-board/next/domain/people"
	"github.com/guilbep/git-board/next/kernel"
	"github.com/guilbep/git-board/next/store/git"
	"github.com/guilbep/git-board/next/view"
)

// Board is an open board and its services.
type Board struct {
	Issues *issues.Service
	People *people.Service
	Config *conf.File
	Views  map[string]issues.View
	Kernel *kernel.Board
	Dir    string // the repository's working directory, or the folder opened
	clone  *clone
}

// Options say how to open a board.
type Options struct {
	// Who reads the clone's board.me and board.avatars: what show and log
	// print of people, @me, and every write.
	Who bool
	// Warm starts reading an entity's packs at once: show and log read
	// one.
	Warm bool
	// Notice is told what a read leaves aside (an entity broken, a
	// counter that doesn't read), as v1 logs it.
	Notice issues.Notice
}

// Open opens the board of the repository holding dir: the git
// datastore, its view in <common dir>/board/v2/view.db (v1's index.db is
// v1's), .git-board.toml, and with o.Who who writes.
func Open(ctx context.Context, dir string, o Options) (*Board, error) {
	type set struct {
		me, avatars string
		err         error
	}
	settled := make(chan set, 1)
	if o.Who { // git config runs while the rest opens
		go func() {
			var s set
			s.me, s.avatars, s.err = settings(ctx, dir)
			settled <- s
		}()
	}
	ds, err := git.Open(ctx, dir)
	if err != nil {
		return nil, err
	}
	if o.Warm {
		go ds.Warm(ctx)
	}
	f, err := conf.Load(configPath(dir))
	if err != nil {
		return nil, err
	}
	tables := issues.Declare(f)
	schema, probs := tables.Schema()
	if len(probs) > 0 {
		return nil, fmt.Errorf("%s: %w", conf.FileName, probs[0])
	}
	views, probs := tables.Views()
	if len(probs) > 0 {
		return nil, fmt.Errorf("%s: %w", conf.FileName, probs[0])
	}
	v, err := view.Open(ctx, filepath.Join(ds.Folder(), "v2", "view.db"))
	if err != nil {
		return nil, fmt.Errorf("opening the view: %w", err)
	}
	kb, err := kernel.Open(ctx, ds, v, issues.Kind, people.Kind)
	if err != nil {
		v.Close()
		return nil, err
	}
	c := &clone{dir: dir}
	b := &Board{Kernel: kb, Config: f, Views: views, Dir: dir, clone: c,
		Issues: &issues.Service{Board: kb, Schema: schema, Avatars: people.Forge, Notice: o.Notice},
		People: &people.Service{Board: kb, Avatars: people.Forge, SetMe: c.setMe}}
	if o.Who {
		set := <-settled
		if set.err != nil {
			kb.Close()
			return nil, set.err
		}
		b.Issues.Me, b.Issues.Avatars = kernel.ID(set.me), people.ParseAvatars(set.avatars)
		b.People.Me, b.People.Avatars = b.Issues.Me, b.Issues.Avatars
	}
	return b, nil
}

// Close lets the board go.
func (b *Board) Close() error { return b.Kernel.Close() }

// configPath is the nearest .git-board.toml up to the repository's root,
// else where it goes at that root.
func configPath(dir string) string {
	for {
		p := filepath.Join(dir, conf.FileName)
		if _, err := os.Stat(p); err == nil {
			return p
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return p
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return conf.FileName
		}
		dir = parent
	}
}

// settings are the clone's board.me, from its own config only
// (data-model.md, Me), and board.avatars, from any level: one git.
func settings(ctx context.Context, dir string) (me, avatars string, err error) {
	out, err := gitRun(ctx, dir, "config", "--show-scope", "--get-regexp", `^board\.(me|avatars)$`)
	if ee := (*exec.ExitError)(nil); errors.As(err, &ee) && ee.ExitCode() == 1 {
		return "", "", nil // neither is set
	}
	if err != nil {
		return "", "", err
	}
	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		scope, kv, _ := strings.Cut(line, "\t")
		k, v, _ := strings.Cut(kv, " ")
		switch {
		case k == "board.me" && scope == "local":
			me = v
		case k == "board.avatars":
			avatars = v // the last level wins, as git's --get
		}
	}
	return me, avatars, nil
}

// clone is this clone's own git config: who writes.
type clone struct{ dir string }

// setMe writes board.me in the clone's own config, never the global one.
func (c *clone) setMe(ctx context.Context, id kernel.ID) error {
	_, err := gitRun(ctx, c.dir, "config", "--local", "board.me", string(id))
	return err
}

// GitConfig is a git config value at any level, "" when unset: user.name
// for me --new.
func (b *Board) GitConfig(ctx context.Context, key string) string {
	out, err := gitRun(ctx, b.Dir, "config", "--get", key)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func gitRun(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return string(out), nil
}

// Views reads the views .git-board.toml names, without opening the board.
func Views(dir string) (map[string]issues.View, error) {
	f, err := conf.Load(configPath(dir))
	if err != nil {
		return nil, err
	}
	views, probs := issues.Declare(f).Views()
	if len(probs) > 0 {
		return nil, fmt.Errorf("%s: %w", conf.FileName, probs[0])
	}
	return views, nil
}
