package store

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/guilbep/git-board/core/forge"
	"github.com/guilbep/git-board/core/forge/github"
	"github.com/guilbep/git-board/core/forge/gitlab"
	"github.com/guilbep/git-board/core/people"
	"github.com/guilbep/git-board/internal/config"
)

// A forge's token is never in git-board's files: it comes from the
// environment, for its own host only, else from git's credential helper,
// by the adapters' own rule (github.Token, gitlab.Token). Settings stores
// a new one with git credential approve. No token is ever shown back.

// Credential is a forge account with a token, as the settings page lists
// it: never the token.
type Credential struct {
	ID     string `json:"id"` // the forge's name
	Target string `json:"target"`
	Host   string `json:"host"`
	Login  string `json:"login"`
}

// Token is the token of login on f's host, or an error without one.
func (b *Backend) Token(ctx context.Context, f config.Forge, login string) (string, error) {
	find := b.Tokens
	if find == nil {
		find = forgeToken
	}
	t, err := find(ctx, f.Kind, f.BaseURL(), login)
	if err == nil && t == "" {
		err = fmt.Errorf("no token for %q on %s", login, f.Host)
	}
	return t, err
}

func forgeToken(ctx context.Context, kind, baseURL, login string) (string, error) {
	if kind == "gitlab" {
		return gitlab.Token(ctx, baseURL, login)
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	return github.Token(ctx, u.Host, login)
}

// StoreToken keeps login's token for f's host in git's credential helper.
func (b *Backend) StoreToken(ctx context.Context, f config.Forge, login, token string) error {
	store := b.Approve
	if store == nil {
		store = approve
	}
	return store(ctx, f.BaseURL(), login, token)
}

// approve gives the token to git credential approve, on its input only,
// never on a command line; its output, which may hold it, is never shown.
func approve(ctx context.Context, baseURL, login, token string) error {
	u, err := url.Parse(baseURL)
	if err != nil || strings.ContainsAny(u.Host+login+token, "\r\n\x00") {
		return errors.New("bad host, login or token")
	}
	_, err = forge.Credential(ctx, "approve", "protocol="+u.Scheme+"\nhost="+u.Host+"\nusername="+login+"\npassword="+token+"\n\n")
	return err
}

// ForgeToken is the token for the first of forges on host, as its posting
// account: "" for none.
func (b *Backend) ForgeToken(forges []config.Forge, host string) string {
	for _, f := range forges {
		if f.Host == host {
			t, _ := b.Token(ctx, f, f.Login)
			return t
		}
	}
	return ""
}

// Credentials are the forges whose posting account has a token.
func (b *Backend) Credentials(forges []config.Forge) []Credential {
	res := []Credential{}
	for _, f := range forges {
		if _, err := b.Token(ctx, f, f.Login); err == nil {
			res = append(res, Credential{ID: f.Name, Target: f.Kind, Host: f.Host, Login: f.Login})
		}
	}
	return res
}

// GitHubToken is the GitHub login of the person git-board writes as, their
// github.com account's, and that login's token: who opens and merges pull
// requests from git-board, never with another person's token.
func (b *Backend) GitHubToken() (login, token string, err error) {
	id, err := b.me()
	if err != nil {
		return "", "", err
	}
	p, err := people.Load(b.s, id)
	if err != nil {
		return "", "", err
	}
	for _, a := range p.Accounts {
		if a.Server == "github.com" {
			login = a.Login
			break
		}
	}
	if login == "" {
		return "", "", fmt.Errorf("no GitHub login for %s: link your person to its github.com account", p.Name)
	}
	t, err := b.Token(ctx, config.Forge{Kind: "github", Host: "github.com"}, login)
	if err != nil {
		return login, "", fmt.Errorf("no GitHub token for %s", login)
	}
	return login, t, nil
}
