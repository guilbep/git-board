package gitbugio

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/git-bug/git-bug/bridge/core/auth"
	"github.com/git-bug/git-bug/repository"
)

// Storage is git-bug's folder in the repository (.git/git-bug), for
// git-board's caches.
func (b *Backend) Storage() repository.LocalStorage {
	return b.c.LocalStorage()
}

// ForgeToken returns a token stored in git-bug's keyring for a forge host
// ("github.com", or a GitLab or Gitea instance by its base URL), "" when
// there's none.
func (b *Backend) ForgeToken(host string) string {
	creds, err := auth.List(b.c, auth.WithKind(auth.KindToken))
	if err != nil {
		return ""
	}
	for _, c := range creds {
		if credentialHost(c) == host {
			return c.(*auth.Token).Value
		}
	}
	return ""
}

// GitHubToken is the GitHub login of the identity git-board writes as,
// and that login's token in git-bug's keyring: who opens and merges pull
// requests from git-board, as the bridge posts that person's comments.
// Never another login's token.
func (b *Backend) GitHubToken() (login, token string, err error) {
	login = b.myGitHubLogin()
	if login == "" {
		return "", "", errors.New("no GitHub login: choose who you are in git-board first")
	}
	creds, err := auth.List(b.c, auth.WithTarget("github"), auth.WithKind(auth.KindToken))
	if err != nil {
		return login, "", err
	}
	for _, c := range creds {
		if l, _ := c.GetMetadata(auth.MetaKeyLogin); strings.EqualFold(l, login) {
			return login, c.(*auth.Token).Value, nil
		}
	}
	return login, "", fmt.Errorf("no GitHub token for %s", login)
}

// ForgeHosts returns the hosts git-bug holds a token for.
func (b *Backend) ForgeHosts() []string {
	creds, err := auth.List(b.c, auth.WithKind(auth.KindToken))
	if err != nil {
		return nil
	}
	var hosts []string
	for _, c := range creds {
		if h := credentialHost(c); h != "" && !slices.Contains(hosts, h) {
			hosts = append(hosts, h)
		}
	}
	return hosts
}

// credentialHost is the host a credential is for: github.com for GitHub,
// its base URL's host for an instance.
func credentialHost(c auth.Credential) string {
	if c.Target() == "github" {
		return "github.com"
	}
	base, ok := c.GetMetadata(auth.MetaKeyBaseURL)
	if !ok {
		return ""
	}
	u, err := url.Parse(base)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// Credential is a token in git-bug's credential store, as the settings
// page lists it: never its value.
type Credential struct {
	ID     string `json:"id"` // its first 7 characters, enough to pick it
	Target string `json:"target"`
	Host   string `json:"host"`
	Login  string `json:"login"`
}

// Credentials lists git-bug's stored tokens, without their values.
func (b *Backend) Credentials() ([]Credential, error) {
	creds, err := auth.List(b.c, auth.WithKind(auth.KindToken))
	if err != nil {
		return nil, err
	}
	res := []Credential{}
	for _, c := range creds {
		login, _ := c.GetMetadata(auth.MetaKeyLogin)
		res = append(res, Credential{ID: c.ID().Human(), Target: c.Target(), Host: credentialHost(c), Login: login})
	}
	slices.SortFunc(res, func(a, b Credential) int {
		return strings.Compare(a.Target+a.Host+a.Login, b.Target+b.Host+b.Login)
	})
	return res, nil
}
