package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A command-line reset refuses while a server runs on the repository: it
// would empty the board under the server's feet. Serial: it chdirs.
func TestResetRefusesWhileServed(t *testing.T) {
	dir, b := seedRepo(t)
	gitDir := b.Store().CommonDir()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"gitDir": gitDir})
	}))
	t.Cleanup(ts.Close)
	if err := os.MkdirAll(filepath.Join(gitDir, "git-board"), 0o755); err != nil {
		t.Fatal(err)
	}
	inst := fmt.Sprintf(`{"url": %q, "pid": %d}`, ts.URL, os.Getpid())
	if err := os.WriteFile(filepath.Join(gitDir, "git-board", "server.json"), []byte(inst), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	out, err := execute(t, []string{"reset", "--hard"})
	if err == nil || !strings.Contains(err.Error(), "git board serve runs on this repository ("+ts.URL+")") {
		t.Fatalf("err = %v\n%s", err, out)
	}
}

func TestResetNeedsHard(t *testing.T) {
	dir, _ := seedRepo(t)
	t.Chdir(dir)
	out, err := execute(t, []string{"reset"})
	if err == nil || !strings.Contains(err.Error(), "only reset --hard exists") {
		t.Fatalf("err = %v\n%s", err, out)
	}
}

func TestResetHardCommand(t *testing.T) {
	dir, b := seedRepo(t)
	seedBug(t, dir, "goes")
	t.Chdir(dir)
	out, err := execute(t, []string{"reset", "--hard"})
	if err != nil || !strings.Contains(out, "reset: ") || !strings.Contains(out, "this clone's board is empty") {
		t.Fatalf("err = %v\n%s", err, out)
	}
	if is, _ := b.ListIssues(); len(is) != 0 {
		t.Errorf("issues left: %v", is)
	}
}
