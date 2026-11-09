package app

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/guilbep/git-board/conf"
	"github.com/guilbep/git-board/forge"
	"github.com/guilbep/git-board/kernel"
)

// The forges' tables, written as the settings' Add a forge writes them
// (docs/rewrite/api.md, routes 33 to 36), the token to git's credential
// helper, never the file (ADR 0010).

// NewForge is a forge to add: its kind, its project's address, a name
// ("": gh, gl, then gh2… the first free), the posting login, a token
// ("": none stored), whether it syncs, and its way (forge.Table's way,
// "": both).
type NewForge struct {
	Kind  string `json:"kind"`
	URL   string `json:"url"`
	Name  string `json:"name"`
	Login string `json:"login"`
	Token string `json:"token"`
	Sync  *bool  `json:"sync"` // absent: on
	Way   string `json:"way"`  // push, pull, or both ("")
}

var forgeName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// invalidForge is a request that names no forge: Usage, on field.
func invalidForge(format string, a ...any) error {
	return &Error{Class: Usage, Code: "invalid", Err: fmt.Errorf(format, a...)}
}

// AddForge stores n's token, and writes its [forges.<name>] table: a
// GitLab project by its id, read from the forge. A
// table turns the forges found in the remotes off, so with none in the
// file yet, theirs are written first (each read once through the forge,
// as a sync reads it); one naming n's project already refuses n. It
// answers the name written.
func (b *Board) AddForge(ctx context.Context, n NewForge) (string, error) {
	if n.Kind != "github" && n.Kind != "gitlab" {
		return "", invalidForge("forge %q: github or gitlab", n.Kind)
	}
	u, err := url.Parse(strings.TrimSpace(n.URL))
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return "", invalidForge("%q isn't a project's address, like https://github.com/owner/repo", n.URL)
	}
	path, _, _ := strings.Cut(strings.Trim(u.Path, "/"), "/-/")
	project := strings.TrimSuffix(path, ".git")
	if strings.Count(project, "/") < 1 {
		return "", invalidForge("%q names no project: its address is its owner's, then its name", n.URL)
	}
	if n.Name = strings.TrimSpace(n.Name); n.Name != "" && !forgeName.MatchString(n.Name) {
		return "", invalidForge("%q: a forge's name is letters, digits, - and _", n.Name)
	}
	switch n.Way {
	case "", forge.WayPush, forge.WayPull, forge.WayBoth:
	default:
		return "", invalidForge("way %q: push, pull or both", n.Way)
	}
	f := forge.Table{Kind: n.Kind, Host: u.Host, Project: project, Login: strings.TrimSpace(n.Login), Sync: n.Sync == nil || *n.Sync, Way: n.Way}
	set := map[string]any{}
	var taken []string
	if b.Config.Forges.Discover {
		found, err := b.foundToWrite(ctx)
		if err != nil {
			return "", err
		}
		for _, g := range found {
			if strings.EqualFold(g.Host+"/"+g.Project, f.Host+"/"+f.Project) || strings.EqualFold(g.Host+"/"+g.Path, f.Host+"/"+f.Project) {
				return "", invalidForge("the git remotes name %s %s already: it is the forge %s", f.Host, f.Project, g.Name)
			}
			set["forges."+g.Name] = forgeTable(g)
			taken = append(taken, g.Name)
		}
	} else {
		for _, g := range b.Config.Forges.List {
			taken = append(taken, g.Name)
		}
	}
	short := map[string]string{"github": "gh", "gitlab": "gl"}[n.Kind]
	switch {
	case n.Name != "" && slices.Contains(taken, n.Name):
		return "", invalidForge("a forge is named %s already", n.Name)
	case n.Name != "":
		f.Name = n.Name
	default:
		f.Name = short
		for i := 2; slices.Contains(taken, f.Name); i++ {
			f.Name = fmt.Sprintf("%s%d", short, i)
		}
	}
	// the token first: GitLab's table names the project by its id, which
	// the forge answers with it
	if n.Token = strings.TrimSpace(n.Token); n.Token != "" {
		if err := StoreToken(ctx, f, cmp.Or(f.Login, b.myLogin(ctx, f.Host)), n.Token); err != nil {
			return "", err
		}
	}
	if f.Kind == "gitlab" {
		id, err := resolveProject(ctx, f)
		if err != nil {
			return "", invalidForge("%s %s can't be read: %v", f.Host, f.Project, err)
		}
		f.Project = id
	}
	set["forges."+f.Name] = forgeTable(f)
	if _, err := SetConfigKeys(b.Dir, set, "", false); err != nil {
		return "", err
	}
	return f.Name, nil
}

// forgeTable is f as its table writes it: the host when not the kind's
// own, the login when set, the way when not both.
func forgeTable(f forge.Table) map[string]any {
	t := map[string]any{"kind": f.Kind, "project": f.Project, "sync": f.Sync}
	if f.Host != map[string]string{"github": "github.com", "gitlab": "gitlab.com"}[f.Kind] {
		t["host"] = f.Host
	}
	if f.Login != "" {
		t["login"] = f.Login
	}
	if f.Way != "" && f.Way != forge.WayBoth {
		t["way"] = f.Way
	}
	return t
}

// foundToWrite are the forges found in the git remotes, to be written as
// tables: a table turns the found ones off. Each is named as the forge
// names it, as a sync keys its records (a GitHub project's spelling, a
// GitLab project's id), so the records of a sync already run still match.
// A GitHub project with no token yet is written as the URL names it:
// nothing has synced with it. One that cannot be read otherwise refuses
// the write, since it would be turned off unsaid. Two URLs of one project
// are one table.
func (b *Board) foundToWrite(ctx context.Context) ([]forge.Table, error) {
	found, err := b.FoundForges(ctx, func(string) {})
	if err != nil {
		return nil, err
	}
	var out []forge.Table
	var left []string
	seen := map[string]bool{}
	for _, f := range found {
		project, err := resolveProject(ctx, f)
		switch {
		case err == nil:
			if f.Kind == "gitlab" {
				f.Path = f.Project
			}
			f.Project = project
		case f.Kind == "github" && errors.Is(err, forge.ErrNoToken):
			// written as the URL names it
		default:
			left = append(left, fmt.Sprintf("forge %s, %s %s: %v", f.Name, f.Host, f.Project, err))
			continue
		}
		if key := strings.ToLower(f.Host + "/" + f.Project); !seen[key] {
			seen[key] = true
			out = append(out, f)
		}
	}
	if len(left) > 0 {
		return nil, invalidForge("the forges found in the remotes would be dropped, and one can't be written: %s", strings.Join(left, "; "))
	}
	return out, nil
}

// tableNamed is the file's forge called name, else NotFound.
func (b *Board) tableNamed(name string) error {
	if slices.ContainsFunc(b.Config.Forges.List, func(f forge.Table) bool { return f.Name == name }) {
		return nil
	}
	return fmt.Errorf("%w: forge %q has no table in %s", kernel.ErrNotFound, name, conf.FileName)
}

// setForgeKey sets forge name's key to v (nil removes it). A forge found
// in the git remotes gets its table then, with the other found ones,
// as Add a forge writes them: one click makes it read-only, with no
// table to write first.
func (b *Board) setForgeKey(ctx context.Context, name, key string, v any) error {
	if b.tableNamed(name) == nil {
		_, err := SetConfigKeys(b.Dir, map[string]any{"forges." + name + "." + key: v}, "", false)
		return err
	}
	if !b.Config.Forges.Discover {
		return b.tableNamed(name)
	}
	found, err := b.foundToWrite(ctx)
	if err != nil {
		return err
	}
	set := map[string]any{}
	for _, g := range found {
		t := forgeTable(g)
		if g.Name == name {
			if t[key] = v; v == nil {
				delete(t, key)
			}
		}
		set["forges."+g.Name] = t
	}
	if _, ok := set["forges."+name]; !ok {
		return b.tableNamed(name)
	}
	_, err = SetConfigKeys(b.Dir, set, "", false)
	return err
}

// SetForgeSync turns forge name's sync on or off, in its table.
func (b *Board) SetForgeSync(ctx context.Context, name string, on bool) error {
	return b.setForgeKey(ctx, name, "sync", on)
}

// SetForgeWay sets forge name's way, its direction (forge.Table's way):
// push (a backup: writes go there, nothing is pulled), pull (read-only),
// or both. Both is the key's default, so the file says nothing then.
func (b *Board) SetForgeWay(ctx context.Context, name, way string) error {
	switch way {
	case forge.WayPush, forge.WayPull, forge.WayBoth:
	default:
		return &Error{Class: Usage, Code: "invalid", Err: fmt.Errorf("way %q: push, pull or both", way)}
	}
	v := any(nil) // both: the key's default, no line in the file
	if way == forge.WayPush || way == forge.WayPull {
		v = way
	}
	return b.setForgeKey(ctx, name, "way", v)
}

// RemoveForge removes forge name's table; its token stays in git's
// credential helper, which other tools may use.
func (b *Board) RemoveForge(name string) error {
	if err := b.tableNamed(name); err != nil {
		return err
	}
	_, err := SetConfigKeys(b.Dir, map[string]any{"forges." + name: nil}, "", false)
	return err
}

// Credential is a token found for a forge's account: where, never what.
type Credential struct {
	Host   string `json:"host"`
	Login  string `json:"login"`
	Source string `json:"source"`
}

// Credentials are the tokens forge status finds, each host and login
// once, with nothing asked of a forge.
func (b *Board) Credentials(ctx context.Context) ([]Credential, error) {
	list, err := b.ForgeStatuses(ctx, false)
	if err != nil {
		return nil, err
	}
	out := []Credential{}
	for _, f := range list {
		for _, t := range f.Tokens {
			c := Credential{Host: f.Host, Login: t.Login, Source: t.Source}
			if t.Found && !slices.Contains(out, c) {
				out = append(out, c)
			}
		}
	}
	return out, nil
}
