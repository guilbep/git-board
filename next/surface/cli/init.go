package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/next/app"
)

// git board init, as v1's.

func newInit() *cobra.Command {
	var template string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Write the default .git-board.toml at the repository's root",
		Long: `Writes the default .git-board.toml at the repository's root: the
statuses, priorities, kinds and estimates as fields, branch names, and
four views (all, backlog, board, mine), each explained in a comment.
It never overwrites one.

--template names the template, else the global file's, else the default: a built-in one by name
(enac, EPFL-ENAC's labels and GitHub Project), a file, or an http(s) URL.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			path, notes, err := app.InitConfig(cwd, template)
			if err != nil {
				return err
			}
			for _, n := range notes {
				fmt.Fprintln(cmd.OutOrStdout(), n)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "wrote %s: edit it in Settings › Project file (git board), or by hand\n", path)
			return nil
		},
	}
	cmd.Flags().StringVar(&template, "template", "", "start from a template: enac, a file or a URL")
	return cmd
}
