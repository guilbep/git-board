package config

import (
	"fmt"
	"os/exec"
	"slices"
	"strings"
)

// The work-in-progress features stay behind a switch, off by default, so
// git-board presents its core: your issues, kept in git, whatever the
// forge. The team turns one on in .git-board.toml:
//
//	[experimental]
//	features = ["cycle", "agents", "worktrees"]
//
// and one person trying it alone in their clone's git config, a comma
// list that replaces the file's there: git config board.experimental cycle.

// The experimental features' names.
const (
	FeatureCycle     = "cycle"     // the development cycle: steps, gates, the Work page, git board verify
	FeatureAgents    = "agents"    // coding agents on an issue, and the AI's suggestions
	FeatureWorktrees = "worktrees" // wtx: a worktree, a tmux session and an agent per branch
)

// LocalExperimental is the git config key that replaces [experimental]
// features in one clone.
const LocalExperimental = "board.experimental"

// ExperimentalFeature is one feature the switch holds, as Settings lists it.
type ExperimentalFeature struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// ExperimentalFeatures are the features behind the switch, in the order
// Settings lists them.
var ExperimentalFeatures = []ExperimentalFeature{
	{FeatureCycle, "Development cycle", "Each issue walks the project's steps, from intent to sign-off, on its own page; the Work page; git board verify."},
	{FeatureAgents, "Coding agents", "Start or brief a coding agent on an issue from its page, and let an AI suggest an issue's risk."},
	{FeatureWorktrees, "Worktrees", "Settings › Worktrees writes wtx.toml: a worktree, a tmux session and an agent per branch."},
}

// experimentalTable is the [experimental] table.
type experimentalTable struct {
	Features []string `toml:"features"`
}

// Experimental tells whether a work-in-progress feature is on: this
// clone's board.experimental when set, else the file's [experimental]
// features. A [cycle] table that doesn't check out keeps the cycle off.
func (c *Config) Experimental(name string) bool {
	if c == nil {
		return false
	}
	if name == FeatureCycle && c.cycleErr != nil {
		return false
	}
	list := c.Features
	if c.local != nil {
		list = *c.local
	}
	return slices.Contains(list, name)
}

// ExperimentalOn are the features on, in ExperimentalFeatures' order;
// never nil.
func (c *Config) ExperimentalOn() []string {
	on := []string{}
	for _, f := range ExperimentalFeatures {
		if c.Experimental(f.Name) {
			on = append(on, f.Name)
		}
	}
	return on
}

// Local is this clone's board.experimental, and whether it sets one.
func (c *Config) Local() ([]string, bool) {
	if c == nil || c.local == nil {
		return nil, false
	}
	return *c.local, true
}

// UseLocal reads board.experimental from the git config dir sees: when
// set, its list replaces the file's features in this clone.
func (c *Config) UseLocal(dir string) {
	out, err := exec.Command("git", "-C", dir, "config", "--get", LocalExperimental).Output()
	if err != nil {
		c.local = nil // unset (git exits 1), or no git: the file's
		return
	}
	c.SetLocal(splitFeatures(string(out)))
}

// SetLocal makes list this clone's features, as board.experimental would.
func (c *Config) SetLocal(list []string) {
	c.local = &list
}

// splitFeatures reads a comma list: "cycle, agents" or "cycle,agents".
func splitFeatures(s string) []string {
	list := []string{}
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == '\n' || r == '\t' }) {
		list = append(list, strings.ToLower(f))
	}
	return list
}

// ExperimentalOff is the refusal of a feature that is off, naming the
// switch that turns it on.
func ExperimentalOff(name string) error {
	title := name
	for _, f := range ExperimentalFeatures {
		if f.Name == name {
			title = f.Title
		}
	}
	return fmt.Errorf("%s is an experimental feature, off: add %q to [experimental] features in .git-board.toml, or for this clone alone: git config %s %s",
		title, name, LocalExperimental, name)
}

// experimentalWarnings are Check's: a name no feature has, and a [cycle]
// table set aside while the cycle is off.
func (c *Config) experimentalWarnings() []string {
	var out []string
	for _, f := range c.Features {
		if !slices.ContainsFunc(ExperimentalFeatures, func(e ExperimentalFeature) bool { return e.Name == f }) {
			out = append(out, fmt.Sprintf("experimental.features: %q isn't a feature (cycle, agents, worktrees)", f))
		}
	}
	if c.cycleErr != nil {
		out = append(out, fmt.Sprintf("%v: ignored while the cycle is off ([experimental] features)", c.cycleErr))
	}
	return out
}

// SetExperimentalText is text with [experimental] features set to list,
// in ExperimentalFeatures' order; no table when list is empty. The file's
// other tables stay.
func SetExperimentalText(text string, list []string) string {
	var on []string
	for _, f := range ExperimentalFeatures {
		if slices.Contains(list, f.Name) {
			on = append(on, f.Name)
		}
	}
	if len(on) == 0 {
		if !strings.Contains(text, "[experimental]") {
			return text
		}
		return withoutTable(text, "[experimental]")
	}
	return upsertTable(text, "[experimental]", "[experimental]\nfeatures = "+tomlArray(on)+"\n")
}
