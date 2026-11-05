package cli

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/next/domain/releases"
	"github.com/guilbep/git-board/next/tools/ship"
)

// The release commands (v1's cmd/git-board release.go and deploy.go):
// the runner (next/tools/ship), as the application wires it.

// withRunner runs f with the repository's Runner, which prints its steps
// to cmd's output.
func withRunner(cmd *cobra.Command, f func(ctx context.Context, r *ship.Runner, out io.Writer) error) error {
	b, err := open(cmd, true)
	if err != nil {
		return err
	}
	defer b.Close()
	r, err := b.Runner(cmd.Context())
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	r.Step = func(s ship.Step) {
		if s.State == "done" {
			fmt.Fprintln(out, strings.TrimRight("  "+s.Name+"  "+s.Detail, " "))
		}
	}
	return f(cmd.Context(), r, out)
}

func newRelease() *cobra.Command {
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
	return withRunner(cmd, func(ctx context.Context, r *ship.Runner, out io.Writer) error {
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
		pending, err := ship.Pending(ctx, r.Dir)
		for _, p := range pending {
			fmt.Fprintf(out, "pending %s on %s: cut offline; the next cut, promote, rollback or deploy pushes it\n", p.Name, short(p.Commit))
		}
		return err
	})
}

// envCommit is the commit shown: the targets' when they agree, else the first target's.
func envCommit(e ship.EnvState) string {
	if e.Commit != "" || len(e.Targets) == 0 {
		return e.Commit
	}
	return e.Targets[0].Deployed.Commit
}

func envVersion(e ship.EnvState) string {
	if e.Version != "" || len(e.Targets) == 0 {
		return e.Version
	}
	return e.Targets[0].Deployed.Version
}

func short(c string) string { return c[:min(7, len(c))] }

// envNote is what is notable about e: unadopted, split, or behind the trunk.
func envNote(e ship.EnvState, base string) string {
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
			c := ship.Cut{Final: final, Hotfix: hotfix, Deploy: !noDeploy, Commit: commit}
			if version != "" {
				v, ok := releases.ParseTag("v" + strings.TrimPrefix(version, "v"))
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
			return withRunner(cmd, func(ctx context.Context, r *ship.Runner, out io.Writer) error {
				r.Offline = offline
				t, err := r.Cut(ctx, c)
				if t.Name != "" { // pushed, even when a later step failed
					fmt.Fprintf(out, "cut %s\n", t.Name)
				}
				if errors.Is(err, ship.ErrFetch) {
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
			return withRunner(cmd, func(ctx context.Context, r *ship.Runner, out io.Writer) error {
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
			return withRunner(cmd, func(ctx context.Context, r *ship.Runner, out io.Writer) error {
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
			return withRunner(cmd, func(ctx context.Context, r *ship.Runner, out io.Writer) error {
				ps, err := r.Proposals(ctx)
				if err != nil {
					return err
				}
				for _, s := range sets {
					if err := applySet(r, ps, s); err != nil {
						return err
					}
				}
				var known []ship.Proposal
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
func targetNo(r *ship.Runner, p ship.Proposal) int {
	return 1 + slices.IndexFunc(r.Cfg.Deploy, func(d releases.Target) bool {
		return d.Repo == p.Repo && strings.ReplaceAll(d.Path, "{env}", p.Env) == p.Path
	})
}

// applySet fills in ps the proposal "<env>/<n>=<sha>" names.
func applySet(r *ship.Runner, ps []ship.Proposal, s string) error {
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
			return withRunner(cmd, func(ctx context.Context, r *ship.Runner, out io.Writer) error {
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
			return withRunner(cmd, func(ctx context.Context, r *ship.Runner, out io.Writer) error {
				return r.Yank(ctx, args[0], reason)
			})
		},
	}
	cmd.Flags().StringVar(&reason, "reason", "", "why (required)")
	return cmd
}

// watchInterval is how often deploy --watch fetches: a var for the tests.
var watchInterval = 60 * time.Second

func newDeploy() *cobra.Command {
	var tag string
	var watch bool
	cmd := &cobra.Command{
		Use:   "deploy <env>",
		Short: "Deploy to an environment",
		Long: `Deploys the trunk's head to the environment that follows the trunk, or,
with --release, a release tag to any environment. --watch keeps the followed
environment on the trunk's head: it fetches every minute and deploys when
behind, until interrupted.`,
		Example: `  git-board deploy dev
  git-board deploy dev --watch
  git-board deploy stage --release v1.4.20-rc.2`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			env := args[0]
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			cmd.SetContext(ctx)
			return withRunner(cmd, func(ctx context.Context, r *ship.Runner, out io.Writer) error {
				followed := env == r.Cfg.Followed()
				switch {
				case watch && (!followed || tag != ""):
					return errors.New("--watch is only for the environment that follows the trunk, without --release")
				case tag == "" && !followed:
					return fmt.Errorf("%s doesn't follow the trunk: deploy it a release with --release <tag>", env)
				}
				if err := deployOnce(ctx, r, out, env, tag); err != nil {
					return err
				}
				for watch {
					select {
					case <-ctx.Done():
						return nil
					case <-time.After(watchInterval):
					}
					if err := redeployIfBehind(ctx, r, out, env); err != nil && ctx.Err() == nil {
						fmt.Fprintln(out, "watch:", err) // the next tick tries again
					}
				}
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&tag, "release", "", "the release tag to deploy")
	cmd.Flags().BoolVar(&watch, "watch", false, "keep the followed environment on the trunk's head")
	return cmd
}

// redeployIfBehind deploys the trunk's head to env when it runs less.
func redeployIfBehind(ctx context.Context, r *ship.Runner, out io.Writer, env string) error {
	st, err := r.Status(ctx)
	if err != nil {
		return err
	}
	for _, e := range st.Envs {
		if e.Name == env && len(e.Behind) > 0 {
			return deployOnce(ctx, r, out, env, "")
		}
	}
	return nil
}

// deployOnce deploys tag (else the trunk's head) to env and prints what it runs.
func deployOnce(ctx context.Context, r *ship.Runner, out io.Writer, env, tag string) error {
	if tag == "" {
		if err := r.DeployFollowed(ctx); err != nil {
			return err
		}
	} else {
		tags, err := ship.ReleaseTags(ctx, r.Dir)
		if err != nil {
			return err
		}
		i := slices.IndexFunc(tags, func(t releases.Tag) bool { return t.Name == tag })
		if i < 0 {
			return fmt.Errorf("no release tag %q", tag)
		}
		if err := r.Deploy(ctx, env, ship.Target{Commit: tags[i].Commit, Version: tags[i].Name}); err != nil {
			return err
		}
	}
	st, err := r.Status(ctx)
	if err != nil {
		return err
	}
	for _, e := range st.Envs {
		if e.Name == env {
			fmt.Fprintf(out, "%s runs %s\n", env, e.Version)
		}
	}
	return nil
}
