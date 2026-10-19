package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"slices"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/internal/release"
)

// watchInterval is how often deploy --watch fetches: a var for the tests.
var watchInterval = 60 * time.Second

func newDeployCmd() *cobra.Command {
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
			return withRunner(cmd, func(ctx context.Context, r *release.Runner, out io.Writer) error {
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
func redeployIfBehind(ctx context.Context, r *release.Runner, out io.Writer, env string) error {
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
func deployOnce(ctx context.Context, r *release.Runner, out io.Writer, env, tag string) error {
	if tag == "" {
		if err := r.DeployFollowed(ctx); err != nil {
			return err
		}
	} else {
		tags, err := release.ReleaseTags(ctx, r.Dir)
		if err != nil {
			return err
		}
		i := slices.IndexFunc(tags, func(t release.Tag) bool { return t.Name == tag })
		if i < 0 {
			return fmt.Errorf("no release tag %q", tag)
		}
		if err := r.Deploy(ctx, env, release.Target{Commit: tags[i].Commit, Version: tags[i].Name}); err != nil {
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
