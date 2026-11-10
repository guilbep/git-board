package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// board.avatars, set for this clone or for every repository of the user
// (git's global level), and unset back to the default. It sets
// GIT_CONFIG_GLOBAL, so it runs serially.
func TestSetAvatarsGlobal(t *testing.T) {
	global := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(global, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	b, err := Open(t.Context(), dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	get := func(scope string) string {
		out, _ := exec.Command("git", "-C", dir, "config", scope, "--get", AvatarsKey).Output()
		return strings.TrimSpace(string(out))
	}
	if err := b.SetAvatars(t.Context(), "gravatar", true); err != nil {
		t.Fatal(err)
	}
	if get("--global") != "gravatar" || get("--local") != "" {
		t.Fatalf("global %q, local %q", get("--global"), get("--local"))
	}
	if err := b.SetAvatars(t.Context(), "none", false); err != nil {
		t.Fatal(err)
	}
	if get("--global") != "gravatar" || get("--local") != "none" {
		t.Fatalf("a local write: global %q, local %q", get("--global"), get("--local"))
	}
	for _, global := range []bool{true, false, true} { // unset twice: the second finds none, and is fine
		if err := b.SetAvatars(t.Context(), "", global); err != nil {
			t.Fatal(err)
		}
	}
	if get("--global") != "" || get("--local") != "" {
		t.Fatalf("unset: global %q, local %q", get("--global"), get("--local"))
	}
	if err := b.SetAvatars(t.Context(), "gif", true); Classify(err) != Usage {
		t.Fatalf("a bad mode = %v", err)
	}
}
