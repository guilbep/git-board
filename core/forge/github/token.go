package github

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/guilbep/git-board/core/forge"
)

// lookup is an account's token on host: the environment variable when set,
// else what git's credential helper holds for the host and login. A
// github.com token is never sent to an Enterprise host, nor the reverse.
func lookup(ctx context.Context, host, login string) (string, error) {
	name := "GITHUB_ENTERPRISE_TOKEN"
	if host == "github.com" {
		name = "GITHUB_TOKEN"
	}
	if t := os.Getenv(name); t != "" {
		return t, nil
	}
	return credential(ctx, host, login)
}

// credential asks git's credential helper, never the terminal. Its answer
// holds the password, so it is never put in an error.
func credential(ctx context.Context, host, login string) (string, error) {
	if strings.ContainsAny(host+login, "\r\n\x00") {
		return "", fmt.Errorf("github: bad host or login: %w", forge.ErrNoToken)
	}
	in := "protocol=https\nhost=" + host + "\n"
	if login != "" {
		in += "username=" + login + "\n"
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "credential", "fill")
	cmd.Stdin = strings.NewReader(in + "\n")
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=")
	out, err := cmd.Output()
	if err == nil {
		for line := range strings.Lines(string(out)) {
			if v, ok := strings.CutPrefix(strings.TrimRight(line, "\r\n"), "password="); ok && v != "" {
				return v, nil
			}
		}
	}
	return "", fmt.Errorf("github: no token for %q on %s: %w", login, host, forge.ErrNoToken)
}
