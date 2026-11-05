package code_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/guilbep/git-board/next/tools/code"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

// A Repo that keeps its reads reads the branches once, its copies
// sharing them; without, each read is git's, as it is now.
func TestKept(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "dev")
	git(t, dir, "commit", "-q", "--allow-empty", "-m", "init")
	git(t, dir, "branch", "fix/1-a")
	names := func(r code.Repo) []string {
		bs, err := r.Branches(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, b := range bs {
			out = append(out, b.Name)
		}
		return out
	}
	r := code.Repo{Dir: dir, Kept: code.NewKept()}
	copied := r
	if got := names(r); !slices.Equal(got, []string{"dev", "fix/1-a"}) {
		t.Fatalf("Branches = %v", got)
	}
	git(t, dir, "branch", "fix/2-b")
	if got := names(copied); !slices.Equal(got, []string{"dev", "fix/1-a"}) {
		t.Fatalf("a copy of a Repo that kept its reads = %v, want what it read", got)
	}
	if got := names(code.Repo{Dir: dir}); !slices.Equal(got, []string{"dev", "fix/1-a", "fix/2-b"}) {
		t.Fatalf("a Repo that keeps nothing = %v", got)
	}
}

// GrepFiles answers the same kept or not: kept, a regexp anchored on a
// literal is matched against the lines one git grep found for it.
func TestGrepFilesKept(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "dev")
	for name, text := range map[string]string{
		"plans/a.md": "---\nissue: 12\nstatus: draft\n---\n",
		"plans/b.md": "---\nissue: \"#12\"\n---\nissue: 13\n",
		"plans/c.md": "---\nissue: CO2-120\n---\n",
		"plans/d.md": "no front matter\n  issue: 12\n",
		"other/e.md": "issue: 12\n",
	} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "plans")
	kept := code.Repo{Dir: dir, Kept: code.NewKept()}
	for _, at := range []struct{ ref, dir string }{{"dev", "plans"}, {"dev", "other"}, {"dev", "none"}, {"nowhere", "plans"}} {
		for _, re := range []string{
			`^issue: *["']?([A-Za-z0-9]+-|#)?12["']? *$`,
			`^issue: *["']?([A-Za-z0-9]+-|#)?120["']? *$`,
			`^issue: *["']?13`,
			`^issue: *["']?abcdef0`,
			`issue: 12`, // not anchored: git's
			`^status|issue: 12$`,
		} {
			want, werr := code.Repo{Dir: dir}.GrepFiles(ctx, at.ref, at.dir, re)
			got, gerr := kept.GrepFiles(ctx, at.ref, at.dir, re)
			if !slices.Equal(got, want) || (gerr == nil) != (werr == nil) {
				t.Errorf("GrepFiles(%s, %s, %q) kept = %v, %v; want %v, %v", at.ref, at.dir, re, got, gerr, want, werr)
			}
		}
	}
}
