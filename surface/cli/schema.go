package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/app"
	"github.com/guilbep/git-board/conf"
)

// config schema, upgrade and downgrade: the file's schema version
// (docs/guide/config-schema.md).

func newConfigSchema() *cobra.Command {
	var js *jsonOut
	cmd := &cobra.Command{
		Use:   "schema",
		Short: "Show the file's schema version, this git-board's, and the migrations between",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := os.Getwd()
			if err != nil {
				return err
			}
			s, err := app.ConfigSchema(dir)
			if err != nil {
				return err
			}
			if js.on() {
				return js.write(cmd.OutOrStdout(), s)
			}
			printSchema(cmd.OutOrStdout(), s)
			return nil
		},
	}
	js = addJSON(cmd, `print {"file", "binary", "pending"}`, jsonFields(app.SchemaState{}), true)
	return cmd
}

func printSchema(out io.Writer, s app.SchemaState) {
	switch {
	case s.File == 0:
		fmt.Fprintf(out, "%s: none here (git board init writes one)\n", conf.FileName)
	case s.File > s.Binary:
		fmt.Fprintf(out, "%s: schema %d, newer than this git-board: it reads what it knows and writes nothing (upgrade git-board)\n", conf.FileName, s.File)
	default:
		fmt.Fprintf(out, "%s: schema %d\n", conf.FileName, s.File)
	}
	fmt.Fprintf(out, "git-board: schema %d\n", s.Binary)
	if len(s.Pending) == 0 {
		return
	}
	fmt.Fprintln(out, "pending:")
	printSteps(out, s.Pending, true)
	fmt.Fprintln(out, "git board config upgrade writes them (--dry-run shows the diff); a set or unset does too")
}

func printSteps(out io.Writer, steps []conf.Migration, up bool) {
	for _, m := range steps {
		from, to := m.From, m.To
		if !up {
			from, to = to, from
		}
		if up {
			fmt.Fprintf(out, "  %d to %d: %s\n", from, to, m.Desc)
			continue
		}
		fmt.Fprintf(out, "  %d to %d, undoing what %d to %d does: %s\n", from, to, m.From, m.To, m.Desc)
		if m.Lossy != "" {
			fmt.Fprintf(out, "    lossy: %s\n", m.Lossy)
		}
	}
}

// newConfigMigrate is config upgrade, or downgrade.
func newConfigMigrate(up bool) *cobra.Command {
	var to int
	var dry bool
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade .git-board.toml to this git-board's schema, or --to a version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := os.Getwd()
			if err != nil {
				return err
			}
			if !up && !cmd.Flags().Changed("to") {
				return usageError{fmt.Errorf("downgrade needs --to: the version to go down to")}
			}
			m, err := app.MigrateConfig(dir, to, up, dry)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			switch {
			case len(m.Steps) == 0:
				fmt.Fprintf(out, "%s is schema %d already\n", conf.FileName, m.From)
			case dry:
				fmt.Fprint(out, m.Diff)
			default:
				fmt.Fprintf(out, "%s: schema %d to %d\n", conf.FileName, m.From, m.To)
				printSteps(out, m.Steps, up)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&to, "to", conf.Current, "the schema `version` to go to")
	cmd.Flags().BoolVar(&dry, "dry-run", false, "print the diff, write nothing")
	if !up {
		cmd.Use = "downgrade --to <version>"
		cmd.Short = "Downgrade .git-board.toml to an older schema, for an older git-board"
	}
	return cmd
}
