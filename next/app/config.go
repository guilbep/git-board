package app

import (
	"fmt"
	"slices"
	"strings"

	"github.com/guilbep/git-board/next/conf"
	"github.com/guilbep/git-board/next/domain/cycle"
	"github.com/guilbep/git-board/next/domain/issues"
	"github.com/guilbep/git-board/next/domain/releases"
	"github.com/guilbep/git-board/next/forge"
	"github.com/guilbep/git-board/next/tools"
)

// Config is .git-board.toml, each table declared by its domain (design.md,
// Config) and read.
type Config struct {
	File     *conf.File
	Schema   *issues.Schema
	Views    map[string]issues.View
	Branches issues.Branches
	Forges   forge.Forges
	Repos    forge.Repos
	Release  releases.Config
	Cycle    cycle.Config
	Tools    tools.Tables
	// Features are [experimental] features: the file's, or this clone's
	// board.experimental when it sets one.
	Features []string
}

// LocalExperimental is the git config key that replaces [experimental]
// features in one clone.
const LocalExperimental = "board.experimental"

// Experimental are the features behind the switch (ADR 0018).
var Experimental = []string{cycle.Feature, "agents", "worktrees"}

type experimental struct {
	Features []string `toml:"features"`
}

// ReadConfig declares every domain's table on f and reads them, as v1's
// config does: a file v1 refuses is refused, with v1's message (the first
// problem). local is the clone's board.experimental, nil when unset.
func ReadConfig(f *conf.File, local []string) (*Config, error) {
	tables := issues.Declare(f)
	ft := forge.Declare(f)
	rel := releases.Declare(f)
	cyc := cycle.Declare(f)
	exp := conf.Declare(f, conf.Table[experimental]{Name: "experimental"})
	c := &Config{File: f, Tools: tools.Declare(f)}
	var probs []conf.Problem
	add := func(p []conf.Problem) { probs = append(probs, p...) }
	var p []conf.Problem
	c.Schema, p = tables.Schema()
	add(p)
	c.Views, p = tables.Views()
	add(p)
	c.Branches, p = tables.Branches()
	add(p)
	c.Repos, p = ft.Repos()
	add(p)
	e, p := exp.Get()
	add(p)
	c.Features = e.Features
	if local != nil {
		c.Features = local
	}
	var statuses []string
	if s := c.Schema; s != nil {
		statuses = s.Fields["status"].Options
	}
	var err error
	// the cycle is experimental: its table breaks the file only while
	// the team, or this clone, turned it on; off, it is set aside
	if c.Cycle, err = cyc.Get(statuses); err != nil {
		if slices.Contains(e.Features, cycle.Feature) || slices.Contains(local, cycle.Feature) {
			add([]conf.Problem{{Table: "cycle", Err: err}})
		}
		c.Features = slices.DeleteFunc(slices.Clone(c.Features), func(s string) bool { return s == cycle.Feature })
	}
	c.Release, p = rel.Get()
	add(p)
	c.Forges, p = ft.Forges(c.Schema)
	add(p)
	for _, h := range []interface{ Problems() []conf.Problem }{c.Tools.GithubProject, c.Tools.Uploads, c.Tools.Template} {
		add(h.Problems())
	}
	if len(probs) > 0 {
		return nil, fmt.Errorf("parsing %s: %w", f.Path(), probs[0])
	}
	return c, nil
}

// On says an experimental feature is on.
func (c *Config) On(feature string) bool { return slices.Contains(c.Features, feature) }

// SplitFeatures reads board.experimental's comma list.
func SplitFeatures(s string) []string {
	list := []string{}
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == '\n' || r == '\t' }) {
		list = append(list, strings.ToLower(f))
	}
	return list
}
