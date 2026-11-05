package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/app"
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

When origin is on GitHub, the file starts as its labels and its first
Project's fields (names, colors, descriptions, values in order); the
template fills what they don't have. On GitLab, its labels: GitLab has
no Projects. No token, or nothing read: the template, and a line saying
what wasn't read.

--template names the template, else the global file's, else the default: a built-in one by name
(enac, EPFL-ENAC's labels and GitHub Project), a file, or an http(s) URL.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			path, lines, err := app.InitConfig(cmd.Context(), cwd, template)
			for _, l := range lines {
				fmt.Fprintln(cmd.OutOrStdout(), l)
			}
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "wrote %s: edit it in Settings › Project file (git board), or by hand\n", path)
			return nil
		},
	}
	cmd.Flags().StringVar(&template, "template", "", "start from a template: enac, a file or a URL")
	return cmd
}
