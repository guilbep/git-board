package golden

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestDogfood is the dogfooding job (design.md, section 5): a real board,
// read by two binaries, the release before and this build, must print the
// same ls --json, and show --json of every issue, deleted ones too. It
// runs only when GIT_BOARD_DOGFOOD_DIR names a repository (make dogfood,
// .github/workflows/dogfood.yml), with GIT_BOARD_DOGFOOD_OLD and
// GIT_BOARD_DOGFOOD_NEW the binaries. GIT_BOARD_DOGFOOD_OUT, when set, is
// a folder that receives each differing case's outputs, in old/ and new/.
//
// Each binary reads its own copy of the board: a clone of the
// repository's checked-out commit with its .git-board.toml as it is in
// the working tree, and its refs/board/*, and no remote. The repository
// itself is never written, and nothing is fetched or pushed. Outputs are
// compared as make compat compares them (Run, Normalise, Env): exit code
// and stdout; stderr is kept in the files, not compared (v2 words its
// errors anew, compat.md).
func TestDogfood(t *testing.T) {
	dir := os.Getenv("GIT_BOARD_DOGFOOD_DIR")
	if dir == "" {
		t.Skip("GIT_BOARD_DOGFOOD_DIR names no repository (make dogfood)")
	}
	bins := map[string]string{"old": os.Getenv("GIT_BOARD_DOGFOOD_OLD"), "new": os.Getenv("GIT_BOARD_DOGFOOD_NEW")}
	for side, bin := range bins {
		if bin == "" {
			t.Fatalf("GIT_BOARD_DOGFOOD_%s names no binary", strings.ToUpper(side))
		}
		abs, err := filepath.Abs(bin)
		if err != nil {
			t.Fatal(err)
		}
		bins[side] = abs
	}
	out := os.Getenv("GIT_BOARD_DOGFOOD_OUT")
	ctx := context.Background()
	if ds, _ := exec.CommandContext(ctx, "git", "-C", dir, "config", "board.datastore").Output(); strings.TrimSpace(string(ds)) != "" {
		t.Fatalf("%s keeps its board in %s: the dogfooding reads boards kept in git", dir, strings.TrimSpace(string(ds)))
	}

	root := t.TempDir()
	checkouts := map[string]string{}
	for side := range bins {
		checkouts[side] = filepath.Join(root, side)
		if err := copyBoard(ctx, dir, checkouts[side]); err != nil {
			t.Fatal(err)
		}
	}
	ids, err := Board{}.IssueIDs(ctx, checkouts["old"])
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) == 0 {
		t.Fatalf("%s has no issue in refs/board/issue/: fetch them first (git fetch origin 'refs/board/*:refs/board/*')", dir)
	}
	cases := []Case{{"ls", "ls --json"}, {"ls-closed", "ls is:closed --json"}}
	for _, id := range ids {
		cases = append(cases, Case{"show-" + id[:12], "show " + id + " --json"})
	}
	t.Logf("%s: %d issues, %d cases", dir, len(ids), len(cases))

	differ := 0
	for _, c := range cases {
		res := map[string]Result{}
		for side, bin := range bins {
			home := filepath.Join(root, "home-"+side)
			if err := os.MkdirAll(home, 0o755); err != nil {
				t.Fatal(err)
			}
			r, err := Run(ctx, bin, checkouts[side], home, c)
			if err != nil {
				t.Fatalf("%s %s: %v", side, c.Line, err)
			}
			res[side] = r
		}
		o, n := res["old"], res["new"]
		if o.Exit == n.Exit && bytes.Equal(o.Stdout, n.Stdout) {
			continue
		}
		differ++
		t.Errorf("%s: exit %d, %d; stdout differs at line %d", c.Line, o.Exit, n.Exit, firstDiff(o.Stdout, n.Stdout))
		if out != "" {
			for side, r := range res {
				if err := writeResult(filepath.Join(out, side), c.Name, r); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	if differ > 0 {
		t.Logf("%d of %d cases differ", differ, len(cases))
	}
}

// copyBoard makes at dst the copy of the board at src the binaries read:
// a clone of its commit, its working tree's .git-board.toml, its
// refs/board/*, no remote.
func copyBoard(ctx context.Context, src, dst string) error {
	git := func(dir string, args ...string) error {
		cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
		cmd.Env = Env(filepath.Dir(dst))
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return nil
	}
	abs, err := filepath.Abs(src)
	if err != nil {
		return err
	}
	if err := git(filepath.Dir(dst), "clone", "-q", "--no-tags", abs, dst); err != nil {
		return err
	}
	if err := git(dst, "fetch", "-q", abs, "+refs/board/*:refs/board/*"); err != nil {
		return err
	}
	if err := git(dst, "remote", "remove", "origin"); err != nil {
		return err
	}
	toml, err := os.ReadFile(filepath.Join(abs, ".git-board.toml"))
	switch {
	case os.IsNotExist(err): // as the working tree has it: none
		if err := os.Remove(filepath.Join(dst, ".git-board.toml")); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	case err != nil:
		return err
	}
	return os.WriteFile(filepath.Join(dst, ".git-board.toml"), toml, 0o644)
}

// firstDiff is the first line, from 1, where a and b differ.
func firstDiff(a, b []byte) int {
	la, lb := bytes.Split(a, []byte("\n")), bytes.Split(b, []byte("\n"))
	for i := range min(len(la), len(lb)) {
		if !bytes.Equal(la[i], lb[i]) {
			return i + 1
		}
	}
	return min(len(la), len(lb)) + 1
}

// writeResult writes r in dir as <name>.out, <name>.err and <name>.exit.
func writeResult(dir, name string, r Result) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for ext, data := range map[string][]byte{".out": r.Stdout, ".err": r.Stderr, ".exit": fmt.Appendf(nil, "%d\n", r.Exit)} {
		if err := os.WriteFile(filepath.Join(dir, name+ext), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}
