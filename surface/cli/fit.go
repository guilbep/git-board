package cli

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/tools/ghproject"
)

// git board project fit (docs/prd/config-fit.md): what .git-board.toml
// lacks for the Project the team already has, as a diff, and the
// questions only a person answers.

func newProjectFit() *cobra.Command {
	var apply bool
	var maps, addKinds, unmapped []string
	var limit int
	cmd := &cobra.Command{
		Use:   "fit",
		Short: "Suggest the changes to .git-board.toml that fit the Project, and write them with --apply",
		Long: `Check the Project, and say what .git-board.toml lacks for it: an option of
a status the Project has, the kind a Category names, a sprint's dates. The
changes git-board derives with no guess are shown as the diff --apply
writes. The others are questions, each with the flag that settles it.

A change adds: it never removes an option, a kind, a sprint or a line of
yours. Without --apply nothing is written. With --apply and a terminal,
each open question is asked; a question left open writes nothing.`,
		Example: `  git board project fit
  git board project fit --apply --map 'status:QA Validation=qa' --map category:TASK=feat
  git board project fit --apply --add-kind category:DATA --unmapped category:SPECS`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			answers := map[string]string{}
			for _, m := range maps {
				i := strings.LastIndexByte(m, '=')
				if i <= 0 {
					return usageError{fmt.Errorf("--map %q: <field>:<the Project's name>=<value>", m)}
				}
				answers[m[:i]] = "map=" + m[i+1:]
			}
			for _, id := range addKinds {
				answers[id] = ghproject.AnswerAddKind
			}
			for _, id := range unmapped {
				answers[id] = ghproject.AnswerUnmapped
			}
			b, err := open(cmd, true)
			if err != nil {
				return err
			}
			defer b.Close()
			r, err := b.ProjectRun(cmd.Context(), ghproject.Both, false, limit)
			if err != nil {
				return err
			}
			if len(r.Fit) == 0 {
				fmt.Fprintf(out, "%s fits the Project %s: nothing to change\n", ".git-board.toml", r.Project)
				return nil
			}
			if apply && isTerminal(cmd.InOrStdin()) {
				askFit(cmd, r.Fit, answers)
			}
			f, err := b.Fit(r, nil, answers, !apply)
			if err != nil {
				return err
			}
			fmt.Fprint(out, f.Diff)
			for _, p := range f.Open {
				fmt.Fprintf(out, "? %s\n", p.Says)
				if p.Note != "" {
					fmt.Fprintf(out, "    %s\n", p.Note)
				}
				for _, flag := range fitFlags(p) {
					fmt.Fprintf(out, "    %s\n", flag)
				}
			}
			words := ghproject.FitWords(f.Changes, f.Clears)
			if len(f.Open) > 0 {
				words += fmt.Sprintf(", %d question(s) open", len(f.Open))
			}
			if !apply {
				fmt.Fprintf(out, "%s\nchecked: nothing written (--apply writes it)\n", words)
				return nil
			}
			fmt.Fprintf(out, "wrote %s: %s\n", ".git-board.toml", words)
			return nil
		},
	}
	cmd.Flags().BoolVar(&apply, "apply", false, "write the changes to .git-board.toml")
	cmd.Flags().StringArrayVar(&maps, "map", nil, "settle a question: category:TASK=feat, 'status:QA Validation=qa'")
	cmd.Flags().StringArrayVar(&addKinds, "add-kind", nil, "settle a Category as a new kind: category:DATA")
	cmd.Flags().StringArrayVar(&unmapped, "unmapped", nil, "leave a value of the Project unmapped: category:SPECS")
	cmd.Flags().IntVar(&limit, "limit", ghproject.DefaultLimit, "max Project items to read")
	return cmd
}

// fitFlags are the flags that settle a question, one a line.
func fitFlags(p ghproject.Proposal) []string {
	id := p.ID
	if strings.ContainsAny(id, " '\"") {
		id = "'" + strings.ReplaceAll(id, "'", `'\''`) + "'"
	}
	var values []string
	var out []string
	for _, c := range p.Choices {
		switch v, mapped := strings.CutPrefix(c, "map="); {
		case mapped:
			values = append(values, v)
		case c == ghproject.AnswerAddKind:
			out = append(out, "--add-kind "+id)
		case c == ghproject.AnswerUnmapped:
			out = append(out, "--unmapped "+id)
		}
	}
	if len(values) > 0 {
		quoted := strings.TrimSuffix(id, "'")
		end := id[len(quoted):]
		out = append([]string{fmt.Sprintf("--map %s=<%s>%s", quoted, strings.Join(values, "|"), end)}, out...)
	}
	return out
}

// askFit asks each open question at the terminal, and records the
// answers: a value (or map=<value>), add-kind, unmapped; an empty line
// leaves the question open.
func askFit(cmd *cobra.Command, ps []ghproject.Proposal, answers map[string]string) {
	in, errs := bufio.NewReader(cmd.InOrStdin()), cmd.ErrOrStderr()
	for _, p := range ps {
		if _, done := answers[p.ID]; done || p.Ticked {
			continue
		}
		fmt.Fprintf(errs, "%s\n  %s\n  (empty: leave it open) > ", p.Says, strings.Join(p.Choices, ", "))
		line, err := in.ReadString('\n')
		a := strings.TrimSpace(line)
		if a != "" {
			if a != ghproject.AnswerAddKind && a != ghproject.AnswerUnmapped && !strings.HasPrefix(a, "map=") {
				a = "map=" + a
			}
			answers[p.ID] = a
		}
		if err == io.EOF {
			return
		}
	}
}
