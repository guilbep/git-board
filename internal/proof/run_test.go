package proof

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/config"
)

func TestRun(t *testing.T) {
	t.Parallel()
	keyFile, signers := key(t, "alice")
	dir := repo(t)
	before := commit(t, dir, map[string]string{"README": "r"})
	after := commit(t, dir, map[string]string{"marker": "m"})
	run(t, dir, "checkout", "-q", before)
	// untracked in the working tree: a clean checkout doesn't see it
	if err := os.WriteFile(filepath.Join(dir, "marker"), []byte("leak"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := config.Suite{Run: []string{"sh", "-c", `echo "foo=$FOO"; env; test -f marker`}, Env: map[string]string{"FOO": "bar"}}

	r, err := Run(ctx(t), dir, before, "marker", s, keyFile, "alice-a-0001")
	if err != nil || r.Verdict != Failed || r.Commit != before {
		t.Fatalf("before: %+v %v", r, err)
	}
	r, err = Run(ctx(t), dir, after, "marker", s, keyFile, "alice-a-0001")
	if err != nil || r.Verdict != Passed {
		t.Fatalf("after: %+v %v", r, err)
	}

	st, err := StatusAt(ctx(t), dir, after, "marker", s, []byte(signers))
	if err != nil || !st.Proven || st.Signer != "alice" || st.Result.Log != r.Log {
		t.Errorf("status after: %+v %v", st, err)
	}
	logs, _ := LogDir(ctx(t), dir)
	out, err := os.ReadFile(filepath.Join(logs, strings.TrimPrefix(r.Log, "sha256:")))
	if err != nil || !strings.Contains(string(out), "foo=bar") {
		t.Errorf("log: %q %v", out, err)
	}
	// the environment is cleared: a variable of ours outside passEnv is gone
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if !slices.Contains(passEnv, k) && k != "PWD" && k != "SHLVL" && k != "_" && k != "OLDPWD" {
			if strings.Contains(string(out), "\n"+k+"=") {
				t.Errorf("%s reached the check", k)
			}
			break
		}
	}
	if left := run(t, dir, "worktree", "list", "--porcelain"); strings.Count(left, "worktree ") != 1 {
		t.Errorf("checkout left behind:\n%s", left)
	}

	if _, err := Run(ctx(t), dir, after, "nope", config.Suite{Run: []string{"no-such-binary-gb"}}, keyFile, "alice-a-0001"); err == nil {
		t.Error("a missing binary is a verdict")
	}
	if _, err := Run(ctx(t), dir, after, "forge", config.Suite{Run: []string{"true"}, Where: "ci"}, keyFile, "alice-a-0001"); err == nil {
		t.Error("where = ci ran")
	}
}
