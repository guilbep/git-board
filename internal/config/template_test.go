package config

import (
	"os"
	"path/filepath"
	"testing"
)

// The global file's template is what a plain start uses; no file, the
// default.
func TestGlobalTemplate(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if s, err := Start(""); err != nil || s != Default {
		t.Fatalf("no global file: %v, default %v", err, s == Default)
	}
	if err := os.MkdirAll(filepath.Join(dir, "git-board"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "git-board", "config.toml"), []byte(`template = "enac"`), 0o644); err != nil {
		t.Fatal(err)
	}
	enac, _ := Template("enac")
	if s, err := Start(""); err != nil || s != enac {
		t.Errorf("template = enac: %v, enac %v", err, s == enac)
	}
	if s, _ := Start("enac"); s != enac {
		t.Error("--template enac isn't enac")
	}
}
