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
	if g, err := (Base{Branch: "main"}).Gate(ctx(t), dir, empty); err != nil || !g.Proven || !strings.Contains(g.Reason, "no checks") {
		t.Errorf("no checks: %+v %v", g, err)
	}

	vet := Suite{Run: []string{"sh", "-c", "test -f ok"}, Files: []string{"ok", "src/**"}}
	base := Base{Branch: "main", SignersFile: ".allowed_signers",
		Checks: map[string]Suite{"vet": vet, "forge": {Run: []string{"false"}, Where: "ci"}},
		Tests:  map[string]Suite{"unit": {Run: []string{"false"}}}}
	head := commit(t, dir, map[string]string{"src/a.go": "a"})

	g, err := base.Gate(ctx(t), dir, head)
	if err != nil || g.Proven || !strings.Contains(g.Reason, "no signers file .allowed_signers on main") {
		t.Errorf("no signers: %+v %v", g, err)
	}
	base.Signers = []byte(signers)

	g, _ = base.Gate(ctx(t), dir, head)
	if g.Proven || g.Reason != "vet not run at "+head[:7] || len(g.Checks) != 1 {
		t.Errorf("not run: %+v", g)
	}
	if _, err := Run(ctx(t), dir, head, "vet", vet, keyFile, "alice-a-0001"); err != nil {
		t.Fatal(err)
	}
	if g, _ = base.Gate(ctx(t), dir, head); g.Proven || !strings.Contains(g.Reason, "vet FAILED, signed by alice") {
		t.Errorf("failed: %+v", g)
	}

	head = commit(t, dir, map[string]string{"ok": "1"})
	if g, _ = base.Gate(ctx(t), dir, head); g.Proven || !strings.Contains(g.Reason, "not run") {
		t.Errorf("a FAILED at an older commit, a file under files changed since: %+v", g)
	}
	if _, err := Run(ctx(t), dir, head, "vet", vet, keyFile, "alice-a-0001"); err != nil {
		t.Fatal(err)
	}
	if g, _ = base.Gate(ctx(t), dir, head); !g.Proven {
		t.Errorf("passed: %+v", g)
	}
	// a commit that changes nothing vet reads keeps its proof
	head = commit(t, dir, map[string]string{"README": "changed"})
	if g, _ = base.Gate(ctx(t), dir, head); !g.Proven {
		t.Errorf("a change outside files: %+v", g)
	}
	if _, err := base.Gate(ctx(t), dir, "nope"); err == nil {
		t.Error("an unknown commit read as not run")
	}
}

func TestShow(t *testing.T) {
	t.Parallel()
	dir := repo(t)
	commit(t, dir, map[string]string{"a": "at main"})
	if got, err := Show(ctx(t), dir, "main", "a"); err != nil || string(got) != "at main" {
		t.Errorf("Show(main, a) = %q, %v", got, err)
	}
	if got, err := Show(ctx(t), dir, "main", "missing"); err != nil || got != nil {
		t.Errorf("a file main lacks = %q, %v; want nil", got, err)
	}
	for _, r := range []string{"nope", "--all", ""} {
		if _, err := Show(ctx(t), dir, r, "a"); err == nil {
			t.Errorf("Show(%q) found a revision", r)
		}
	}
}
