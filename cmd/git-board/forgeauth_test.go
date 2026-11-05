package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/config"
)

func TestForgeLoginAndStatus(t *testing.T) {
	newTestRepo(t)
	if err := os.WriteFile(configPath, []byte(testConfig+"\n[forges.github]\nkind = \"github\"\nproject = \"guilbep/git-board-test\"\nlogin = \"poster\"\nsync = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	creds := filepath.Join(t.TempDir(), "creds")
	if out, err := exec.Command("git", "config", "credential.helper", "store --file="+creds).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}

	out, err := execute(t, []string{"forge", "status"})
	if err != nil || !strings.Contains(out, "posting token for poster: none found: git board forge login github --login poster") {
		t.Fatalf("status before = %q, %v", out, err)
	}
	if _, err := executeIn(t, "", false, []string{"forge", "login", "github"}); exitCode(err) != exitUsage {
		t.Errorf("login with no token = %v, want exit %d", err, exitUsage)
	}
	if _, err := executeIn(t, "s3cret\n", false, []string{"forge", "login", "nope"}); exitCode(err) != exitNotFound {
		t.Errorf("login on no such forge = %v, want exit %d", err, exitNotFound)
	}
	if out, err := executeIn(t, "s3cret\n", false, []string{"forge", "login", "github"}); err != nil || strings.Contains(out, "s3cret") {
		t.Fatalf("login = %q, %v", out, err)
	}
	if data, _ := os.ReadFile(creds); !strings.Contains(string(data), "poster:s3cret@github.com") {
		t.Errorf("the helper holds %q", data)
	}

	was := forgeMe
	forgeMe = func(_ context.Context, f config.Forge) (string, error) { return "poster", nil }
	t.Cleanup(func() { forgeMe = was })
	out, err = execute(t, []string{"forge", "status", "--check"})
	if err != nil || !strings.Contains(out, "posting token for poster: found (git's credential helper), it is poster's") {
		t.Fatalf("status after = %q, %v", out, err)
	}
	if strings.Contains(out, "s3cret") {
		t.Errorf("status prints the token:\n%s", out)
	}
}
