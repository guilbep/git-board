package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guilbep/git-board/domain/releases/proof"
)

// The gate, as git board verify reads it: off with the cycle off; a check
// not run, failed, then passed, signed by a signer the base names.
func TestVerify(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		t.Skip("no ssh-keygen")
	}
	ctx := t.Context()
	dir := t.TempDir()
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
	gitT(t, dir, "init", "-q", "-b", "main")
	write(".git-board.toml", "[cycle.checks.ok]\nrun = [\"sh\", \"-c\", \"test -f fixed\"]\n")
	write(".allowed_signers", `alice namespaces="git-board-ci" `+string(pub))
	gitT(t, dir, "add", "-A")
	gitT(t, dir, "commit", "-q", "-m", "checks")
	gitT(t, dir, "switch", "-q", "-c", "fix")
	open := func() *Board {
		b, err := Open(ctx, dir, Options{})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { b.Close() })
		return b
	}
	if _, _, _, err := open().Verify(ctx, "", ""); err == nil || Classify(err) != Off {
		t.Fatalf("cycle off: %v", err)
	}
	gitT(t, dir, "config", "board.experimental", "cycle")
	b := open()
	if _, _, g, err := b.Verify(ctx, "", "fix"); err != nil || g.Proven || !strings.Contains(g.Reason, "ok not run at") {
		t.Errorf("not run: %+v %v", g, err)
	}
	ok := proof.Suite{Run: []string{"sh", "-c", "test -f fixed"}}
	if _, err := proof.Run(ctx, dir, gitT(t, dir, "rev-parse", "HEAD"), "ok", ok, key, "alice-a-0001"); err != nil {
		t.Fatal(err)
	}
	if _, _, g, err := b.Verify(ctx, "", ""); err != nil || g.Proven || len(g.Checks) != 1 || g.Checks[0].Signer != "alice" {
		t.Errorf("failed: %+v %v", g, err)
	}
	write("fixed", "1")
	gitT(t, dir, "add", "-A")
	gitT(t, dir, "commit", "-q", "-m", "fix")
	if _, err := proof.Run(ctx, dir, gitT(t, dir, "rev-parse", "HEAD"), "ok", ok, key, "alice-a-0001"); err != nil {
		t.Fatal(err)
	}
	if _, base, g, err := b.Verify(ctx, "", ""); err != nil || !g.Proven || base != "main" {
		t.Errorf("passed: %+v on %s, %v", g, base, err)
	}
	if _, _, _, err := b.Verify(ctx, "", "nope"); err == nil {
		t.Error("an unknown commit verified")
	}
}
