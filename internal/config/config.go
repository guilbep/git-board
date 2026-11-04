// Package config parses .git-board.toml: the issue domain's schema
// (core/fields, which resolves the sprint aliases) and the application's
// tables. Pure logic only — no I/O beyond reading the file, no printing.
package config

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/BurntSushi/toml"
	"golang.org/x/text/unicode/norm"

	"github.com/guilbep/git-board/core/fields"
	"github.com/guilbep/git-board/core/proof"
	"github.com/guilbep/git-board/core/release"
)

// View is one [views.<name>] table.
type View struct {
	Layout string `toml:"layout" json:"layout"`
	Filter string `toml:"filter" json:"filter"`
	Group  string `toml:"group" json:"group"`
	Sort   string `toml:"sort" json:"sort"`
	// Columns a table shows, in order (id, title, is, a field, links,
	// other); none: the default ones.
	Columns []string `toml:"columns" json:"columns,omitempty"`
	// Lanes a board shows, in order: values of the group field ("(none)"
	// for the issues without one); none: every value.
	Lanes []string `toml:"lanes" json:"lanes,omitempty"`
	// Limits caps a lane's cards (work in progress): in-progress = 5.
	Limits map[string]int `toml:"limits" json:"limits,omitempty"`
}

// reposTable mirrors the [repos] table shape (`list = [...]`).
type reposTable struct {
	List      []string `toml:"list"`
	Assignees string   `toml:"assignees"`
}

// GithubProject is the [github-project] table: which GitHub Projects (v2)
// board `bridge pull` reads from by default, so --owner/--project don't
// need retyping on every run. Set via `git board project configure`.
type GithubProject struct {
	Owner  string `toml:"owner" json:"owner"`
	Number int    `toml:"number" json:"number"`
	// Write lets `bridge sync` write git-board's changes to the Project.
	// Off by default: the Project is only read.
	Write bool `toml:"write" json:"write"`
}

// raw mirrors the on-disk TOML shape for decoding.
type raw struct {
	Repos         reposTable              `toml:"repos"`
	Fields        map[string]fields.Field `toml:"fields"`
	Sprints       fields.Sprints          `toml:"sprints"`
	Views         map[string]View         `toml:"views"`
	GithubProject GithubProject           `toml:"github-project"`
	Uploads       Uploads                 `toml:"uploads"`
	Branches      Branches                `toml:"branches"`
	Roles         map[string]string       `toml:"roles"`
	Cycle         Cycle                   `toml:"cycle"`
	Template      TemplateRef             `toml:"template"`
	Release       release.Config          `toml:"release"`
	Labels        fields.Labels           `toml:"labels"`
	Experimental  experimentalTable       `toml:"experimental"`
	// Forges holds publish-private, discover and a table per forge
	// (forges.go).
	Forges map[string]toml.Primitive `toml:"forges"`
}

// checkLabels: a listed label is plain (prio:p1 is a field's value), not
// a kind, and the colors and descriptions are the list's own.
func checkLabels(l fields.Labels, kinds map[string]string) error {
	for _, name := range l.List {
		switch {
		case strings.TrimSpace(name) == "":
			return errors.New("[labels] list: an empty label")
		case strings.Contains(name, ":"):
			return fmt.Errorf("[labels] list: %q is a field's value (name:value): give that field the option instead", name)
		case kinds[name] != "":
			return fmt.Errorf("[labels] list: %q is a kind, in [branches] kinds already", name)
		}
	}
	for name, color := range l.Colors {
		if !slices.Contains(l.List, name) {
			return fmt.Errorf("[labels] colors: %q isn't in the list", name)
		}
		c := normColor(color)
		if !slices.Contains(Palette, c) && !isHex(c) {
			return fmt.Errorf("[labels] colors: %q for %q isn't a color: %s, or #rrggbb", color, name, strings.Join(Palette, ", "))
		}
		l.Colors[name] = c
	}
	for name, d := range l.Descriptions {
		if !slices.Contains(l.List, name) {
			return fmt.Errorf("[labels] descriptions: %q isn't in the list", name)
		}
		if n := utf8.RuneCountInString(d); n > 100 {
			return fmt.Errorf("[labels] descriptions: %q's is %d characters, GitHub takes 100 at most", name, n)
		}
	}
	return nil
}

// LabelColors are the list's colors: the file's, else the palette's in
// turn, as a field's values get theirs.
func (c *Config) LabelColors() map[string]string {
	res := map[string]string{}
	i := 0
	for _, l := range c.Labels.List {
		if col := c.Labels.Colors[l]; col != "" {
			res[l] = col
			continue
		}
		res[l] = Palette[i%len(Palette)]
		i++
	}
	return res
}

// Uploads is the [uploads] table: where files attached to issues go. The
// keys stay out of the file: ACCESS_KEY_ID, ACCESS_KEY_SECRET and
// BUCKET_NAME in the environment (internal/uploads).
type Uploads struct {
	Endpoint  string `toml:"endpoint" json:"endpoint"`     // S3, default https://s3.epfl.ch
	PublicURL string `toml:"public_url" json:"public_url"` // where the bucket is served, default https://enacit4r-cdn.epfl.ch
	Prefix    string `toml:"prefix" json:"prefix"`         // default git-board/<repository>
	Region    string `toml:"region" json:"region"`         // default us-east-1
}

// Branches is the [branches] table: the branch an issue suggests
// (internal/branches).
type Branches struct {
	Base string `toml:"base" json:"base"` // default: the remote's default branch, else main, master or trunk
	Name string `toml:"name" json:"name"` // {kind}, {number}, {n}, {slug}; default "{number}/{n}-{slug}"
	// Kinds maps an issue's kind (its kind: field, or a flat label listed
	// here) to the {kind} of its branch: the kind itself, bug = "bug"
	// (checkKindWords).
	Kinds map[string]string `toml:"kinds" json:"kinds"`
}

// checkKindWords: a branch starts with its kind's label, always, so a
// kind's word is the kind itself (bug = "bug"); bug = "fix" is refused at
// its line, with the fix.
func checkKindWords(data []byte, kinds map[string]string) error {
	for _, k := range slices.Sorted(maps.Keys(kinds)) {
		w := kinds[k]
		if w == k {
			continue
		}
		at := regexp.MustCompile(`(?m)^.*\b` + regexp.QuoteMeta(k) + `\s*=\s*"` + regexp.QuoteMeta(w) + `"`).FindIndex(data)
		line := ""
		if at != nil {
			line = fmt.Sprintf("line %d: ", bytes.Count(data[:at[0]], []byte("\n"))+1)
		}
		return fmt.Errorf(`%s[branches] kinds: %s = %q: a branch starts with its kind's label, write %s = %q (its branches start %s/)`, line, k, w, k, k, k)
	}
	return nil
}

// Kind is an issue's kind: its kind: field, else the first of its flat
// labels that Kinds lists. The label, not its branch's: bug, not fix.
func (b Branches) Kind(labels []string) string {
	for _, l := range labels {
		if v, ok := strings.CutPrefix(l, "kind:"); ok {
			return v
		}
	}
	for _, l := range slices.Sorted(slices.Values(labels)) {
		if _, ok := b.Kinds[l]; ok {
			return l
		}
	}
	return ""
}

// Config is the parsed .git-board.toml.
type Config struct {
	Repos []string
	// Assignees is how the forges hold assignees: "native" (their
	// assignees, the default) or "labels" (assignee:<login> labels). Every
	// clone must agree, so it lives here, not in a clone's git config.
	Assignees string
	// Schema is the issue domain's part of the file: [fields], [sprints],
	// [roles] (roles.go) and [labels].
	fields.Schema
	Views         map[string]View
	viewOrder     []string // ViewNames
	GithubProject GithubProject
	Uploads       Uploads
	Branches      Branches
	// Cycle is the [cycle] table: the development cycle's stages and
	// gates (cycle.go).
	Cycle Cycle
	// Release is the [release] table (release.go).
	Release release.Config
	// Forges are the [forges.<name>] tables, by name (forges.go).
	Forges []Forge
	// PublishPrivate lets what was pulled from a forge project that isn't
	// public go to one that is: [forges] publish-private.
	PublishPrivate bool
	// Discover finds the forges in the git remotes: no [forges.<name>]
	// table, and no [forges] discover = false.
	Discover bool
	// Template is the [template] table: the template the file came from,
	// for lint to list the drift.
	Template TemplateRef
	// Unknown is the file's keys git-board doesn't read, a misspelled one
	// above all (Load): each is its zero value, so lint names them.
	Unknown []string
	// Features are [experimental] features, the work-in-progress features
	// the team turned on; a clone's git config board.experimental
	// replaces them (experimental.go, Experimental).
	Features []string
	// local is board.experimental, when this clone's git config sets it
	// (UseLocal); cycleErr what [cycle] fails, kept while the cycle is off
	local    *[]string
	cycleErr error
}

// IssueSchema is the schema the core's issue domain reads (core/fields:
// assignments, filters, the inbox): the file's, nil for no config.
func (c *Config) IssueSchema() *fields.Schema {
	if c == nil {
		return nil
	}
	return &c.Schema
}

// TemplateRef names a built-in template and its version (templates/).
type TemplateRef struct {
	Name    string `toml:"name"`
	Version int    `toml:"version"`
}

// FileName is the config's name, at the root of the repository.
const FileName = ".git-board.toml"

// Path is the config of the repository holding the current directory: the
// nearest FileName up to the repository's root (the directory holding
// .git), else where it goes at that root — so git-board reads the same
// config from any subdirectory, and doesn't quietly start without one.
func Path() string {
	dir, err := os.Getwd()
	if err != nil {
		return FileName
	}
	return PathIn(dir)
}

// PathIn is Path from dir rather than the current directory.
func PathIn(dir string) string {
	for {
		p := filepath.Join(dir, FileName)
		if _, err := os.Stat(p); err == nil {
			return p
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return p
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return FileName
		}
		dir = parent
	}
}

// Load reads and parses a .git-board.toml file at path; an empty config
// when there is none.
func Load(path string) (*Config, error) {
	// No config is a valid start: fields are the labels' own namespaces.
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		cfg := &Config{Discover: true}
		cfg.UseLocal(filepath.Dir(path))
		return cfg, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg, md, err := parse(data)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	for _, k := range md.Undecoded() {
		cfg.Unknown = append(cfg.Unknown, k.String())
	}
	warnUnknown(path, cfg.Unknown)
	// a clone's own board.experimental, the cycle's table checked when it
	// turns the cycle on
	cfg.UseLocal(filepath.Dir(path))
	if cfg.local != nil && slices.Contains(*cfg.local, FeatureCycle) && cfg.cycleErr != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, cfg.cycleErr)
	}
	return cfg, nil
}

// Warnings is where Load names the keys it doesn't know: stderr, every
// command's.
var Warnings io.Writer = os.Stderr

// warned is the keys Load has named, by file: a command loads its config
// more than once.
var warned sync.Map

// warnUnknown names each unknown key on Warnings, once a file and key: a
// misspelled key isn't an error (an older git-board reads a newer file),
// but it shouldn't be silently nothing.
func warnUnknown(path string, keys []string) {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	for _, k := range keys {
		if _, done := warned.LoadOrStore(abs+"\x00"+k, true); !done {
			fmt.Fprintf(Warnings, "warning: %s: %s: git-board doesn't know this key, ignored (git board lint)\n", path, k)
		}
	}
}

func parse(data []byte) (*Config, toml.MetaData, error) {
	var r raw
	md, err := toml.Decode(string(data), &r)
	if err != nil {
		return nil, md, err
	}
	if err := checkColors(r.Fields); err != nil {
		return nil, md, err
	}
	if err := checkTypes(r.Fields); err != nil {
		return nil, md, err
	}
	if err := checkRoles(r.Roles, r.Fields); err != nil {
		return nil, md, err
	}
	if a := r.Repos.Assignees; a != "" && a != "native" && a != "labels" {
		return nil, md, fmt.Errorf("[repos] assignees: native or labels, not %q", a)
	}
	r.Cycle.On = md.IsDefined("cycle") // a subtable alone, [cycle.signoffs], defines it too
	// the cycle is experimental: its table breaks the file only when the
	// team turned it on; off, what it fails is a warning (Check)
	cycleErr := checkCycleTable(md, &r.Cycle, r.Fields)
	if cycleErr != nil && slices.Contains(r.Experimental.Features, FeatureCycle) {
		return nil, md, cycleErr
	}
	if err := checkRelease(r.Release); err != nil {
		return nil, md, err
	}
	if err := checkDescriptions(r.Fields); err != nil {
		return nil, md, err
	}
	if err := checkKindWords(data, r.Branches.Kinds); err != nil {
		return nil, md, err
	}
	r.Labels.Defined = md.IsDefined("labels")
	if err := checkLabels(r.Labels, r.Branches.Kinds); err != nil {
		return nil, md, err
	}
	forges, publishPrivate, discover, err := parseForges(md, r.Forges)
	if err != nil {
		return nil, md, err
	}
	if err := checkAliases(r.Fields); err != nil {
		return nil, md, err
	}
	if err := checkFieldMappings(&Config{Forges: forges, Schema: fields.Schema{Fields: r.Fields, Roles: r.Roles, Sprints: r.Sprints}}); err != nil {
		return nil, md, err
	}
	// the kinds are plain labels unless [fields.kind] has options of its own
	if kinds := keysInOrder(md, r.Branches.Kinds, "branches", "kinds"); len(kinds) > 0 && len(r.Fields["kind"].Options) == 0 {
		if r.Fields == nil {
			r.Fields = map[string]fields.Field{}
		}
		f := r.Fields["kind"]
		f.Single, f.Options, f.Flat = true, kinds, true
		r.Fields["kind"] = f
	}
	return &Config{
		Repos:     r.Repos.List,
		Assignees: cmp.Or(r.Repos.Assignees, "native"),
		Schema: fields.Schema{
			Fields: r.Fields, Sprints: r.Sprints, Roles: r.Roles, Labels: r.Labels,
			Order: keysInOrder(md, r.Fields, "fields"),
		},
		Views:          r.Views,
		viewOrder:      keysInOrder(md, r.Views, "views"),
		GithubProject:  r.GithubProject,
		Uploads:        r.Uploads,
		Branches:       r.Branches,
		Cycle:          r.Cycle,
		Release:        r.Release,
		Template:       r.Template,
		Forges:         forges,
		PublishPrivate: publishPrivate,
		Discover:       discover,
		Features:       r.Experimental.Features,
		cycleErr:       cycleErr,
	}, md, nil
}

// keysInOrder are table's keys (m's) as the file lists them: the
// [branches] kinds, the views.
func keysInOrder[V any](md toml.MetaData, m map[string]V, table ...string) []string {
	var res []string
	for _, k := range md.Keys() {
		if len(k) == len(table)+1 && slices.Equal(k[:len(table)], table) {
			res = append(res, k[len(table)])
		}
	}
	// the keys the file has no table for (the flat kinds' field, made from
	// [branches.kinds]) after the file's, by name
	res = slices.DeleteFunc(res, func(k string) bool { _, ok := m[k]; return !ok })
	for _, k := range slices.Sorted(maps.Keys(m)) {
		if !slices.Contains(res, k) {
			res = append(res, k)
		}
	}
	return res
}

// ViewNames are the views' names as the file lists them: the board's tabs,
// the first the one it opens on. A Config not parsed from a file: sorted.
func (c *Config) ViewNames() []string {
	if len(c.viewOrder) == len(c.Views) {
		return c.viewOrder
	}
	return slices.Sorted(maps.Keys(c.Views))
}

// checkDescriptions keeps each value's description to GitHub's 100
// characters, or says which is longer.
func checkDescriptions(defs map[string]fields.Field) error {
	for name, f := range defs {
		for v, d := range f.Descriptions {
			if n := utf8.RuneCountInString(d); n > 100 {
				return fmt.Errorf("fields.%s.descriptions: %q's is %d characters, GitHub takes 100 at most", name, v, n)
			}
		}
	}
	return nil
}

// ProjectFields are the fields the GitHub Project sync keeps in step with
// the Project (internal/ghbridge), milestone read only.
var ProjectFields = []string{"status", "prio", "mod", "est", "sprint", "moscow", "milestone"}

// Held says whether the forges' sync keeps a label off the forge named by
// key (syncer.Options Held): a label is held when its field's home on
// that forge (FieldTarget) is not a label — the Project, the assignees,
// the milestone, the due date, nowhere. The milestone is the forge's own,
// never ours; an issue reference (parent:) is held where its field says
// so, as everywhere the field maps to sub-issues or nothing. A label
// without a namespace is never held.
// ponytail: every GitHub repository of the board, not the Project's alone:
// one Project per board.
func (c *Config) Held(key, label string) bool {
	field, _, ok := strings.Cut(label, ":")
	if !ok {
		return false
	}
	field = strings.ToLower(field) // GitHub's label names ignore case
	if field == "milestone" {
		return true
	}
	f, _ := c.forgeOf(key)
	switch c.FieldTarget(f, field) {
	case TargetLabel, TargetScopedLabel:
		return false
	}
	return true
}

// SyncsAssignees says whether the assignees live natively on the forge a
// sync key names: the assignee field's home there (FieldTarget) is the
// forge's own assignees. A mapping moving them to a label, or nowhere,
// silences the native writes and reads — both sync ways read the field
// from its home alone.
func (c *Config) SyncsAssignees(key string) bool {
	f, _ := c.forgeOf(key)
	return c.FieldTarget(f, "assignee") == TargetAssignees
}

// FlatKinds tells the kinds are plain labels, [branches] kinds without
// [fields.kind] options: bug, not kind:bug. They are the kind field all the
// same.
func (c *Config) FlatKinds() bool {
	f, ok := c.Fields["kind"]
	return f.Flat || (!ok && len(c.Branches.Kinds) > 0)
}

// Label is the label a field's value is stored as: bug for a flat kind,
// else field:value.
func (c *Config) Label(field, value string) string {
	if _, ok := c.Branches.Kinds[value]; ok && field == "kind" && c.FlatKinds() {
		return value
	}
	return field + ":" + value
}

// SetGithubProject upserts the [github-project] table in the
// .git-board.toml at path, so `bridge pull` doesn't need --owner/--project
// on every run. It edits the file textually (find-the-table-or-append)
// rather than decoding and re-encoding the whole document, because
// BurntSushi/toml's encoder drops comments and formatting on a full
// round-trip — every other table in the file is left byte-for-byte alone.
// Creates the file if it doesn't exist yet.
func SetGithubProject(path, owner string, number int) error {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	updated := WithGithubProject(string(data), GithubProject{Owner: owner, Number: number})
	// a file every command would fail on isn't written: the old one stays
	if _, _, err := parse([]byte(updated)); err != nil {
		return fmt.Errorf("%s with [github-project] owner = %q, number = %d wouldn't parse, left as it was: %w", path, owner, number, err)
	}
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// WithGithubProject is content with its [github-project] table replaced,
// or appended: what Settings › Sync's Project line writes. write = true
// stays when gp has it. Pure, as WithRelease.
func WithGithubProject(content string, gp GithubProject) string {
	block := fmt.Sprintf("[github-project]\nowner = %q\nnumber = %d\n", gp.Owner, gp.Number)
	if gp.Write {
		block += "write = true\n"
	}
	return upsertTable(content, "[github-project]", block)
}

// WithoutGithubProject is content without its [github-project] table.
func WithoutGithubProject(content string) string {
	return withoutTable(content, "[github-project]")
}

// WithRelease is content with its [release] table replaced, or appended,
// by one of binaries, platforms, build (when set) and remote (when not
// origin): what Enable binary release writes. The file's other tables and
// comments stay. Pure: the server checks the result before saving it.
func WithRelease(content string, r release.Config) string {
	quoted := func(ss []string) string {
		q := make([]string, len(ss))
		for i, s := range ss {
			q[i] = fmt.Sprintf("%q", s)
		}
		return strings.Join(q, ", ")
	}
	var b strings.Builder
	b.WriteString("[release]\n")
	fmt.Fprintf(&b, "binaries = [%s]\n", quoted(r.Binaries))
	if len(r.Platforms) > 0 {
		fmt.Fprintf(&b, "platforms = [%s]\n", quoted(r.Platforms))
	}
	if r.Build != "" {
		fmt.Fprintf(&b, "build = %q\n", r.Build)
	}
	if r.Remote != "" && r.Remote != "origin" {
		fmt.Fprintf(&b, "remote = %q\n", r.Remote)
	}
	return upsertTable(content, "[release]", b.String())
}

// WithChecks is content with a [cycle.checks.<name>] table per check,
// replaced or appended: what Propose checks writes. Pure, as WithRelease.
func WithChecks(content string, checks map[string]proof.Suite) string {
	list := func(ss []string) string {
		q := make([]string, len(ss))
		for i, s := range ss {
			q[i] = fmt.Sprintf("%q", s)
		}
		return "[" + strings.Join(q, ", ") + "]"
	}
	for _, name := range slices.Sorted(maps.Keys(checks)) {
		c := checks[name]
		var b strings.Builder
		fmt.Fprintf(&b, "[cycle.checks.%s]\nrun = %s\n", name, list(c.Run))
		if c.Dir != "" {
			fmt.Fprintf(&b, "dir = %q\n", c.Dir)
		}
		if len(c.Files) > 0 {
			fmt.Fprintf(&b, "files = %s\n", list(c.Files))
		}
		content = upsertTable(content, "[cycle.checks."+name+"]", b.String())
	}
	return content
}

// upsertTable replaces the named top-level table (header through the line
// before the next "[" header and its comments, or EOF) if present, else
// appends it — a blank line ahead of it if the file is non-empty.
func upsertTable(content, header, block string) string {
	lines := strings.Split(content, "\n")
	start := -1
	end := len(lines)
	for i, line := range lines {
		if start < 0 {
			if sameHeader(line, header) {
				start = i
			}
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(line), "[") {
			end = i
			break
		}
	}
	if end < len(lines) {
		// the comments above the next header are its own: they stay
		for end > start+1 && (strings.TrimSpace(lines[end-1]) == "" || strings.HasPrefix(strings.TrimSpace(lines[end-1]), "#")) {
			end--
		}
		for strings.TrimSpace(lines[end]) == "" {
			end++
		}
	}

	if start < 0 {
		if strings.TrimSpace(content) == "" {
			return block
		}
		return strings.TrimRight(content, "\n") + "\n\n" + block
	}

	before := strings.Join(lines[:start], "\n")
	after := strings.Join(lines[end:], "\n")
	var b strings.Builder
	if before != "" {
		b.WriteString(before)
		b.WriteString("\n")
	}
	b.WriteString(block)
	if strings.TrimSpace(after) != "" {
		b.WriteString("\n")
		b.WriteString(after)
	}
	return b.String()
}

// tableHeader matches a table's header line: the spaces TOML allows
// inside the brackets, and a comment after them.
var tableHeader = regexp.MustCompile(`^\s*\[\s*([^\[\]]*?)\s*\]\s*(#.*)?$`)

// sameHeader is whether line is header's table: [ a . b ]  # c is [a.b].
// A quoted key's own spaces count, the ones around the dots don't.
func sameHeader(line, header string) bool {
	norm := func(l string) (string, bool) {
		m := tableHeader.FindStringSubmatch(l)
		if m == nil {
			return "", false
		}
		var parts []string
		for _, p := range splitDotted(m[1]) {
			parts = append(parts, strings.TrimSpace(p))
		}
		return strings.Join(parts, "."), true
	}
	got, ok := norm(line)
	want, _ := norm(header)
	return ok && got == want
}

// splitDotted splits a dotted key at the dots outside quotes.
func splitDotted(key string) []string {
	var parts []string
	var quote rune
	start := 0
	for i, r := range key {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '"' || r == '\'':
			quote = r
		case r == '.':
			parts = append(parts, key[start:i])
			start = i + 1
		}
	}
	return append(parts, key[start:])
}

// AppendSprint appends a new [[sprints.list]] entry to the .git-board.toml
// at path, via the same targeted text-surgery SetGithubProject uses (find
// the right spot, splice text) rather than decode+re-encode, which would
// drop comments/formatting elsewhere in the file. Errors if slug is already
// used by a sprint in the current file.
func AppendSprint(path, slug, title string, start time.Time) error {
	cfg, err := Load(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	for _, s := range cfg.Sprints.List {
		if s.Slug == slug {
			return fmt.Errorf("sprint %q already exists in %s", slug, path)
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	block := fmt.Sprintf("[[sprints.list]]\nslug = %q\ntitle = %q\nstart = %s\n",
		slug, title, start.Format("2006-01-02"))
	updated := appendAfterLastMatch(string(data), "[[sprints.list]]", block)
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// appendAfterLastMatch inserts block right after the last line-exact-match
// of header (through the line before the next "[" header, or EOF) — so a
// new [[sprints.list]] entry lands grouped with the existing ones instead
// of at the end of unrelated tables. Appends at EOF (blank-line separated)
// if header doesn't occur in content at all.
func appendAfterLastMatch(content, header, block string) string {
	lines := strings.Split(content, "\n")
	end := -1
	for i, line := range lines {
		if strings.TrimSpace(line) != header {
			continue
		}
		end = len(lines)
		for j := i + 1; j < len(lines); j++ {
			if strings.HasPrefix(strings.TrimSpace(lines[j]), "[") {
				end = j
				break
			}
		}
	}

	if end < 0 {
		if strings.TrimSpace(content) == "" {
			return block
		}
		return strings.TrimRight(content, "\n") + "\n\n" + block
	}

	before := strings.TrimRight(strings.Join(lines[:end], "\n"), "\n")
	after := strings.Join(lines[end:], "\n")
	var b strings.Builder
	b.WriteString(before)
	b.WriteString("\n\n")
	b.WriteString(block)
	if strings.TrimSpace(after) != "" {
		b.WriteString("\n")
		b.WriteString(after)
	}
	return b.String()
}

var slugNonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

// Slug lower-cases value to letters, digits and dashes: "QA Validation",
// "Comité Spécialisé" are "qa-validation", "comite-specialise", as
// [fields.*].options are written. Accents are dropped from their letter,
// not the letter with them.
func Slug(value string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(strings.ToLower(strings.TrimSpace(value))) {
		if !unicode.Is(unicode.Mn, r) {
			b.WriteRune(r)
		}
	}
	return strings.Trim(slugNonAlnum.ReplaceAllString(b.String(), "-"), "-")
}
