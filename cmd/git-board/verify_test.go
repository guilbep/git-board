package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/proof"
)

func TestVerify(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		t.Skip("no ssh-keygen")
	}
	ctx := context.Background()
	dir := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "commit.gpgsign=false"}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	key := filepath.Join(t.TempDir(), "key")
	if out, err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-C", "alice", "-f", key).CombinedOutput(); err != nil {
		t.Fatalf("ssh-keygen: %v\n%s", err, out)
	}
	pub, _ := os.ReadFile(key + ".pub")

	git("init", "-q", "-b", "main")
	git("config", "user.name", "dev")
	git("config", "user.email", "dev@example.com")
	write(".git-board.toml", "[cycle.checks.ok]\nrun = [\"sh\", \"-c\", \"test -f fixed\"]\n")
	write(".allowed_signers", `alice namespaces="git-board-ci" `+string(pub))
	git("add", "-A")
	git("commit", "-q", "-m", "checks")
	git("switch", "-q", "-c", "fix")

	verify := func(commit string) (string, error) {
		var out bytes.Buffer
		err := runVerify(ctx, &out, dir, "", commit)
		return out.String(), err
	}
	if out, err := verify("fix"); err == nil || !strings.Contains(err.Error(), "ok not run at") {
		t.Errorf("not run: %q %v", out, err)
	}
	ok := config.Suite{Run: []string{"sh", "-c", "test -f fixed"}}
	if _, err := proof.Run(ctx, dir, git("rev-parse", "HEAD"), "ok", ok, key, "alice-a-0001"); err != nil {
		t.Fatal(err)
	}
	if out, err := verify("HEAD"); err == nil || !strings.Contains(out, "ok           FAILED, signed by alice") {
		t.Errorf("failed: %q %v", out, err)
	}

	write("fixed", "1")
	git("add", "-A")
	git("commit", "-q", "-m", "fix")
	if _, err := proof.Run(ctx, dir, git("rev-parse", "HEAD"), "ok", ok, key, "alice-a-0001"); err != nil {
		t.Fatal(err)
	}
	out, err := verify("HEAD")
	if err != nil || !strings.Contains(out, "against the checks on main") || !strings.Contains(out, "PASSED, signed by alice") {
		t.Errorf("passed: %q %v", out, err)
	}
	if _, err := verify("nope"); err == nil {
		t.Error("an unknown commit verified")
	}
}
