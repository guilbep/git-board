package app

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/guilbep/git-board/conf"
	"github.com/guilbep/git-board/domain/cycle"
	"github.com/guilbep/git-board/domain/issues"
	"github.com/guilbep/git-board/domain/releases"
	"github.com/guilbep/git-board/forge"
	"github.com/guilbep/git-board/kernel"
	"github.com/guilbep/git-board/tools"
)

// Config is .git-board.toml, each table declared by its domain (design.md,
// Config) and read.
type Config struct {
	File *conf.File
	// SchemaVersion is the file's schema version (conf.File.Version): 1
	// without a schema key, 0 without a file. CurrentSchema is this
	// git-board's.
	SchemaVersion int
	Schema        *issues.Schema
	Views         map[string]issues.View
	Branches      issues.Branches
	Forges        forge.Forges
	Repos         forge.Repos
	Release       releases.Config
	Cycle         cycle.Config
	Tools         tools.Tables
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
	c := &Config{File: f, SchemaVersion: f.Version(), Tools: tools.Declare(f)}
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
// refused (Usage) and nothing is written. An older schema is written
// upgraded to this git-board's (the result says so); a newer one isn't
// written.
func SetConfig(dir, key, literal string) (ConfigWrite, error) {
	path := configPath(dir)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return ConfigWrite{}, fmt.Errorf("no %s here: git board init writes one", conf.FileName)
	}
	if err != nil {
		return ConfigWrite{}, err
	}
	usage := func(err error) error { return &Error{Class: Usage, Err: err} }
	text, from, err := conf.ForWrite(string(data))
	if err != nil {
		return ConfigWrite{From: from}, usage(err)
	}
	w := ConfigWrite{From: from, To: conf.Current}
	table, name, err := conf.SplitKey(text, key)
	if err != nil {
		return w, usage(err)
	}
	out := conf.SetKeys(text, table, []string{name}, map[string]string{name: literal})
	f, err := conf.Parse(path, []byte(out))
	if err == nil {
		_, err = readConfig(f, nil)
	}
	if err != nil {
		return w, usage(fmt.Errorf("%s = %s: %w", key, literal, err))
	}
	for _, k := range f.Unknown() {
		if k == key || strings.HasPrefix(k, key+".") {
			return w, usage(fmt.Errorf("%s: %w", key, conf.ErrUnknownKey))
		}
	}
	if out == text { // a write that changes nothing upgrades nothing
		return ConfigWrite{From: from, To: from}, nil
	}
	return w, writeConfig(path, out)
}

// ConfigWrite is what a write did to the file's schema: From, the version
// it was, To the one it is. From < To: the write upgraded it.
type ConfigWrite struct{ From, To int }

// Upgraded says the write upgraded the file's schema.
func (w ConfigWrite) Upgraded() bool { return w.From < w.To }

// Notice is the line a surface prints once, on the write that upgraded
// the file; "" when it didn't.
func (w ConfigWrite) Notice() string {
	if !w.Upgraded() {
		return ""
	}
	return fmt.Sprintf("%s upgraded from schema %d to %d (git board config schema says what changed)", conf.FileName, w.From, w.To)
}

// CurrentSchema is the schema version this git-board writes: what a
// Config's SchemaVersion is compared with.
const CurrentSchema = conf.Current

// SchemaState is the file's schema against this git-board's: config
// schema, Settings.
type SchemaState struct {
	File    int              `json:"file"`   // the file's version; 0 without a file
	Binary  int              `json:"binary"` // this git-board's, CurrentSchema
	Pending []conf.Migration `json:"pending"`
}

// ConfigSchema is the schema state of the .git-board.toml of dir's
// repository.
func ConfigSchema(dir string) (SchemaState, error) {
	s := SchemaState{Binary: conf.Current, Pending: []conf.Migration{}}
	data, err := os.ReadFile(configPath(dir))
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if s.File, err = conf.Version(string(data)); err != nil {
		return s, fmt.Errorf("parsing %s: %w", conf.FileName, err)
	}
	if s.File < conf.Current {
		s.Pending, _ = conf.Steps(s.File, conf.Current)
	}
	return s, nil
}

// Migrated is what MigrateConfig did, or would do.
type Migrated struct {
	From  int              `json:"from"`
	To    int              `json:"to"`
	Steps []conf.Migration `json:"steps"`
	Diff  string           `json:"diff"` // diff -u of the file
}

// MigrateConfig moves the .git-board.toml of dir's repository to schema
// to, up or down: up when up says so (an upgrade's to is at least the
// file's), down else. dry writes nothing.
func MigrateConfig(dir string, to int, up, dry bool) (Migrated, error) {
	path := configPath(dir)
	usage := func(err error) error { return &Error{Class: Usage, Err: err} }
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Migrated{}, &Error{Class: NotFound, Err: fmt.Errorf("%w: no %s here", errNotFound, conf.FileName)}
	}
	if err != nil {
		return Migrated{}, err
	}
	from, err := conf.Version(string(data))
	if err != nil {
		return Migrated{}, usage(fmt.Errorf("parsing %s: %w", conf.FileName, err))
	}
	m := Migrated{From: from, To: to, Steps: []conf.Migration{}}
	if _, err := conf.Steps(1, to); err != nil {
		return m, usage(err)
	}
	switch {
	case from > conf.Current:
		return m, usage(&conf.NewerError{File: from})
	case up && to < from:
		return m, usage(fmt.Errorf("%s is schema %d: git board config downgrade --to %d goes down", conf.FileName, from, to))
	case !up && to > from:
		return m, usage(fmt.Errorf("%s is schema %d: git board config upgrade --to %d goes up", conf.FileName, from, to))
	}
	out, steps, err := conf.Migrate(string(data), to)
	if err != nil {
		return m, usage(err)
	}
	if steps != nil {
		m.Steps = steps
	}
	m.Diff = conf.Diff(fmt.Sprintf("%s (schema %d)", conf.FileName, from), fmt.Sprintf("%s (schema %d)", conf.FileName, to), string(data), out)
	if dry || out == string(data) {
		return m, nil
	}
	return m, writeConfig(path, out)
}

// writeConfig writes the file, its mode kept.
func writeConfig(path, text string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(text), info.Mode().Perm())
}

// Held says whether the forges' sync keeps a label off the forge of key
// (forge.Forges.Held): on GitHub, a field the Project holds is the
// Project's, by its [fields.X] project map or [github-project].
func (c *Config) Held(key, label string) bool {
	gp, _ := c.Tools.GithubProject.Get()
	inProject := func(field string) bool {
		return len(c.Schema.Fields[field].Project) > 0 || gp.Holds(field)
	}
	return c.Forges.Held(c.Schema, inProject, key, label)
}

// ConfigText is the .git-board.toml of dir's repository, nil when there
// is none, and its version: a hash of its text, which a write expects.
func ConfigText(dir string) ([]byte, string, error) {
	data, err := os.ReadFile(configPath(dir))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, TextVersion(nil), nil
	}
	return data, TextVersion(data), err
}

// TextVersion is a text's version: its SHA-256, short.
func TextVersion(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:12])
}

// CheckConfig is what is wrong with a .git-board.toml's text: it doesn't
// parse, a table is broken, or a key git-board doesn't read. nil when
// nothing is.
func CheckConfig(dir string, text []byte) error {
	f, err := conf.Parse(configPath(dir), text)
	if err == nil {
		_, err = readConfig(f, nil)
	}
	if err != nil {
		return &Error{Class: Usage, Err: err}
	}
	if u := f.Unknown(); len(u) > 0 {
		return &Error{Class: Usage, Err: fmt.Errorf("%s: %w", u[0], conf.ErrUnknownKey)}
	}
	return nil
}

// SaveConfig writes text as dir's .git-board.toml, checked, when the
// file is still at base ("" writes over whatever it is: Stale
// otherwise). With dry, it checks and writes nothing.
func SaveConfig(dir string, text []byte, base string, dry bool) error {
	if err := CheckConfig(dir, text); err != nil {
		return err
	}
	cur, v, err := ConfigText(dir)
	if err != nil {
		return err
	}
	if base != "" && base != v {
		return &Error{Class: Stale, Err: fmt.Errorf("%s: %w", conf.FileName, kernel.ErrStale)}
	}
	if dry || bytes.Equal(cur, text) {
		return nil
	}
	mode := fs.FileMode(0o644)
	if info, err := os.Stat(configPath(dir)); err == nil {
		mode = info.Mode().Perm()
	}
	return os.WriteFile(configPath(dir), text, mode)
}

// SetConfigKeys sets each key of dir's .git-board.toml to its value, a
// whole subtree (conf.SetValue: a table's keys in place, nil removes the
// key and its tables), and answers the file's new text; nothing is
// written unless all are good, the file is still at base ("": whatever
// it is), and not dry.
func SetConfigKeys(dir string, set map[string]any, base string, dry bool) ([]byte, error) {
	cur, _, err := ConfigText(dir)
	if err != nil {
		return nil, err
	}
	text := string(cur)
	for _, key := range slices.Sorted(maps.Keys(set)) {
		// a top table (cycle) is a key here, as a whole: set or removed
		_, isTable := set[key].(map[string]any)
		top := !strings.Contains(key, ".") && (isTable || set[key] == nil) && conf.SimpleName(key)
		if _, _, err := conf.SplitKey(text, key); err != nil && !top {
			return nil, &Error{Class: Usage, Err: err}
		}
		if text, err = conf.SetValue(text, key, set[key]); err != nil {
			return nil, &Error{Class: Usage, Err: fmt.Errorf("%s: %w", key, err)}
		}
	}
	return []byte(text), SaveConfig(dir, []byte(text), base, dry)
}
