package code_test

import (
	"context"
	"os/exec"
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
