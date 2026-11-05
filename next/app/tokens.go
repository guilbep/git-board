package app

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"slices"
	"strings"

	"github.com/guilbep/git-board/next/domain/people"
	"github.com/guilbep/git-board/next/forge"
	"github.com/guilbep/git-board/next/forge/github"
	"github.com/guilbep/git-board/next/forge/gitlab"
	"github.com/guilbep/git-board/next/kernel"
)

// The forges' tokens (ADR 0010): from the environment, else git's
// credential helper; git-board keeps none. What forge status and forge
// login are, and the settings' Add a forge with a token, as v1's
// cmd/git-board/forgeauth.go.

// TokenStatus is one account's token on a forge: found or not, where,
// and, checked, whose the forge says it is. Never the token.
type TokenStatus struct {
	Login  string `json:"login"`            // the account it is looked up for; "" for any
	Role   string `json:"role"`             // posting (the forge's table's login) or you
	Found  bool   `json:"found"`            // a token was found
	Source string `json:"source,omitempty"` // an environment variable, or git's credential helper
	Owner  string `json:"owner,omitempty"`  // checked: the login the forge says the token is
	Error  string `json:"error,omitempty"`  // checked: what the forge answered instead
}

// ForgeStatus is a forge and its tokens.
type ForgeStatus struct {
	Name       string        `json:"name"`
	Kind       string        `json:"kind"`
	Host       string        `json:"host"`
	Project    string        `json:"project"`
	Sync       bool          `json:"sync"`
	Discovered bool          `json:"discovered,omitempty"` // found in the git remotes, not in .git-board.toml
	Tokens     []TokenStatus `json:"tokens"`
}

// The forges' doors, which tests replace: the token git-board finds for
// login on f, the login a forge says f's token for login is, and git
// credential approve.
var (
	FindToken = func(ctx context.Context, f forge.Table, login string) (string, error) {
		if f.Kind == "gitlab" {
			return gitlab.Token(ctx, baseURL(f), login)
		}
		return github.Token(ctx, f.Host, login)
	}
	ForgeOwner = func(ctx context.Context, f forge.Table, login string) (string, error) {
		f.Sync = true
		f.Login = login
		fs, err := (&Board{}).OpenForges(ctx, []forge.Table{f})
		if err != nil {
			return "", err
		}
		a, err := fs[0].Forge.Me(ctx)
		return a.Login, err
	}
	ApproveToken = func(ctx context.Context, baseURL, login, token string) error {
		u, err := url.Parse(baseURL)
		if err != nil || u.Host == "" || strings.ContainsAny(u.Host+login+token, "\r\n\x00") {
			return errors.New("bad host, login or token")
		}
		_, err = forge.Credential(ctx, "approve", "protocol="+u.Scheme+"\nhost="+u.Host+"\nusername="+login+"\npassword="+token+"\n\n")
		return err
	}
)

// ForgeStatuses lists the forges (FoundForges: nothing asked of a forge)
// and, for each, whether a token is found for its posting account and
// for your own account on its host, and where. check asks each forge
// whose each token is.
func (b *Board) ForgeStatuses(ctx context.Context, check bool) ([]ForgeStatus, error) {
	forges, err := b.FoundForges(ctx, func(string) {})
	if err != nil {
		return nil, err
	}
	out := []ForgeStatus{}
	for _, f := range forges {
		st := ForgeStatus{Name: f.Name, Kind: f.Kind, Host: f.Host, Project: cmp.Or(f.Path, f.Project), Sync: f.Sync, Discovered: f.Discovered}
		logins := [][2]string{{f.Login, "posting"}}
		if mine := b.myLogin(ctx, f.Host); mine != "" && mine != f.Login {
			logins = append(logins, [2]string{mine, "you"})
		}
		for _, l := range logins {
			ts := TokenStatus{Login: l[0], Role: l[1]}
			if t, err := FindToken(ctx, f, l[0]); err == nil && t != "" {
				ts.Found, ts.Source = true, tokenSource(f)
				if check {
					if who, err := ForgeOwner(ctx, f, l[0]); err != nil {
						ts.Error = err.Error()
					} else {
						ts.Owner = who
					}
				}
			}
			st.Tokens = append(st.Tokens, ts)
		}
		out = append(out, st)
	}
	return out, nil
}

// LoginFor is whose token forge name takes by default: login when given,
// else yours on its host, else its posting account. It answers the forge.
func (b *Board) LoginFor(ctx context.Context, name, login string) (forge.Table, string, error) {
	forges, err := b.FoundForges(ctx, func(string) {})
	if err != nil {
		return forge.Table{}, "", err
	}
	i := slices.IndexFunc(forges, func(f forge.Table) bool { return f.Name == name })
	if i < 0 {
		var names []string
		for _, f := range forges {
			names = append(names, f.Name)
		}
		return forge.Table{}, "", fmt.Errorf("%w: forge %q (the forges: %s)", kernel.ErrNotFound, name, cmp.Or(strings.Join(names, ", "), "none"))
	}
	f := forges[i]
	who := cmp.Or(login, b.myLogin(ctx, f.Host), f.Login)
	if who == "" {
		return f, "", &Error{Class: Usage, Code: "invalid", Err: fmt.Errorf("whose token? --login <your login on %s>", f.Host)}
	}
	return f, who, nil
}

// StoreToken keeps login's token for f's host in git's credential helper
// (git credential approve, the token on its input only).
func StoreToken(ctx context.Context, f forge.Table, login, token string) error {
	if token == "" {
		return &Error{Class: Usage, Code: "invalid", Err: errors.New("no token")}
	}
	if err := ApproveToken(ctx, tokenURL(f), login, token); err != nil {
		return fmt.Errorf("storing the token with git credential approve: %w", err)
	}
	return nil
}

// tokenURL is where f's token is kept: its host, over https (http on a
// loopback GitLab).
func tokenURL(f forge.Table) string {
	if f.Kind == "gitlab" {
		return baseURL(f)
	}
	return "https://" + f.Host
}

// myLogin is your login on host: your person's account there; "" for
// none.
func (b *Board) myLogin(ctx context.Context, host string) string {
	if b.People == nil || b.People.Me == "" {
		return ""
	}
	d, err := people.Read(ctx, b.Kernel, b.People.Avatars)
	if err != nil {
		return ""
	}
	for _, a := range d.ByID[b.People.Me].Accounts {
		if a.Server == host {
			return a.Login
		}
	}
	return ""
}

// tokenSource is where a token for f comes from when the environment has
// one for its host: that variable's name; else git's credential helper.
func tokenSource(f forge.Table) string {
	switch {
	case f.Kind == "github" && f.Host == "github.com" && os.Getenv("GITHUB_TOKEN") != "":
		return "GITHUB_TOKEN"
	case f.Kind == "github" && f.Host == os.Getenv("GITHUB_ENTERPRISE_HOST") && os.Getenv("GITHUB_ENTERPRISE_TOKEN") != "":
		return "GITHUB_ENTERPRISE_TOKEN"
	case f.Kind == "gitlab" && f.Host == cmp.Or(os.Getenv("GITLAB_HOST"), "gitlab.com") && os.Getenv("GITLAB_TOKEN") != "":
		return "GITLAB_TOKEN"
	}
	return "git's credential helper"
}
