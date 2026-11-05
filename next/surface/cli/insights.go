package cli

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/next/app"
)

func newInsights() *cobra.Command {
	var field, view, me string
	var days, weeks int
	var js *jsonOut
	cmd := &cobra.Command{
		Use:   "insights",
		Short: "The Insights page in the terminal: flow, throughput, cycle time, burn-up",
		Long: `What the web UI's Insights page shows, as text: the issues' history over
the last --days (90), replayed for --field (status).

  flow        the open issues by the field's value, today and --days ago
  throughput  the issues opened and closed each week, the last --weeks
  cycle time  how long issues take from started (or opened) to closed
  burn-up     the current sprint's issues, and how many are closed

--view takes a view's issues alone, open and closed. --json prints what
the page reads (GET /api/insights).`,
		Example: `  git board insights
  git board insights --days 30 --field prio
  git board insights --view backlog --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if days <= 0 || days > 730 {
				return usageError{fmt.Errorf("--days %d: 1 to 730", days)}
			}
			b, err := open(cmd, false)
			if err != nil {
				return err
			}
			defer b.Close()
			rep, err := b.Insights(cmd.Context(), app.InsightsReq{Field: field, Days: days, View: view, Me: resolveMe(me)})
			if err != nil {
				return err
			}
			if js.on() {
				return js.write(cmd.OutOrStdout(), rep)
			}
			printInsights(cmd.OutOrStdout(), rep, weeks)
			return nil
		},
	}
	cmd.Flags().StringVar(&field, "field", "status", "the field the flow is drawn for")
	cmd.Flags().IntVar(&days, "days", 90, "how many days back")
	cmd.Flags().IntVar(&weeks, "weeks", 8, "how many weeks the throughput lists")
	cmd.Flags().StringVar(&view, "view", "", "a view's issues alone (.git-board.toml [views])")
	js = addJSON(cmd, "print the report as JSON, as the page reads it", jsonFields(app.InsightsReport{}), true)
	meFlag(cmd, &me)
	return cmd
}

func printInsights(out io.Writer, r app.InsightsReport, weeks int) {
	for _, k := range []string{"flow", "throughput", "cycle", "burnup"} {
		if h := r.Headlines[k]; h != "" {
			fmt.Fprintln(out, h)
		}
	}
	if n := len(r.Days); n > 0 {
		fmt.Fprintf(out, "\nopen issues by %s\n", r.Field)
		w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
		fmt.Fprintf(w, "%s\t%s\t%s\n", strings.ToUpper(r.Field), r.Days[0], r.Days[n-1])
		for _, v := range r.Values {
			fmt.Fprintf(w, "%s\t%d\t%d\n", v, r.Open[v][0], r.Open[v][n-1])
		}
		w.Flush()
	}
	if len(r.Weeks) > 0 {
		fmt.Fprintln(out, "\nthroughput")
		w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "WEEK OF\tOPENED\tCLOSED")
		list := r.Weeks
		if weeks > 0 && len(list) > weeks {
			list = list[len(list)-weeks:]
		}
		for _, wk := range list {
			fmt.Fprintf(w, "%s\t%d\t%d\n", wk.Start, wk.Opened, wk.Closed)
		}
		w.Flush()
	}
	if c := r.Cycle; len(c.Days) > 0 {
		fmt.Fprintf(out, "\ncycle time, from %s: median %.1f days, 85%% within %.1f days, %d issues\n", c.From, c.Median, c.P85, len(c.Days))
	}
	if b := r.Burnup; b != nil && len(b.Days) > 0 {
		last := len(b.Days) - 1
		fmt.Fprintf(out, "\nburn-up of %s (%s to %s): %d of %d closed\n", b.Title, b.Start, b.End, b.Done[last], b.Scope[last])
	}
}
