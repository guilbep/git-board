package forge

import (
	"context"
	"errors"
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
		if ctx.Err() != nil {
			err = ctx.Err() // the timeout's
		}
		return "", fmt.Errorf("git credential %s: %w", action, err)
	}
	return string(out), nil
}

// Password is what git's credential helper holds for login on host, else
// for the host alone, as a keychain's entry without a username is: a
// token's owner is checked where it matters (As). A helper that timed out
// isn't asked again. ErrNoToken for none.
func Password(ctx context.Context, scheme, host, login string) (string, error) {
	if strings.ContainsAny(scheme+host+login, "\r\n\x00") {
		return "", ErrNoToken
	}
	t, err := fill(ctx, scheme, host, login)
	if t == "" && login != "" && !errors.Is(err, context.DeadlineExceeded) {
		t, _ = fill(ctx, scheme, host, "")
	}
	if t == "" {
		return "", ErrNoToken
	}
	return t, nil
}

// fill is the password git credential fill answers, "" for none.
func fill(ctx context.Context, scheme, host, login string) (string, error) {
	in := "protocol=" + scheme + "\nhost=" + host + "\n"
	if login != "" {
		in += "username=" + login + "\n"
	}
	out, err := Credential(ctx, "fill", in+"\n")
	if err != nil {
		return "", err
	}
	for line := range strings.Lines(out) {
		if v, ok := strings.CutPrefix(strings.TrimRight(line, "\r\n"), "password="); ok && v != "" {
			return v, nil
		}
	}
	return "", nil
}
