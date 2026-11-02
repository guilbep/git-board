package config

import (
	"cmp"
	"fmt"
	"maps"
	"net"
	"regexp"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
)

// Forge is a [forges.<name>] table: a forge project the issues sync with.
// Its token is never here: the environment or git's credential helper
// holds it (docs/guide/forges.md).
type Forge struct {
	Name string `toml:"-" json:"name"`
	Kind string `toml:"kind" json:"kind"` // github, gitlab
	// Project is owner/repo on GitHub, the project's numeric id on GitLab:
	// a path changes when a group is renamed, the id never.
	Project string `toml:"project" json:"project"`
	Host    string `toml:"host" json:"host"`   // default github.com, gitlab.com
	Login   string `toml:"login" json:"login"` // the posting account
	Sync    bool   `toml:"sync" json:"sync"`
	// Discovered: found in the git remotes, with no table. A GitLab one's
	// Project is its path until the run reads its id; Path keeps the path.
	Discovered bool   `toml:"-" json:"discovered,omitempty"`
	Path       string `toml:"-" json:"path,omitempty"`
	// Fields maps a git-board field to where it lives on this forge
	// (mapping.go): only the overrides; a field missing follows its
	// type's default.
	Fields map[string]FieldMapping `toml:"fields" json:"fields,omitempty"`
}

// BaseURL is the forge's web address: https, or http for a loopback host,
// as the adapters accept.
func (f Forge) BaseURL() string {
	host := f.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return "http://" + f.Host
	}
	return "https://" + f.Host
}

// Where is the project's web page.
func (f Forge) Where() string {
	if f.Kind == "gitlab" && f.Path == "" {
		return f.BaseURL() + "/projects/" + f.Project
	}
	return f.BaseURL() + "/" + cmp.Or(f.Path, f.Project)
}

var (
	forgeName  = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	ghProject  = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	glProject  = regexp.MustCompile(`^[0-9]+$`)
	glPath     = regexp.MustCompile(`^[A-Za-z0-9_.-]+(/[A-Za-z0-9_.-]+)+$`)
	forgeHost  = regexp.MustCompile(`^[A-Za-z0-9.-]+(:[0-9]+)?$`)
	forgeLogin = regexp.MustCompile(`^[A-Za-z0-9_.-]*$`)
)

// checkForge fills the host's default, and refuses a table git-board
// can't sync with.
func checkForge(f *Forge) error {
	where := "[forges." + f.Name + "]"
	switch f.Kind {
	case "github":
		f.Host = cmp.Or(f.Host, "github.com")
		if !ghProject.MatchString(f.Project) {
			return fmt.Errorf("%s project: owner/repo, not %q", where, f.Project)
		}
	case "gitlab":
		f.Host = cmp.Or(f.Host, "gitlab.com")
		if !glProject.MatchString(f.Project) {
			return fmt.Errorf("%s project: the project's numeric id, not %q", where, f.Project)
		}
	default:
		return fmt.Errorf("%s kind: github or gitlab, not %q", where, f.Kind)
	}
	if !forgeHost.MatchString(f.Host) {
		return fmt.Errorf("%s host: a host name, not %q", where, f.Host)
	}
	if !forgeLogin.MatchString(f.Login) {
		return fmt.Errorf("%s login: a login, not %q", where, f.Login)
	}
	return nil
}

// parseForges reads the [forges] table: publish-private, discover, and a
// table per forge. Discovery is on with no table, unless discover = false.
func parseForges(md toml.MetaData, tables map[string]toml.Primitive) (forges []Forge, publishPrivate, discover bool, err error) {
	discover = true
	for _, name := range slices.Sorted(maps.Keys(tables)) {
		if name == "publish-private" || name == "discover" {
			v := map[string]*bool{"publish-private": &publishPrivate, "discover": &discover}[name]
			if err := md.PrimitiveDecode(tables[name], v); err != nil {
				return nil, false, false, fmt.Errorf("[forges] %s: %w", name, err)
			}
			continue
		}
		if !forgeName.MatchString(name) {
			return nil, false, false, fmt.Errorf("[forges.%s]: a forge's name is letters, digits, - and _", name)
		}
		f := Forge{Name: name}
		if err := md.PrimitiveDecode(tables[name], &f); err != nil {
			return nil, false, false, fmt.Errorf("[forges.%s]: %w", name, err)
		}
		if err := checkForge(&f); err != nil {
			return nil, false, false, err
		}
		forges = append(forges, f)
	}
	return forges, publishPrivate, discover && len(forges) == 0, nil
}

// Forge is the forge of that name.
func (c *Config) Forge(name string) (Forge, bool) {
	i := slices.IndexFunc(c.Forges, func(f Forge) bool { return f.Name == name })
	if i < 0 {
		return Forge{}, false
	}
	return c.Forges[i], true
}

// WithForge is content with f's table replaced, or appended. The file's
// other tables and comments stay, and so do the forge's [forges.<name>.*]
// subtables — [forges.gh.fields] above all: the mappings aren't the
// bridges API's to rewrite. They stay because upsertTable's span ends at
// the next header line, a subtable's included; only the base table's own
// keys are rewritten. Pure, as WithRelease.
func WithForge(content string, f Forge) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[forges.%s]\nkind = %q\nproject = %q\n", f.Name, f.Kind, f.Project)
	if f.Host != "" && f.Host != map[string]string{"github": "github.com", "gitlab": "gitlab.com"}[f.Kind] {
		fmt.Fprintf(&b, "host = %q\n", f.Host)
	}
	if f.Login != "" {
		fmt.Fprintf(&b, "login = %q\n", f.Login)
	}
	fmt.Fprintf(&b, "sync = %t\n", f.Sync)
	return upsertTable(content, "[forges."+f.Name+"]", b.String())
}

var blankLines = regexp.MustCompile(`\n{3,}`)

// WithoutForge is content without the named forge's table.
func WithoutForge(content, name string) string {
	return withoutTable(content, "[forges."+name+"]")
}

// SetForgeFieldsText is content with a forge's [forges.<name>.fields]
// holding set, without remove: one inline table per field
// (status = { to = "project", field = "Status" }), one values subtable
// where the mapping renames values. The other keys and the comments
// stay; the parse refuses a mapping that fits nothing, as the file would.
func SetForgeFieldsText(content, forge string, set map[string]FieldMapping, remove []string) string {
	header := "[forges." + forge + ".fields]"
	for _, field := range remove {
		content = deleteTable(content, "forges."+forge+".fields."+field)
		content = setTableKeys(content, header, lastTableHeader(content, "forges."+forge), []string{field}, map[string]string{field: ""}, false)
	}
	if len(set) == 0 {
		return content
	}
	keys := slices.Sorted(maps.Keys(set))
	// a mapping without its target means nothing (the parse refuses one):
	// nothing of it written
	keys = slices.DeleteFunc(keys, func(field string) bool { return set[field].To == "" })
	if len(keys) == 0 {
		return content
	}
	vals := map[string]string{}
	for _, field := range keys {
		vals[field] = set[field].toml()
	}
	content = setTableKeys(content, header, lastTableHeader(content, "forges."+forge), keys, vals, false)
	for _, field := range keys {
		// the values subtable sits one level under the inline mapping, as
		// TOML requires: status = { … } and [forges.gh.fields.status.values]
		dotted := "forges." + forge + ".fields." + field + ".values"
		if len(set[field].Values) == 0 {
			content = deleteTable(content, dotted)
			continue
		}
		var b strings.Builder
		fmt.Fprintf(&b, "[%s]\n", dotted)
		for _, v := range slices.Sorted(maps.Keys(set[field].Values)) {
			fmt.Fprintf(&b, "%s = %s\n", quoteKey(v), tomlString(set[field].Values[v]))
		}
		content = upsertTable(content, "["+dotted+"]", b.String())
	}
	return content
}

// quoteKey is a table key bare when it can be, quoted when not
// (in-progress stays bare, "In Progress" doesn't).
func quoteKey(k string) string {
	if viewName.MatchString(k) {
		return k
	}
	return tomlString(k)
}

// toml is the mapping as one inline table, only what is set.
func (m FieldMapping) toml() string {
	parts := []string{"to = " + tomlString(m.To)}
	if m.As != "" {
		parts = append(parts, "as = "+tomlString(m.As))
	}
	if m.Field != "" {
		parts = append(parts, "field = "+tomlString(m.Field))
	}
	return "{ " + strings.Join(parts, ", ") + " }"
}

func withoutTable(content, header string) string {
	out := blankLines.ReplaceAllString(upsertTable(content, header, ""), "\n\n")
	return strings.TrimRight(out, "\n") + "\n"
}
