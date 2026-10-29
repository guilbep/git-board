package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"github.com/guilbep/git-board/core/syncer"
	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/store"
)

// The forges alone: GitHub's and GitLab's issues, a translation of the
// board made both ways. No git step: git board pull and push move the
// board itself.

// forgeWay is one way of the forge step: SyncForges, GetForges or
// SendForges.
type forgeWay func(*store.Backend, context.Context, store.ForgeRun) (syncer.Report, error)

func newForgeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "forge",
		Short: "The forges alone: pull, push or sync their issues, with no git step",
		Long: `The forges alone: the issues on GitHub and GitLab, which git board sync
moves between its git pull and its git push. These run that step on its
own, one way or both, with the forges of .git-board.toml, or those found
in the git remotes, whose sync is on.`,
	}
	cmd.AddCommand(
		newForgeWayCmd("pull", "Take the forges' changes, and send nothing",
			`Take the forges' changes: new issues and comments, edits, closings. Nothing
goes to a forge.`, (*store.Backend).GetForges),
		newForgeWayCmd("push", "Send this clone's changes to the forges, and take nothing",
			`Send this clone's changes to the forges, as Send does in Settings › Sync.
Nothing comes back from them. A forge this repository never synced with,
or 50 creations or more on one, gets no issue created without
--allow-create.`, (*store.Backend).SendForges),
		newForgeWayCmd("sync", "Take the forges' changes, then send this clone's",
			`Take the forges' changes, then send this clone's: the forge step of git
board sync, alone. Creations wait for consent as push says.`, (*store.Backend).SyncForges),
	)
	return cmd
}

func newForgeWayCmd(use, short, long string, way forgeWay) *cobra.Command {
	var check, allowCreate bool
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Long:  long,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			b, err := store.OpenCwd()
			if err != nil {
				return err
			}
			defer b.Close()
			forges, left, err := runForges(cmd.Context(), out, b, cfg)
			if err != nil {
				return err
			}
			var on []config.Forge
			for _, f := range forges {
				if f.Sync {
					on = append(on, f)
				}
			}
			switch {
			case len(on) == 0 && len(left) > 0:
				return store.ErrForgesLeftOut
			case len(on) == 0:
				return errors.New("no forge has its sync on")
			}
			say := func(format string, a ...any) { fmt.Fprintf(out, format+"\n", a...) }
			if err := runForgeWay(cmd.Context(), b, cfg, on, way, check, allowCreate, "", say); err != nil {
				return err
			}
			if check {
				say("checked: nothing written")
			} else {
				say("done")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "the dry run: say what it would do, write nothing")
	cmd.Flags().BoolVar(&allowCreate, "allow-create", false, "create issues on a forge never synced with, or 50 or more on one")
	return cmd
}

// runForgeWay runs one way of the forge step on the forges on, each event
// told in a line. merge is the git remote whose fetched copies a check
// counts in first, as the whole sync's check does; "" for none.
func runForgeWay(ctx context.Context, b *store.Backend, cfg *config.Config, on []config.Forge, way forgeWay,
	check, allowCreate bool, merge string, say func(format string, a ...any)) error {
	fs, err := openForges(ctx, on)
	if err != nil {
		return err
	}
	if c, err := syncer.Cut(b.Store()); err == nil && c != nil {
		for _, f := range c.Forges {
			say("  the sync cut %s read %s issues of %s: a pull goes on from there",
				c.At.Format("Jan 2 15:04"), english.Sprintf("%d of %d", f.Read, f.Total), f.Key)
		}
	}
	run := store.ForgeRun{Forges: fs, PublishPrivate: cfg.PublishPrivate, DryRun: check, AllowCreate: allowCreate, Merge: merge}
	run.See = func(e store.SyncEvent) {
		if e.What == "progress" {
			if line := progressLine(e); line != "" {
				say("  %s: %s", e.Forge, line)
			}
			return
		}
		line := e.Text(check, eventIssue(b, e.Issue))
		if e.What == "held" {
			line += " (--allow-create)"
		}
		if line != "" {
			say("  %s: %s", e.Forge, line)
		}
	}
	_, err = way(b, ctx, run)
	return err
}

// longPull is how many issues a pull reads before it says it takes minutes
// (the web's ways.ts says the same).
const longPull = 200

var english = message.NewPrinter(language.English)

// progressLine is a way's line every hundred issues; a long pull's first
// says it takes minutes. "" between.
func progressLine(e store.SyncEvent) string {
	switch {
	case e.Way == "pull" && e.Done == 1 && e.Total >= longPull:
		return english.Sprintf("%d issues to read: this takes several minutes", e.Total)
	case e.Done%100 == 0 || e.Done == e.Total && e.Total >= 100:
		verb := "pulling"
		if e.Way == "push" {
			verb = "pushing"
		}
		return english.Sprintf("%s %d of %d", verb, e.Done, e.Total)
	}
	return ""
}
