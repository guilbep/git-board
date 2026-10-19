package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio"
	"github.com/guilbep/git-board/internal/release"
)

// releaseTools builds the registry, builder and chart tools: a var for the
// tests, which swap in releasetest.NewTools.
var releaseTools = release.Exec

// newRunner is the Runner of the repository in the working directory; the
// func closes its git-bug backend.
func newRunner() (*release.Runner, func(), error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, nil, err
	}
	if !cfg.Release.Enabled() {
		return nil, nil, errors.New("no [release] table with environments in .git-board.toml")
	}
	b, err := gitbugio.OpenCwd()
	if err != nil {
		return nil, nil, err
	}
	dir, err := os.Getwd()
	if err != nil {
		_ = b.Close()
		return nil, nil, err
	}
	r, err := release.New(cfg, dir, b)
	if err != nil {
		_ = b.Close()
		return nil, nil, err
	}
	r.Tools = releaseTools()
	return r, func() { _ = b.Close() }, nil
}

// withRunner runs f with a Runner that prints its steps to cmd's output.
func withRunner(cmd *cobra.Command, f func(ctx context.Context, r *release.Runner, out io.Writer) error) error {
	r, closeB, err := newRunner()
	if err != nil {
		return err
	}
	defer closeB()
	out := cmd.OutOrStdout()
	r.Step = func(s release.Step) {
		if s.State == "done" {
			fmt.Fprintln(out, strings.TrimRight("  "+s.Name+"  "+s.Detail, " "))
		}
	}
	return f(cmd.Context(), r, out)
}

func newReleaseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "release",
		Short: "What runs where, and cut, promote, roll back and adopt releases",
		Long: `With no subcommand, prints what runs in each environment.

The [release] table of .git-board.toml says what the app is: its
environments, images, chart and the config repos holding its overlays.
Tags and release records go to the authority remote; never to a tag you
didn't cut.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error { return runStatus(cmd) },
	}
	cmd.AddCommand(
		&cobra.Command{Use: "status", Short: "Print what runs in each environment", Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error { return runStatus(cmd) }},
		newCutCmd(), newPromoteCmd(), newRollbackCmd(), newAdoptCmd(), newNotesCmd(), newYankCmd(),
	)
	return cmd
}

func runStatus(cmd *cobra.Command) error {
	return withRunner(cmd, func(ctx context.Context, r *release.Runner, out io.Writer) error {
		st, err := r.Status(ctx)
		if err != nil {
			return err
		}
		tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		for _, e := range st.Envs {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", e.Name, cmp.Or(envVersion(e), "-"), cmp.Or(short(envCommit(e)), "-"), strings.TrimSpace(envNote(e, r.Base)))
		}
		if err := tw.Flush(); err != nil {
			return err
		}
		pending, err := release.Pending(ctx, r.Dir)
		for _, p := range pending {
			fmt.Fprintf(out, "pending %s on %s: cut offline; the next cut, promote, rollback or deploy pushes it\n", p.Name, short(p.Commit))
		}
		return err
	})
}

// envCommit is the commit shown: the targets' when they agree, else the first target's.
func envCommit(e release.EnvState) string {
	if e.Commit != "" || len(e.Targets) == 0 {
		return e.Commit
	}
	return e.Targets[0].Deployed.Commit
}

func envVersion(e release.EnvState) string {
	if e.Version != "" || len(e.Targets) == 0 {
		return e.Version
	}
	return e.Targets[0].Deployed.Version
}

func short(c string) string { return c[:min(7, len(c))] }

// envNote is what is notable about e: unadopted, split, or behind the trunk.
func envNote(e release.EnvState, base string) string {
	switch {
	case len(e.Targets) == 0:
		return ""
	case !e.Adopted && !e.Split:
		return "not adopted"
	case e.Split:
		var parts []string
		for _, t := range e.Targets[1:] {
			if t.Deployed.Commit != e.Targets[0].Deployed.Commit {
				parts = append(parts, fmt.Sprintf("%s on %s", t.Name, cmp.Or(t.Deployed.Version, "?")))
			}
		}
		return "split: " + strings.Join(parts, ", ")
	case len(e.Behind) == 1:
		return "1 merge behind " + trunkName(base)
	case len(e.Behind) > 1:
		return fmt.Sprintf("%d merges behind %s", len(e.Behind), trunkName(base))
	}
	return ""
}

func trunkName(base string) string { return cmp.Or(base, "dev") }

func newCutCmd() *cobra.Command {
	var version, commit, notesFile string
	var final, hotfix, noDeploy, offline bool
	cmd := &cobra.Command{
		Use:   "cut",
		Short: "Tag the next candidate (or the final), build it and deploy it",
		Example: `  git-board release cut
  git-board release cut --final
  git-board release cut --version 1.4.20 --no-deploy
  git-board release cut --offline --no-deploy`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c := release.Cut{Final: final, Hotfix: hotfix, Deploy: !noDeploy, Commit: commit}
			if version != "" {
				v, ok := release.ParseTag("v" + strings.TrimPrefix(version, "v"))
				switch {
				case !ok:
					return fmt.Errorf("--version %q: want 1.4.20", version)
				case v.RC != 0:
					return errors.New("--version takes a final version like 1.4.20; the rc number is chosen for you")
				}
				c.Version = v
			}
			if notesFile != "" {
				b, err := os.ReadFile(notesFile)
				if err != nil {
					return err
				}
				c.Notes = string(b)
			}
			return withRunner(cmd, func(ctx context.Context, r *release.Runner, out io.Writer) error {
				r.Offline = offline
				t, err := r.Cut(ctx, c)
				if t.Name != "" { // pushed, even when a later step failed
					fmt.Fprintf(out, "cut %s\n", t.Name)
				}
				if errors.Is(err, release.ErrFetch) {
					err = fmt.Errorf("%w\nno network? git-board release cut --offline --no-deploy tags locally; the next online cut or deploy pushes it", err)
				}
				return err
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&version, "version", "", "the final version to cut, e.g. 1.4.20 (default: the draft's)")
	f.BoolVar(&final, "final", false, "cut the final rather than the next candidate")
	f.BoolVar(&hotfix, "hotfix", false, "may leave the line the last environment runs")
	f.StringVar(&commit, "commit", "", "the commit to cut (default: what the followed environment runs)")
	f.BoolVar(&noDeploy, "no-deploy", false, "tag and build, but deploy nowhere")
	f.StringVar(&notesFile, "notes-file", "", "release notes from this file (default: drafted from the issues)")
	f.BoolVar(&offline, "offline", false, "no network: tag locally, pushed and built by the next online cut, promote, rollback or deploy")
	return cmd
}

func newPromoteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "promote <rc-tag>",
		Short: "Cut the final of a candidate and deploy it to the last environment",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withRunner(cmd, func(ctx context.Context, r *release.Runner, out io.Writer) error {
				t, err := r.Promote(ctx, args[0])
				if t.Name != "" {
					fmt.Fprintf(out, "cut %s\n", t.Name)
				}
				return err
			})
		},
	}
}

func newRollbackCmd() *cobra.Command {
	var reason string
	cmd := &cobra.Command{
		Use:   "rollback <env> <tag>",
		Short: "Make an environment run an earlier release again",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if reason == "" {
				return errors.New("--reason is required")
			}
			return withRunner(cmd, func(ctx context.Context, r *release.Runner, out io.Writer) error {
				return r.Rollback(ctx, args[0], args[1], reason)
			})
		},
	}
	cmd.Flags().StringVar(&reason, "reason", "", "why, kept on the release's record (required)")
	return cmd
}

func newAdoptCmd() *cobra.Command {
	var yes bool
	var sets []string
	cmd := &cobra.Command{
		Use:   "adopt",
		Short: "Mark what runs today as a release's commit, so it can be deployed over",
		Long: `Prints, for each deploy target not adopted yet, the commit it is thought to
run. --yes writes them. --set <env>/<n>=<sha> fills one that is unknown,
n being the target's place in [release] deploy, from 1.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withRunner(cmd, func(ctx context.Context, r *release.Runner, out io.Writer) error {
				ps, err := r.Proposals(ctx)
				if err != nil {
					return err
				}
				for _, s := range sets {
					if err := applySet(r, ps, s); err != nil {
						return err
					}
				}
				var known []release.Proposal
				for _, p := range ps {
					n := targetNo(r, p)
					fmt.Fprintf(out, "%s/%d  runs %s  %s (%s)\n", p.Env, n, p.Runs, cmp.Or(short(p.Commit), "-"), cmp.Or(p.From, "unknown"))
					if p.Commit != "" {
						known = append(known, p)
					}
				}
				switch {
				case len(ps) == 0:
					fmt.Fprintln(out, "everything is adopted")
				case !yes:
					fmt.Fprintln(out, "nothing written; --yes adopts the ones with a commit, --set <env>/<n>=<sha> fills an unknown one")
				default:
					if err := r.Adopt(ctx, known); err != nil {
						return err
					}
					if n := len(ps) - len(known); n > 0 {
						fmt.Fprintf(out, "%d not adopted: no commit known — use --set <env>/<n>=<sha>\n", n)
					}
				}
				return nil
			})
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "write the proposals")
	cmd.Flags().StringArrayVar(&sets, "set", nil, "<env>/<n>=<sha>: the commit target n of env runs")
	return cmd
}

// targetNo is p's place in [release] deploy, from 1.
func targetNo(r *release.Runner, p release.Proposal) int {
	return 1 + slices.IndexFunc(r.Cfg.Deploy, func(d config.Target) bool {
		return d.Repo == p.Repo && strings.ReplaceAll(d.Path, "{env}", p.Env) == p.Path
	})
}

// applySet fills in ps the proposal "<env>/<n>=<sha>" names.
func applySet(r *release.Runner, ps []release.Proposal, s string) error {
	key, sha, ok := strings.Cut(s, "=")
	env, n, ok2 := strings.Cut(key, "/")
	i, err := strconv.Atoi(n)
	if !ok || !ok2 || err != nil || sha == "" || i < 1 || i > len(r.Cfg.Deploy) {
		return fmt.Errorf("--set %q: want <env>/<n>=<sha>, n from 1 to %d", s, len(r.Cfg.Deploy))
	}
	for j := range ps {
		if d := r.Cfg.Deploy[i-1]; ps[j].Env == env && ps[j].Repo == d.Repo && ps[j].Path == strings.ReplaceAll(d.Path, "{env}", env) {
			ps[j].Commit, ps[j].From = sha, "set"
			return nil
		}
	}
	return fmt.Errorf("--set %q: %s's target %d isn't waiting to be adopted", s, env, i)
}

func newNotesCmd() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "notes",
		Short: "Print the release notes, newest first",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withRunner(cmd, func(ctx context.Context, r *release.Runner, out io.Writer) error {
				md, err := r.Notes(ctx, all)
				fmt.Fprint(out, md)
				return err
			})
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "candidates too")
	return cmd
}

func newYankCmd() *cobra.Command {
	var reason string
	cmd := &cobra.Command{
		Use:   "yank <tag>",
		Short: "Mark a release as not to be used, with the reason",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if reason == "" {
				return errors.New("--reason is required")
			}
			return withRunner(cmd, func(ctx context.Context, r *release.Runner, out io.Writer) error {
				return r.Yank(ctx, args[0], reason)
			})
		},
	}
	cmd.Flags().StringVar(&reason, "reason", "", "why (required)")
	return cmd
}
