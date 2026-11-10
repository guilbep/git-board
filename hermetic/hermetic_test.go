package hermetic

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMain(m *testing.M) { Run(m) }

// A test reaches a loopback server, and nothing past it: no forge's API,
// no git remote but a local one, no token, no global git-board file.
func TestSealed(t *testing.T) {
	t.Parallel()
	local := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(local.Close)
	if res, err := http.Get(local.URL); err != nil {
		t.Fatalf("loopback: %v", err)
	} else {
		res.Body.Close()
	}
	if _, err := http.Get("https://api.github.com/"); err == nil || !strings.Contains(err.Error(), "hermetic") {
		t.Errorf("api.github.com: %v, want refused", err)
	}
	out, err := exec.Command("git", "ls-remote", "https://github.com/git/git.git").CombinedOutput()
	if err == nil || !strings.Contains(string(out), "not allowed") {
		t.Errorf("git over https: %v, %s", err, out)
	}
	if os.Getenv("GITHUB_TOKEN") != "" || os.Getenv("GH_TOKEN") != "" {
		t.Error("a token in the environment")
	}
	if out, _ := exec.Command("git", "config", "--get-all", "credential.helper").Output(); len(out) != 0 {
		t.Errorf("a credential helper: %s", out)
	}
	if ents, err := os.ReadDir(os.Getenv("XDG_CONFIG_HOME")); err != nil || len(ents) != 0 {
		t.Errorf("XDG_CONFIG_HOME %q: %v, %d entries, want an empty folder", os.Getenv("XDG_CONFIG_HOME"), err, len(ents))
	}
}

// Every test binary of the module is sealed (ADR 0014): a package whose
// tests do not call Run from TestMain reaches the network with the
// user's tokens and credential helper. contrib/ is its own module, with
// its own guard.
func TestEveryPackageIsSealed(t *testing.T) {
	t.Parallel()
	err := filepath.WalkDir("..", func(dir string, d fs.DirEntry, err error) error {
		switch {
		case err != nil || !d.IsDir():
			return err
		case dir != ".." && (strings.HasPrefix(d.Name(), ".") || d.Name() == "testdata" || d.Name() == "node_modules"),
			dir == filepath.Join("..", "contrib"), dir == filepath.Join("..", "web"), dir == filepath.Join("..", "hermetic"):
			return filepath.SkipDir
		}
		tests, _ := filepath.Glob(filepath.Join(dir, "*_test.go"))
		if len(tests) == 0 {
			return nil
		}
		for _, f := range tests {
			if src, err := os.ReadFile(f); err == nil && strings.Contains(string(src), "hermetic.Run(m") {
				return nil
			}
		}
		t.Errorf("%s: no TestMain calling hermetic.Run(m)", dir)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
