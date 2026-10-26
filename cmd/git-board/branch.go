package main

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/internal/branches"
	"github.com/guilbep/git-board/internal/store"
)

func newBranchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "branch <id>",
		Short: "Print the branch to create for an issue, and the command creating it",
		Long: `Print the branch to create for an issue, named from [branches] in
.git-board.toml, and the git command creating it from the base. It
creates nothing.`,
		Example: `  git-board branch a1b2c3`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBranch(cmd.OutOrStdout(), args[0])
		},
	}
}

func runBranch(out io.Writer, idPrefix string) error {
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
	fmt.Fprintf(out, "%s\n%s\n", s.Suggested, s.Command)
	return nil
}
