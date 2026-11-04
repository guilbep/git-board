package main

import (
	"os"
	"strings"
	"testing"
)

func TestConfigCmd(t *testing.T) {
	newTestRepo(t)
	if err := os.WriteFile(configPath, []byte("# ours\n[fields.status]\nsingle = true # one at a time\n\n[views.backlog]\nfilter = \"is:open\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := execute(t, []string{"config", "set", "views.backlog.filter", "is:open prio:p1"}); err != nil {
		t.Fatal(err)
	}
	if out, err := execute(t, []string{"config", "get", "views.backlog.filter"}); err != nil || out != "is:open prio:p1\n" {
		t.Errorf("get = %q, %v", out, err)
	}
	data, _ := os.ReadFile(configPath)
	if !strings.Contains(string(data), "# ours") || !strings.Contains(string(data), "single = true # one at a time") {
		t.Errorf("the file's comments went:\n%s", data)
	}
	if _, err := execute(t, []string{"config", "set", "views.backlog.fliter", "x"}); exitCode(err) != exitUsage {
		t.Errorf("set of an unknown key = %v, want exit %d", err, exitUsage)
	}
	if _, err := execute(t, []string{"config", "get", "sprints.length_days"}); exitCode(err) != exitNotFound {
		t.Errorf("get of a key not set = %v, want exit %d", err, exitNotFound)
	}
	if _, err := execute(t, []string{"config", "unset", "views.backlog.filter"}); err != nil {
		t.Fatal(err)
	}
	if _, err := execute(t, []string{"config", "get", "views.backlog.filter"}); exitCode(err) != exitNotFound {
		t.Errorf("get after unset = %v", err)
	}

	if _, err := execute(t, []string{"config", "set", "board.view", "classic"}); err != nil {
		t.Fatal(err)
	}
	if _, err := execute(t, []string{"config", "set", "board.view", "fancy"}); exitCode(err) != exitUsage {
		t.Errorf("a bad board.view = %v, want exit %d", err, exitUsage)
	}
	if _, err := execute(t, []string{"config", "set", "board.me", "x"}); exitCode(err) != exitUsage {
		t.Errorf("set board.me = %v, want exit %d", err, exitUsage)
	}
	if _, err := execute(t, []string{"config", "set", "board.nope", "x"}); exitCode(err) != exitUsage {
		t.Errorf("set of an unknown git key = %v, want exit %d", err, exitUsage)
	}
	if _, err := execute(t, []string{"config", "set", "board.datastore", "postgres://ann:s3cret@db/board"}); err != nil {
		t.Fatal(err)
	}
	out, err := execute(t, []string{"config", "list"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"fields.status.single = true", "board.view = classic  (local)", "board.me = "} {
		if !strings.Contains(out, want) {
			t.Errorf("list lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "s3cret") {
		t.Errorf("list prints the datastore's password:\n%s", out)
	}
	if out, _ := execute(t, []string{"config", "get", "board.datastore"}); strings.Contains(out, "s3cret") {
		t.Errorf("get prints the password: %s", out)
	}
	execute(t, []string{"config", "unset", "board.datastore"})
}

// -C runs a command in another repository, as git -C does.
func TestDirectoryFlag(t *testing.T) {
	newTestRepo(t)
	repo, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	out, err := execute(t, []string{"-C", repo, "ls"})
	if err != nil || !strings.Contains(out, "Fix invoice bug") {
		t.Fatalf("ls -C = %q, %v", out, err)
	}
	if _, err := execute(t, []string{"ls", "-C", "/nonexistent/dir"}); exitCode(err) != exitUsage {
		t.Errorf("-C on no directory = %v, want exit %d", err, exitUsage)
	}
}
