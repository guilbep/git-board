package hermetic

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
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
