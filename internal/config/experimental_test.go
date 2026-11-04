package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestExperimentalOffByDefault(t *testing.T) {
	t.Parallel()
	var none *Config
	if none.Experimental(FeatureCycle) {
		t.Error("a nil config has the cycle on")
	}
	cfg, _, err := Check([]byte("[cycle]\nready = \"ready\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{FeatureCycle, FeatureAgents, FeatureWorktrees} {
		if cfg.Experimental(f) {
			t.Errorf("%s is on without [experimental]", f)
		}
	}
	if got := cfg.ExperimentalOn(); got == nil || len(got) != 0 {
		t.Errorf("ExperimentalOn = %#v, want an empty list", got)
	}
}

func TestExperimentalFromTheFile(t *testing.T) {
	t.Parallel()
	cfg, warnings, err := Check([]byte("[experimental]\nfeatures = [\"worktrees\", \"cycle\", \"teleport\"]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Experimental(FeatureCycle) || !cfg.Experimental(FeatureWorktrees) || cfg.Experimental(FeatureAgents) {
		t.Errorf("on: %v, want cycle and worktrees", cfg.ExperimentalOn())
	}
	if got := cfg.ExperimentalOn(); !slices.Equal(got, []string{"cycle", "worktrees"}) {
		t.Errorf("ExperimentalOn = %v, want the features' order", got)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], `"teleport" isn't a feature`) {
		t.Errorf("warnings = %q, want the unknown name", warnings)
	}
}

func TestExperimentalLocalReplacesTheFile(t *testing.T) {
	t.Parallel()
	cfg, _, err := Check([]byte("[experimental]\nfeatures = [\"cycle\"]\n"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.SetLocal(splitFeatures("agents, Worktrees"))
	if cfg.Experimental(FeatureCycle) || !cfg.Experimental(FeatureAgents) || !cfg.Experimental(FeatureWorktrees) {
		t.Errorf("on: %v, want the clone's agents and worktrees alone", cfg.ExperimentalOn())
	}
	cfg.SetLocal(splitFeatures(""))
	if len(cfg.ExperimentalOn()) != 0 {
		t.Errorf("an empty board.experimental turns everything off, on: %v", cfg.ExperimentalOn())
	}
}

// A broken [cycle] breaks no command while the cycle is off: a warning;
// on, the file is refused as before.
func TestExperimentalBrokenCycle(t *testing.T) {
	t.Parallel()
	broken := "[cycle]\ntests = [\"*.go\"]\n"
	cfg, warnings, err := Check([]byte(broken))
	if err != nil {
		t.Fatalf("off: %v", err)
	}
	if cfg.Experimental(FeatureCycle) {
		t.Error("the cycle is on")
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "test-files now") || !strings.Contains(warnings[0], "while the cycle is off") {
		t.Errorf("warnings = %q", warnings)
	}
	// a clone turning it on still can't walk a broken cycle
	cfg.SetLocal([]string{FeatureCycle})
	if cfg.Experimental(FeatureCycle) {
		t.Error("a broken [cycle] is on")
	}
	if _, _, err := Check([]byte("[experimental]\nfeatures = [\"cycle\"]\n\n" + broken)); err == nil || !strings.Contains(err.Error(), "test-files now") {
		t.Errorf("on: err = %v, want the table refused", err)
	}
}

// Load reads the clone's board.experimental (serial: it runs git).
func TestLoadReadsBoardExperimental(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	path := filepath.Join(dir, FileName)
	if err := os.WriteFile(path, []byte("[cycle]\ntests = [\"*.go\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("off, a broken [cycle] breaks Load: %v", err)
	}
	if _, set := cfg.Local(); set {
		t.Error("no board.experimental, and Local says one")
	}
	git("config", LocalExperimental, "worktrees,agents")
	cfg, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.ExperimentalOn(); !slices.Equal(got, []string{"agents", "worktrees"}) {
		t.Errorf("on: %v, want the clone's agents and worktrees", got)
	}
	// the clone turning the cycle on has the table checked
	git("config", LocalExperimental, "cycle")
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "test-files now") {
		t.Errorf("err = %v, want the broken [cycle]", err)
	}
}

func TestSetExperimentalText(t *testing.T) {
	t.Parallel()
	text := "# the board\n[repos]\nlist = [\"a/b\"]\n"
	on := SetExperimentalText(text, []string{"worktrees", "cycle", "nope"})
	if want := text + "\n[experimental]\nfeatures = [\"cycle\", \"worktrees\"]\n"; on != want {
		t.Errorf("on:\n%s\nwant:\n%s", on, want)
	}
	cfg, _, err := Check([]byte(on))
	if err != nil || !cfg.Experimental(FeatureCycle) || !cfg.Experimental(FeatureWorktrees) {
		t.Errorf("read back: %v, %v", cfg.ExperimentalOn(), err)
	}
	if off := SetExperimentalText(on, nil); off != text {
		t.Errorf("off:\n%q\nwant:\n%q", off, text)
	}
	if same := SetExperimentalText(text, nil); same != text {
		t.Errorf("none to remove changed the file: %q", same)
	}
}

func TestExperimentalOffNamesTheSwitch(t *testing.T) {
	t.Parallel()
	msg := ExperimentalOff(FeatureCycle).Error()
	for _, want := range []string{"Development cycle", `"cycle"`, "[experimental] features", "git config board.experimental cycle"} {
		if !strings.Contains(msg, want) {
			t.Errorf("%q lacks %q", msg, want)
		}
	}
}
