package app

import (
	"cmp"
	"context"
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
// ("": none stored) and whether it syncs.
type NewForge struct {
	Kind  string `json:"kind"`
	URL   string `json:"url"`
	Name  string `json:"name"`
	Login string `json:"login"`
	Token string `json:"token"`
	Sync  *bool  `json:"sync"` // absent: on
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
	f := forge.Table{Kind: n.Kind, Host: u.Host, Project: project, Login: strings.TrimSpace(n.Login), Sync: n.Sync == nil || *n.Sync}
	set := map[string]any{}
	var taken []string
	if b.Config.Forges.Discover {
		var left []string // one that can't be read would be turned off unsaid
		found, err := b.Forges(ctx, func(line string) {
			if strings.Contains(line, " is left out: ") {
				left = append(left, line)
			}
		})
		if err != nil {
			return "", err
		}
		if len(left) > 0 {
			return "", invalidForge("the forges found in the remotes would be dropped, and one can't be written: %s", strings.Join(left, "; "))
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
// own, the login when set.
func forgeTable(f forge.Table) map[string]any {
	t := map[string]any{"kind": f.Kind, "project": f.Project, "sync": f.Sync}
	if f.Host != map[string]string{"github": "github.com", "gitlab": "gitlab.com"}[f.Kind] {
		t["host"] = f.Host
	}
	if f.Login != "" {
		t["login"] = f.Login
	}
	return t
}

// tableNamed is the file's forge called name, else NotFound: a forge
// found in the remotes has no table to change.
func (b *Board) tableNamed(name string) error {
	if slices.ContainsFunc(b.Config.Forges.List, func(f forge.Table) bool { return f.Name == name }) {
		return nil
	}
	return fmt.Errorf("%w: forge %q has no table in %s (one found in the git remotes is added first)", kernel.ErrNotFound, name, conf.FileName)
}

// SetForgeSync turns forge name's sync on or off, in its table.
func (b *Board) SetForgeSync(name string, on bool) error {
	if err := b.tableNamed(name); err != nil {
		return err
	}
	_, err := SetConfigKeys(b.Dir, map[string]any{"forges." + name + ".sync": on}, "", false)
	return err
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
