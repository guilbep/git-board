package github

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"slices"
	"strings"

	"github.com/guilbep/git-board/core/forge"
)

// Token is an account's token on host, as the adapter finds it: for
// git-board's own calls to the host, which follow the same rule.
func Token(ctx context.Context, host, login string) (string, error) { return lookup(ctx, host, login) }

// lookup is an account's token on host: the environment variable for that
// host, when set, else what git's credential helper holds for the host and
// login. GITHUB_TOKEN is for github.com only, GITHUB_ENTERPRISE_TOKEN for
// the host GITHUB_ENTERPRISE_HOST names only, so a changed setting can't
// send either elsewhere. GH_TOKEN is never read.
func lookup(ctx context.Context, host, login string) (string, error) {
	t := ""
	switch {
	case host == "github.com":
		t = os.Getenv("GITHUB_TOKEN")
	case host == os.Getenv("GITHUB_ENTERPRISE_HOST"):
		t = os.Getenv("GITHUB_ENTERPRISE_TOKEN")
	}
	if t != "" {
		return t, nil
	}
	return credential(ctx, host, login)
}

// checkURL refuses a URL that isn't https, loopback hosts aside, and one
// whose host isn't ok (the forge's own), so no token is sent in clear or away.
func checkURL(raw string, ok ...string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return fmt.Errorf("github: bad URL %q", raw)
	}
	loop := u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"
	if u.Scheme != "https" && !(u.Scheme == "http" && loop) {
		return fmt.Errorf("github: %q isn't https", raw)
	}
	if !loop && !slices.Contains(ok, u.Host) {
		return fmt.Errorf("github: %q is not on %s", raw, strings.Join(ok, " or "))
	}
	return nil
}

// credential asks git's credential helper, never prompting.
func credential(ctx context.Context, host, login string) (string, error) {
	t, err := forge.Password(ctx, "https", host, login)
	if err != nil {
		return "", fmt.Errorf("github: no token for %q on %s: %w", login, host, err)
	}
	return t, nil
}
