package board

import (
	"bufio"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// gitBoardNext is git-board v2's binary: GIT_BOARD_NEXT, else built from
// the git-board checkout this module sits in (contrib/board-agents), else
// the test is skipped (the module on its own, or -short).
func gitBoardNext(t *testing.T) string {
	t.Helper()
	if bin := os.Getenv("GIT_BOARD_NEXT"); bin != "" {
		return bin
	}
	if testing.Short() {
		t.Skip("-short: no git-board-next built")
	}
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "next", "cmd", "git-board-next")); err != nil {
		t.Skip("no git-board checkout around this module, and no GIT_BOARD_NEXT")
	}
	bin := filepath.Join(t.TempDir(), "git-board-next")
	cmd := exec.Command("go", "build", "-o", bin, "./next/cmd/git-board-next")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building git-board-next: %v\n%s", err, out)
	}
	return bin
}

// TestV2 runs the client against git-board-next serve, on a throwaway
// board with the cycle on: one issue in progress on its branch.
func TestV2(t *testing.T) {
	t.Parallel()
	bin := gitBoardNext(t)
	ctx := context.Background()
	dir := t.TempDir()
	run := func(name string, args ...string) string {
		t.Helper()
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s %v: %v\n%s", name, args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	run("git", "init", "-q", "-b", "main")
	run("git", "commit", "-q", "--allow-empty", "-m", "init")
	toml := "[fields.status]\nsingle = true\noptions = [\"backlog\", \"ready\", \"in-progress\", \"in-review\", \"done\"]\n\n[experimental]\nfeatures = [\"cycle\"]\n"
	if err := os.WriteFile(filepath.Join(dir, ".git-board.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	run(bin, "me", "--new", "--name", "Ada", "--email", "ada@example.com")
	id := run(bin, "new", "Export CSV", "-b", "The last row.")
	c := &Client{Bin: bin, Dir: dir}
	branch, err := c.Branch(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	run("git", "switch", "-q", "-c", branch)
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("git", "add", "a.txt")
	run("git", "commit", "-q", "-m", "a")
	run("git", "switch", "-q", "main")
	if err := c.SetStatus(ctx, id, "in-progress"); err != nil {
		t.Fatal(err)
	}

	srv := exec.Command(bin, "serve", "--addr", "127.0.0.1:0")
	srv.Dir = dir
	stdout, err := srv.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Process.Signal(os.Interrupt); _ = srv.Wait() })
	listening := make(chan string, 1)
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			if u, ok := strings.CutPrefix(sc.Text(), "git-board listening on "); ok {
				listening <- u
			}
		}
	}()
	select {
	case u := <-listening:
		c.URL = u
	case <-time.After(30 * time.Second):
		t.Fatal("git-board-next serve: not listening after 30s")
	}

	if u := Discover(ctx, dir); u != c.URL {
		t.Errorf("Discover = %q, want %q", u, c.URL)
	}
	cyc, err := c.Cycle(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if cyc.Name != "Tests" || cyc.Stage != "in-progress" || cyc.Evidence.Branch != branch || cyc.Version == "" ||
		cyc.Evidence.Stages.Ready != "ready" || len(cyc.Evidence.Stages.InProgress) == 0 || cyc.Position() == 0 {
		t.Fatalf("cycle = %+v", cyc)
	}
	agents, err := c.Agents(ctx)
	if err != nil || len(agents) != 1 || agents[0].Branch != branch || agents[0].Issue.ID != id || agents[0].Name != "Tests" || agents[0].State == "" {
		t.Fatalf("agents = %+v, %v", agents, err)
	}
	w, err := c.Comment(ctx, id, "Suggested: **low**", "risk", cyc.Version)
	if err != nil || w.Note != NoStep || w.ID == "" {
		t.Fatalf("comment: %+v, %v", w, err)
	}
	if _, err := c.Comment(ctx, id, "late", "", cyc.Version); !errors.Is(err, ErrStale) {
		t.Fatalf("a comment on the version before: %v", err)
	}
	th, err := c.ReviewThread(ctx, id)
	if err != nil || th.ID != id || th.Version == "" || th.Version == cyc.Version || th.Description() != "The last row." {
		t.Fatalf("thread = %+v, %v", th, err)
	}
	if last := th.Events[len(th.Events)-1]; last.Body != "Suggested: **low**" || !strings.HasPrefix(last.ID, w.ID) && last.ID != w.ID {
		t.Errorf("the comment written: %+v, Location %q", last, w.ID)
	}
	if _, err := c.ReviewThread(ctx, "0000000"); !errors.Is(err, ErrNotFound) {
		t.Errorf("no such issue: %v", err)
	}
}
