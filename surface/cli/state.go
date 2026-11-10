package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/app"
)

// git board state: the board's state, named (app.State), and what to do
// next. GET /api/state answers the same.

func newState() *cobra.Command {
	var js *jsonOut
	cmd := &cobra.Command{
		Use:   "state",
		Short: "The board's state, and what to do next",
		Long: `Say the state of each part of the board, in a word, and the next
actions, the first first:

  board      new (no identity, or nothing yet), or ready
  file       none (the defaults hold), current, older, newer
  me         none (the board is read-only), or chosen
  datastore  git, markdown, postgres, forges
  forge      off, no-token, first (never synced), on, synced
  project    none, broken, configured
  run        idle, running, stopped (the next sync resumes it)

Each next action names the command that does it. Nothing goes to the
network: git board forge status --check asks the forges.

--json prints the same, with each action's command and API route: what
an agent reads.`,
		Example: `  git board state
  git board state --json
  git board state --jq '.next[0].cli'`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			b, err := open(cmd, true)
			if err != nil {
				return err
			}
			defer b.Close()
			s, err := b.State(cmd.Context())
			if err != nil {
				return err
			}
			if js.on() {
				return js.write(cmd.OutOrStdout(), s)
			}
			printState(cmd.OutOrStdout(), s)
			return nil
		},
	}
	js = addJSON(cmd, "print the state as JSON", jsonFields(app.State{}), true)
	return cmd
}

func printState(out io.Writer, s app.State) {
	line := func(k, v string, more ...string) {
		if m := strings.Join(more, ", "); m != "" {
			v += " (" + m + ")"
		}
		fmt.Fprintf(out, "%-10s %s\n", k, v)
	}
	line("board", s.Phase)
	line("file", s.File.State, s.File.Path)
	line("me", s.Me.State, s.Me.ID)
	var ds []string
	if s.Datastore.Where != s.Datastore.State {
		ds = append(ds, s.Datastore.Where)
	}
	if s.Datastore.FromEnv {
		ds = append(ds, "set by "+app.DatastoreEnv)
	}
	line("datastore", s.Datastore.State, ds...)
	for _, f := range s.Forges {
		more := []string{f.Kind + " " + f.Project, f.Way}
		if f.Discovered {
			more = append(more, "found in the git remotes")
		}
		if f.LastRun != "" {
			more = append(more, "last run "+f.LastRun)
		}
		line("forge", f.Name+": "+f.State, more...)
	}
	switch s.Project.State {
	case "configured":
		stated := "the default fields"
		if s.Project.Stated {
			stated = "fields stated"
		}
		line("project", s.Project.State, s.Project.Project, stated)
	case "broken":
		line("project", s.Project.State, s.Project.Problem)
	default:
		line("project", s.Project.State)
	}
	if s.Run.Stopped != nil {
		line("run", s.Run.State, "stopped "+s.Run.Stopped.At.Format("2006-01-02 15:04"))
	} else {
		line("run", s.Run.State, s.Run.Problem)
	}
	line("issues", fmt.Sprint(s.Issues))
	if len(s.Next) == 0 {
		return
	}
	fmt.Fprintln(out, "\nnext:")
	for _, a := range s.Next {
		fmt.Fprintf(out, "  %s\n", a.Say)
		if a.CLI != "" {
			fmt.Fprintf(out, "    %s\n", a.CLI)
		}
	}
}
