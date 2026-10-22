package proof

import (
	"strings"
	"testing"
)

func TestGate(t *testing.T) {
	t.Parallel()
	keyFile, signers := key(t, "alice")
	dir := repo(t)
	empty := commit(t, dir, map[string]string{"README": "r"})
	if g, err := Gate(ctx(t), dir, "main", empty); err != nil || !g.Proven || !strings.Contains(g.Reason, "no checks") {
		t.Errorf("no toml: %+v %v", g, err)
	}

	toml := `[cycle.checks.vet]
run = ["sh", "-c", "test -f ok"]
files = ["ok", "src/**"]

[cycle.tests.unit]
run = ["false"]
`
	commit(t, dir, map[string]string{".git-board.toml": toml})
	run(t, dir, "checkout", "-q", "-b", "feat")
	// the branch adds a check of its own: the base's toml rules
	head := commit(t, dir, map[string]string{"src/a.go": "a", ".git-board.toml": toml + "[cycle.checks.evil]\nrun = [\"false\"]\n"})

	g, err := Gate(ctx(t), dir, "main", head)
	if err != nil || g.Proven || !strings.Contains(g.Reason, "no signers file .allowed_signers on main") {
		t.Errorf("no signers: %+v %v", g, err)
	}
	run(t, dir, "checkout", "-q", "main")
	commit(t, dir, map[string]string{".allowed_signers": signers})
	run(t, dir, "checkout", "-q", "feat")
	run(t, dir, "rebase", "-q", "main")
	head = run(t, dir, "rev-parse", "HEAD")

	g, _ = Gate(ctx(t), dir, "main", head)
	if g.Proven || g.Reason != "vet not run at "+head[:7] || len(g.Checks) != 1 {
		t.Errorf("not run: %+v", g)
	}
	cfg, _, _ := AtBase(ctx(t), dir, "main")
	vet := cfg.Cycle.Checks["vet"]
	if _, err := Run(ctx(t), dir, head, "vet", vet, keyFile, "alice-a-0001"); err != nil {
		t.Fatal(err)
	}
	if g, _ = Gate(ctx(t), dir, "main", head); g.Proven || !strings.Contains(g.Reason, "vet FAILED, signed by alice") {
		t.Errorf("failed: %+v", g)
	}

	head = commit(t, dir, map[string]string{"ok": "1"})
	if g, _ = Gate(ctx(t), dir, "main", head); g.Proven || !strings.Contains(g.Reason, "not run") {
		t.Errorf("a FAILED at an older commit, a file under files changed since: %+v", g)
	}
	if _, err := Run(ctx(t), dir, head, "vet", vet, keyFile, "alice-a-0001"); err != nil {
		t.Fatal(err)
	}
	if g, _ = Gate(ctx(t), dir, "main", head); !g.Proven {
		t.Errorf("passed: %+v", g)
	}
	// a commit that changes nothing vet reads keeps its proof
	head = commit(t, dir, map[string]string{"README": "changed"})
	if g, _ = Gate(ctx(t), dir, "main", head); !g.Proven {
		t.Errorf("a change outside files: %+v", g)
	}
	if _, err := Gate(ctx(t), dir, "nope", head); err == nil {
		t.Error("an unknown base read as no config")
	}
}
