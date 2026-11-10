// Package hermetic keeps a package's tests off the network and away from
// the user's tokens: every test binary calls Run from its TestMain. It is
// v1's core/internal/hermetic, copied when v2 grew beside v1 (ADR 0021).
package hermetic

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// Run runs the tests with no way to a forge, unless BOARD_LIVE is set
// (the live tests, which choose their own projects):
//   - http.DefaultTransport, which every client here uses, dials loopback
//     hosts alone, and no proxy;
//   - no token in the environment, and no git config but the
//     repository's, so no credential helper answers;
//   - no global git-board file: XDG_CONFIG_HOME is an empty folder;
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
	home, err := seal()
	if err != nil {
		fmt.Fprintln(os.Stderr, "hermetic:", err)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(home)
	done(code)
}

// seal cuts the way out, and answers the folder it made: it holds the
// global git config (the user's named who commits, and tests commit, so
// this one does, lowest of all, below a test repository's own) and an
// empty XDG_CONFIG_HOME.
func seal() (string, error) {
	home, err := os.MkdirTemp("", "hermetic-")
	if err != nil {
		return "", err
	}
	xdg := filepath.Join(home, "config")
	if err := os.Mkdir(xdg, 0o700); err != nil {
		return "", err
	}
	f, err := os.Create(filepath.Join(home, "gitconfig"))
	if err != nil {
		return "", err
	}
	// No automatic maintenance: a fetch or a push would start one,
	// detached, that writes into a test's repository after it returned
	// and fails its temporary folder's removal (golden's noMaintenance).
	const config = "[user]\n\tname = Test\n\temail = test@example.com\n" +
		"[maintenance]\n\tauto = false\n\tautoDetach = false\n[gc]\n\tauto = 0\n\tautoDetach = false\n"
	if _, err := f.WriteString(config); err != nil {
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
		"XDG_CONFIG_HOME": xdg,
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
	return home, nil
}
