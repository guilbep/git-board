package app

import (
	"context"
	"fmt"

	"github.com/guilbep/git-board/conf"
	"github.com/guilbep/git-board/domain/cycle"
	"github.com/guilbep/git-board/domain/releases/proof"
	"github.com/guilbep/git-board/tools/ship"
)

// featureTitles are the experimental features as people name them.
var featureTitles = map[string]string{cycle.Feature: "Development cycle", "agents": "Coding agents", "worktrees": "Worktrees"}

// Experimental refuses a feature that is off (ADR 0018), saying how to
// turn it on: class Off, the API's "experimental".
func (c *Config) Experimental(feature string) error {
	if c.On(feature) {
		return nil
	}
	title := featureTitles[feature]
	if title == "" {
		title = feature
	}
	return &Error{Class: Off, Code: "experimental", Err: fmt.Errorf("%s is an experimental feature, off: add %q to [experimental] features in .git-board.toml, or for this clone alone: git config %s %s",
		title, feature, LocalExperimental, feature)}
}

// AtBase is the proof rules as the base branch has them committed: its
// [cycle] tests and checks, and its signers file (ADR 0001, section 5).
// No .git-board.toml there is no checks; no signers file, nil signers.
func AtBase(ctx context.Context, dir, base string) (proof.Base, error) {
	data, err := proof.Show(ctx, dir, base, conf.FileName)
	if err != nil {
		return proof.Base{}, err
	}
	var c cycle.Config
	if data != nil {
		f, err := conf.Parse(conf.FileName, data)
		if err == nil {
			var cfg *Config
			if cfg, err = readConfig(f, nil); err == nil {
				c = cfg.Cycle
			}
		}
		if err != nil {
			return proof.Base{}, fmt.Errorf("%s on %s: %w", conf.FileName, base, err)
		}
	}
	b := proof.Base{Branch: base, Tests: c.Tests, Checks: c.Checks, SignersFile: c.SignersFile()}
	b.Signers, err = proof.Show(ctx, dir, base, b.SignersFile)
	return b, err
}

// BaseRef is the base branch's ref, as the gate reads it: name, else
// origin's default branch, else the first of main, master and trunk
// there is, else main; origin/<name> when origin has it.
func (b *Board) BaseRef(ctx context.Context, name string) string {
	r := b.Code()
	has := func(ref string) bool {
		_, err := ship.Git(ctx, b.Dir, "rev-parse", "--verify", "-q", ref)
		return err == nil
	}
	if name == "" {
		name = r.RemoteHead(ctx, "origin")
	}
	if name == "" {
		name = "main"
		for _, c := range []string{"main", "master", "trunk"} {
			if has("refs/remotes/origin/"+c) || has("refs/heads/"+c) {
				name = c
				break
			}
		}
	}
	if has("refs/remotes/origin/" + name) {
		return "origin/" + name
	}
	return name
}

// Verify is the gate's verdict on a commit (HEAD by default), against the
// checks and the signers committed on base ("": BaseRef of [branches]
// base): v1's git board verify. The cycle's: off, it refuses.
func (b *Board) Verify(ctx context.Context, base, commit string) (sha, baseRef string, g proof.GateResult, err error) {
	if err := b.Config.Experimental(cycle.Feature); err != nil {
		return "", "", g, err
	}
	if commit == "" {
		commit = "HEAD"
	}
	sha, err = ship.Git(ctx, b.Dir, "rev-parse", "--verify", "--end-of-options", commit+"^{commit}")
	if err != nil {
		return "", "", g, err
	}
	if base == "" {
		base = b.BaseRef(ctx, b.Config.Branches.Base)
	}
	rules, err := AtBase(ctx, b.Dir, base)
	if err != nil {
		return sha, base, g, err
	}
	g, err = rules.Gate(ctx, b.Dir, sha)
	return sha, base, g, err
}
