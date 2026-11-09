package app

import (
	"cmp"
	"context"
	"fmt"

	"github.com/guilbep/git-board/forge"
	"github.com/guilbep/git-board/forge/github"
)

// The OAuth device flow (#101, ADR 0010): a person logs in to github.com
// in the browser, with a code git-board shows, and the token goes to
// git's credential helper under the login GitHub says it is. GitHub
// Enterprise and GitLab keep the pasted token: an Enterprise host needs
// its own OAuth App, and a GitLab token from the flow expires within
// hours, which would mean keeping a refresh token.

// GitHubLogin is github.com's address for the device flow: "" is
// https://github.com; tests, a fake. GitHubClientID is the build's OAuth
// App (github.ClientID, set at build time); tests set their own.
var (
	GitHubLogin    = ""
	GitHubClientID = github.ClientID
)

// DeviceLogin says whether forge f can log in with the device flow: on
// github.com, in a build with an OAuth App.
func DeviceLogin(f forge.Table) bool {
	return f.Kind == "github" && f.Host == "github.com" && GitHubClientID != ""
}

// Device is a device flow started for a forge: the code to type at URL.
type Device struct {
	*github.Device
	f forge.Table
}

// StartDevice starts the device flow for forge name.
func (b *Board) StartDevice(ctx context.Context, name string) (*Device, error) {
	f, err := b.forgeNamed(ctx, name)
	if err != nil {
		return nil, err
	}
	if !DeviceLogin(f) {
		return nil, &Error{Class: Usage, Code: "invalid", Err: fmt.Errorf("forge %s (%s): no login in the browser here; paste a token", name, f.Host)}
	}
	d, err := github.StartDevice(ctx, cmp.Or(GitHubLogin, "https://github.com"), GitHubClientID)
	if err != nil {
		return nil, err
	}
	return &Device{Device: d, f: f}, nil
}

// Wait waits for the person to approve, asks GitHub whose the token is,
// and stores it for that login in git's credential helper. It answers
// the login.
func (d *Device) Wait(ctx context.Context) (string, error) {
	token, err := d.Device.Wait(ctx)
	if err != nil {
		return "", err
	}
	g, err := github.New(ctx, github.Options{Project: d.f.Project, Host: d.f.Host, API: GitHubAPI,
		Token: func(context.Context, forge.Account) (string, error) { return token, nil }})
	if err != nil {
		return "", err
	}
	me, err := g.Me(ctx)
	if err != nil {
		return "", fmt.Errorf("asking GitHub whose the new token is: %w", err)
	}
	if err := StoreToken(ctx, d.f, me.Login, token); err != nil {
		return "", err
	}
	return me.Login, nil
}
