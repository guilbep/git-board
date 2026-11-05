// Package cli is the command line: each command builds a request from
// its flags, calls the domain, and prints the answer, as text, Markdown
// or JSON (docs/guide/cli.md). M1 reads: ls, show, log.
package cli

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/next/conf"
	"github.com/guilbep/git-board/next/domain/issues"
	"github.com/guilbep/git-board/next/domain/people"
	"github.com/guilbep/git-board/next/kernel"
	"github.com/guilbep/git-board/next/store/git"
	"github.com/guilbep/git-board/next/view"
)

// The exit codes (docs/guide/cli.md, Exit codes).
const (
	exitError    = 1
	exitUsage    = 2
	exitNotFound = 3
)

// usageError is a command line the command can't run: a flag, an
// argument.
type usageError struct{ error }

func (e usageError) Unwrap() error { return e.error }

// Main runs the command line args, and answers the exit code.
func Main(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	out := bufio.NewWriterSize(stdout, 64<<10)
	defer out.Flush()
	root := newRoot()
	root.SetOut(out)
	root.SetErr(stderr)
	root.SetContext(ctx)
	err := runRoot(root, args)
	if err == nil {
		err = out.Flush()
	}
	if err == nil {
		return 0
	}
	out.Flush() // what was printed before the error
	fmt.Fprintln(stderr, "error:", err)
	return exitCode(err)
}

func exitCode(err error) int {
	var u usageError
	switch {
	case errors.Is(err, kernel.ErrNotFound), errors.Is(err, kernel.ErrAmbiguous):
		return exitNotFound
	case errors.As(err, &u), errors.Is(err, issues.ErrUsage), strings.HasPrefix(err.Error(), "unknown command"):
		return exitUsage
	}
	return exitError
}

func newRoot() *cobra.Command {
	var dir string
	root := &cobra.Command{
		Use:           "git-board-next",
		Short:         "git-board v2, read-only for now: ls, show, log",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(*cobra.Command, []string) error {
			if dir == "" {
				return nil
			}
			if err := os.Chdir(dir); err != nil {
				return usageError{fmt.Errorf("-C %s: %w", dir, err)}
			}
			return nil
		},
	}
	root.PersistentFlags().StringVarP(&dir, "directory", "C", "", "run as if started in this directory, as git -C does")
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return usageError{err} })
	root.AddCommand(newLs(), newShow(), newLog())
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		if args := c.Args; args != nil {
			c.Args = func(cmd *cobra.Command, a []string) error {
				if err := args(cmd, a); err != nil {
					return usageError{err}
				}
				return nil
			}
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(root)
	return root
}

// board is an open board and the issue domain on it.
type board struct {
	kb     *kernel.Board
	issues *issues.Service
}

func (b *board) Close() error { return b.kb.Close() }

// open opens the board of the repository holding the current directory:
// the git datastore, its view in <common dir>/board/v2/view.db (v1's
// index.db is v1's), .git-board.toml, and the clone's board.me and
// board.avatars. who: read board.me and board.avatars (show, log, @me).
func open(ctx context.Context, who bool) (*board, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	type set struct {
		me, avatars string
		err         error
	}
	settled := make(chan set, 1)
	if who { // git config runs while the rest opens
		go func() {
			var s set
			s.me, s.avatars, s.err = settings(ctx, cwd)
			settled <- s
		}()
	}
	ds, err := git.Open(ctx, cwd)
	if err != nil {
		return nil, err
	}
	if who { // show and log read one issue's packs: their cat-file starts now
		go ds.Warm(ctx)
	}
	f, err := conf.Load(configPath(cwd))
	if err != nil {
		return nil, err
	}
	schema, probs := issues.Declare(f).Schema()
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
	s := &issues.Service{Board: kb, Schema: schema, Avatars: people.Forge}
	if who {
		set := <-settled
		if set.err != nil {
			kb.Close()
			return nil, set.err
		}
		s.Me, s.Avatars = kernel.ID(set.me), people.ParseAvatars(set.avatars)
	}
	return &board{kb: kb, issues: s}, nil
}

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
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "config", "--show-scope", "--get-regexp", `^board\.(me|avatars)$`)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if ee := (*exec.ExitError)(nil); errors.As(err, &ee) && ee.ExitCode() == 1 {
		return "", "", nil // neither is set
	}
	if err != nil {
		return "", "", fmt.Errorf("git config: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
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
