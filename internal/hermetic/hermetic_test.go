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
// no git remote but a local one, no token.
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
}

// Every test binary of cmd/, internal/ and core/ is sealed: a package whose
// tests don't call Run from TestMain reaches the network with the user's
// credential helper.
func TestEveryPackageIsSealed(t *testing.T) {
	t.Parallel()
	for _, root := range []string{"../../cmd", "../../internal", "../../core"} {
		err := filepath.WalkDir(root, func(dir string, d fs.DirEntry, err error) error {
			switch {
			case err != nil || !d.IsDir():
				return err
			case d.Name() == "testdata":
				return filepath.SkipDir
			}
			tests, _ := filepath.Glob(filepath.Join(dir, "*_test.go"))
			if len(tests) == 0 || filepath.Base(dir) == "hermetic" {
				return nil
			}
			for _, f := range tests {
				if src, err := os.ReadFile(f); err == nil && strings.Contains(string(src), "hermetic.Run(m)") {
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
}
