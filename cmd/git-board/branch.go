package main

import (
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/internal/branches"
	"github.com/guilbep/git-board/internal/store"
)

func newBranchCmd() *cobra.Command {
	var create bool
	cmd := &cobra.Command{
		Use:   "branch <id>",
		Short: "Print the branch to create for an issue, and the command creating it; --create runs it",
		Long: `Print the branch to create for an issue, named from [branches] in
.git-board.toml, and the git command creating it from the base. It
creates nothing.

--create runs that command: it creates the branch from the base and
switches to it, as gh issue develop --checkout. Each branch goes in the
issue's folder (2518/1-..., 2518/2-...), so a second run creates the
issue's next branch.`,
		Example: `  git-board branch a1b2c3
  git-board branch a1b2c3 --create`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBranch(cmd.OutOrStdout(), cmd.ErrOrStderr(), args[0], create)
		},
	}
	cmd.Flags().BoolVar(&create, "create", false, "create the branch from the base and switch to it")
	return cmd
}

func runBranch(out, errOut io.Writer, idPrefix string, create bool) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	b, err := store.OpenCwd()
	if err != nil {
		return err
	}
	defer b.Close()

	is, err := b.GetIssue(idPrefix)
	if err != nil {
		return err
	}
	s, err := branches.Suggest(b, cfg.Branches, is)
	if err != nil {
		return err
	}
	if !create {
		fmt.Fprintf(out, "%s\n%s\n", s.Suggested, s.Command)
		return nil
	}
	// in the working tree, as the user would type it: git switch checks
	// what the change would lose
	sw := exec.Command("git", "switch", "-c", s.Suggested, s.Start)
	var stderr strings.Builder
	sw.Stdout, sw.Stderr = errOut, &stderr
	if err := sw.Run(); err != nil {
		return fmt.Errorf("%s: %s", s.Command, strings.TrimSpace(stderr.String()))
	}
	fmt.Fprintln(out, s.Suggested)
	return nil
}
