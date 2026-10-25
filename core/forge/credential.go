package forge

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// CredentialTimeout bounds each call to git's credential helper.
var CredentialTimeout = 10 * time.Second

// Credential runs git credential with action (fill, approve) and in as its
// input, never prompting: no terminal, no askpass, no Git Credential
// Manager window, and for CredentialTimeout at most. Its output may hold a
// password, so it is never put in an error.
func Credential(ctx context.Context, action, in string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, CredentialTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "credential", action)
	cmd.Stdin = strings.NewReader(in)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=", "GCM_INTERACTIVE=never")
	cmd.WaitDelay = time.Second // a helper git started may keep the output open
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git credential %s: %w", action, err)
	}
	return string(out), nil
}

// Password is what git's credential helper holds for login on host, or
// ErrNoToken.
func Password(ctx context.Context, scheme, host, login string) (string, error) {
	if strings.ContainsAny(scheme+host+login, "\r\n\x00") {
		return "", ErrNoToken
	}
	in := "protocol=" + scheme + "\nhost=" + host + "\n"
	if login != "" {
		in += "username=" + login + "\n"
	}
	out, err := Credential(ctx, "fill", in+"\n")
	if err != nil {
		return "", ErrNoToken
	}
	for line := range strings.Lines(out) {
		if v, ok := strings.CutPrefix(strings.TrimRight(line, "\r\n"), "password="); ok && v != "" {
			return v, nil
		}
	}
	return "", ErrNoToken
}
