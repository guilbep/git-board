package proof

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Namespace is the SSH signature namespace of a result line.
const Namespace = "git-board-ci"

// ErrUnknownSigner: no key of the signers file made the signature.
var ErrUnknownSigner = errors.New("unknown signer")

// SigningKey is the key that signs this clone's results: git's
// user.signingkey, a path to an SSH key, private or public (ssh-keygen
// then finds the private one beside it, or in the agent).
func SigningKey(ctx context.Context, dir string) (string, error) {
	out, _ := git(ctx, dir, nil, "config", "--get", "user.signingkey")
	key := strings.TrimSpace(string(out))
	switch {
	case key == "":
		return "", errors.New("set git's user.signingkey to the SSH key that signs your commits")
	case strings.HasPrefix(key, "key::"), strings.HasPrefix(key, "ssh-"):
		return "", errors.New("user.signingkey holds a key, not a path to one: git-board signs with a key file")
	}
	if rest, ok := strings.CutPrefix(key, "~/"); ok {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		key = filepath.Join(home, rest)
	}
	return key, nil
}

// Sign signs a payload with the key file: `ssh-keygen -Y sign -n
// git-board-ci`, the signature's base64 on one line.
func Sign(ctx context.Context, keyFile string, payload []byte) (string, error) {
	// a key with a passphrase and no agent would wait on the terminal
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := sshKeygen(ctx, payload, "-Y", "sign", "-n", Namespace, "-f", keyFile)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, l := range strings.Split(string(out), "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "-----") {
			b.WriteString(l)
		}
	}
	return b.String(), nil
}

// Verify checks a signature over a payload against an allowed_signers
// file, and names its signer: `ssh-keygen -Y find-principals`, then `-Y
// verify` as that principal. Nothing in the line names the signer.
func Verify(ctx context.Context, signers, payload []byte, sig string) (string, error) {
	tmp, err := os.MkdirTemp("", "git-board-verify-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	signersFile, sigFile := filepath.Join(tmp, "allowed_signers"), filepath.Join(tmp, "sig")
	var armored strings.Builder
	armored.WriteString("-----BEGIN SSH SIGNATURE-----\n")
	for s := sig; s != ""; {
		n := min(70, len(s))
		armored.WriteString(s[:n] + "\n")
		s = s[n:]
	}
	armored.WriteString("-----END SSH SIGNATURE-----\n")
	if err := errors.Join(os.WriteFile(signersFile, signers, 0o600), os.WriteFile(sigFile, []byte(armored.String()), 0o600)); err != nil {
		return "", err
	}
	out, err := sshKeygen(ctx, nil, "-Y", "find-principals", "-s", sigFile, "-f", signersFile)
	principal, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	if err != nil || principal == "" {
		return "", ErrUnknownSigner
	}
	if _, err := sshKeygen(ctx, payload, "-Y", "verify", "-f", signersFile, "-I", principal, "-n", Namespace, "-s", sigFile); err != nil {
		return "", fmt.Errorf("bad signature by %s: %w", principal, err)
	}
	return principal, nil
}

func sshKeygen(ctx context.Context, stdin []byte, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "ssh-keygen", args...)
	var out, errb bytes.Buffer
	cmd.Stdin, cmd.Stdout, cmd.Stderr = bytes.NewReader(stdin), &out, &errb
	if err := cmd.Run(); err != nil {
		return out.Bytes(), fmt.Errorf("ssh-keygen %s: %w: %s", args[1], err, strings.TrimSpace(errb.String()))
	}
	return out.Bytes(), nil
}
