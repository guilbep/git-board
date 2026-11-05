// Package hermetic keeps a package's tests off the network and away from
// the user's tokens: every test binary calls Run from its TestMain. It is
// v1's core/internal/hermetic, copied: next/ imports nothing of v1.
package hermetic

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"testing"
)

// Run runs the tests with no way to a forge, unless BOARD_LIVE is set
// (the live tests, which choose their own projects):
//   - http.DefaultTransport, which every client here uses, dials loopback
//     hosts alone, and no proxy;
//   - no token in the environment, and no git config but the
//     repository's, so no credential helper answers;
//   - git reaches local repositories alone: no ssh, no https.
//
// after run once the tests ended, before the exit: a package's shared
// fixtures are removed there.
func Run(m *testing.M, after ...func()) {
	done := func(code int) {
		for _, f := range after {
			f()
		}
		os.Exit(code)
	}
	if os.Getenv("BOARD_LIVE") != "" {
		done(m.Run())
	}
	global, err := seal()
	if err != nil {
		fmt.Fprintln(os.Stderr, "hermetic:", err)
		os.Exit(1)
	}
	code := m.Run()
	os.Remove(global)
	done(code)
}

// seal cuts the way out, and answers the global git config it made: the
// user's named who commits, and tests commit, so this one does, lowest of
// all, below a test repository's own.
func seal() (string, error) {
	f, err := os.CreateTemp("", "hermetic-gitconfig-")
	if err != nil {
		return "", err
	}
	if _, err := f.WriteString("[user]\n\tname = Test\n\temail = test@example.com\n"); err != nil {
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	for _, k := range []string{"GH_TOKEN", "GITHUB_TOKEN", "GH_ENTERPRISE_TOKEN", "GITHUB_ENTERPRISE_TOKEN", "GH_HOST",
		"GITLAB_TOKEN", "GITLAB_HOST", "GIT_BOARD_DATASTORE", "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"} {
		os.Unsetenv(k)
	}
	for k, v := range map[string]string{
		"GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": f.Name(), "GIT_TERMINAL_PROMPT": "0",
		"GCM_INTERACTIVE": "never", "GIT_SSH_COMMAND": "false", "GIT_ALLOW_PROTOCOL": "file",
	} {
		os.Setenv(k, v)
	}
	t := http.DefaultTransport.(*http.Transport)
	t.Proxy = nil
	dial := t.DialContext
	t.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return nil, fmt.Errorf("hermetic: a test dialled %s: tests reach loopback hosts alone", addr)
		}
		return dial(ctx, network, addr)
	}
	return f.Name(), nil
}
