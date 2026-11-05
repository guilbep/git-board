package app

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
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
	c, err := readConfig(f, local)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", f.Path(), err)
	}
	return c, nil
}

func readConfig(f *conf.File, local []string) (*Config, error) {
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
		return nil, probs[0]
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

// errNotFound is a key not set: exit code 3, as v1.
var errNotFound = errors.New("not found")

// ConfigPath is the .git-board.toml of the repository holding dir.
func ConfigPath(dir string) string { return configPath(dir) }

// GetConfig is a key's value in the .git-board.toml of dir's
// repository, as TOML writes it.
func GetConfig(dir, key string) (string, error) {
	data, err := os.ReadFile(configPath(dir))
	if errors.Is(err, fs.ErrNotExist) {
		return "", &Error{Class: NotFound, Err: fmt.Errorf("%w: no %s here", errNotFound, conf.FileName)}
	}
	if err != nil {
		return "", err
	}
	v, err := conf.GetKey(string(data), key)
	if errors.Is(err, conf.ErrNoKey) {
		return "", &Error{Class: NotFound, Err: fmt.Errorf("%w: %s is not set in %s", errNotFound, key, conf.FileName)}
	}
	return v, err
}

// SetConfig sets a key of the .git-board.toml of dir's repository to a
// TOML literal (conf.Value), or removes it for "": its line alone. A key
// git-board doesn't read, or a value that leaves the file broken, is
// refused (Usage) and nothing is written.
func SetConfig(dir, key, literal string) error {
	path := configPath(dir)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("no %s here: git board init writes one", conf.FileName)
	}
	if err != nil {
		return err
	}
	usage := func(err error) error { return &Error{Class: Usage, Err: err} }
	table, name, err := conf.SplitKey(string(data), key)
	if err != nil {
		return usage(err)
	}
	out := conf.SetKeys(string(data), table, []string{name}, map[string]string{name: literal})
	f, err := conf.Parse(path, []byte(out))
	if err == nil {
		_, err = readConfig(f, nil)
	}
	if err != nil {
		return usage(fmt.Errorf("%s = %s: %w", key, literal, err))
	}
	for _, k := range f.Unknown() {
		if k == key || strings.HasPrefix(k, key+".") {
			return usage(fmt.Errorf("%s: %w", key, conf.ErrUnknownKey))
		}
	}
	if out == string(data) {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(out), info.Mode().Perm())
}

// projectFields are the fields the GitHub Project bridge keeps in step
// with its Project, once [github-project] names one.
var projectFields = []string{"status", "prio", "mod", "est", "sprint", "moscow", "milestone"}

// Held says whether the forges' sync keeps a label off the forge of key
// (forge.Forges.Held): on GitHub, a field the Project holds is the
// Project's, by its [fields.X] project map or [github-project].
func (c *Config) Held(key, label string) bool {
	gp, _ := c.Tools.GithubProject.Get()
	inProject := func(field string) bool {
		return len(c.Schema.Fields[field].Project) > 0 || gp.Number != 0 && slices.Contains(projectFields, field)
	}
	return c.Forges.Held(c.Schema, inProject, key, label)
}
