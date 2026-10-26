// Package hermetic keeps a package's tests off the network and away from
// the user's tokens: every test binary calls Run from its TestMain. Its
// twin, core/internal/hermetic, does the same for core/, which doesn't
// import internal/.
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
func Run(m *testing.M) {
	if os.Getenv("BOARD_LIVE") != "" {
		os.Exit(m.Run())
	}
	global, err := seal()
	if err != nil {
		fmt.Fprintln(os.Stderr, "hermetic:", err)
		os.Exit(1)
	}
	code := m.Run()
	os.Remove(global)
	os.Exit(code)
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
		"GITLAB_TOKEN", "GITLAB_HOST", "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"} {
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
