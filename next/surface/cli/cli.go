// Package cli is the command line: each command builds a request from
// its flags, calls the domain, and prints the answer, as text, Markdown
// or JSON (docs/guide/cli.md). M1 reads: ls, show, log.
package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/next/app"
	"github.com/guilbep/git-board/next/domain/issues"
)

// The exit codes (docs/guide/cli.md, Exit codes).
const (
	exitError    = 1
	exitUsage    = 2
	exitNotFound = 3
	exitNoMe     = 4
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
	code := exitCode(err)
	if code == exitNoMe {
		fmt.Fprintln(stderr, "hint: git board me --new writes as you (git user.name and user.email); git board me --ls lists who is on the board, git board me <id> writes as one of them")
	}
	return code
}

func exitCode(err error) int {
	var u usageError
	if errors.As(err, &u) || errors.Is(err, issues.ErrInvalid) || errors.Is(err, issues.ErrUnchecked) {
		return exitUsage
	}
	return app.ExitCode(err)
}

func newRoot() *cobra.Command {
	var dir string
	root := &cobra.Command{
		Use:           "git-board-next",
		Short:         "git-board v2, growing beside v1 (docs/rewrite/design.md)",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version(),
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
	// bare: the board in the browser (serve.go)
	var logs logFlags
	var noOpen bool
	root.Args = cobra.NoArgs
	root.RunE = func(cmd *cobra.Command, _ []string) error { return runBoard(cmd, &logs, !noOpen) }
	logs.add(root)
	root.Flags().BoolVar(&noOpen, "no-open", false, "serve the board, or say where it runs, without opening a browser")
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return usageError{err} })
	root.AddCommand(newLs(), newView(), newShow(), newLog(), newLabels(), newMe(),
		newNew(), newComment(), newEdit(), newSet(), newMove(), newClose(), newReopen(), newDelete(), newUndo(), newFollow(), newUnfollow(), newStatus(), newDatastore(), newConfig(), newNumbers(), newForge(), newProject(), newFetch(), newPull(), newPush(), newSync(), newRelease(), newDeploy(), newVerify(), newBranch(), newSprint(), newTui(), newServe(), newInsights())
	root.SetVersionTemplate("git-board version {{.Version}}\n") // v1's, as the binary it replaces
	root.AddCommand(newGet())
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

// board is an open board, through app.
type board struct {
	*app.Board
	issues *issues.Service
}

// open opens the board of the repository holding the current directory
// (app.Open). who: read who writes; what a read leaves aside goes to the
// command's stderr, as v1 logs it.
func open(cmd *cobra.Command, who bool) (*board, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	stderr := cmd.ErrOrStderr()
	b, err := app.Open(cmd.Context(), cwd, app.Options{Who: who, Warm: who,
		Notice: func(format string, args ...any) { fmt.Fprintf(stderr, format+"\n", args...) }})
	if err != nil {
		return nil, err
	}
	// the command is one snapshot: its reads and writes check the heads once
	cmd.SetContext(b.Kernel.Snapshot(cmd.Context()))
	return &board{Board: b, issues: b.Issues}, nil
}
