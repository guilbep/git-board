package core

import (
	"os/exec"
	"strings"
	"testing"
)

var forbiddenPrefixes = []string{
	"github.com/git-bug/git-bug",
	"github.com/guilbep/git-board/internal",
}

// forbidden returns the packages that core must not import.
func forbidden(pkgs []string) []string {
	var bad []string
	for _, p := range pkgs {
		for _, prefix := range forbiddenPrefixes {
			if strings.HasPrefix(p, prefix) {
				bad = append(bad, p)
				break
			}
		}
	}
	return bad
}

func TestCoreImportsNoGitBugNorInternal(t *testing.T) {
	t.Parallel()
	out, err := exec.Command("go", "list", "-deps", "./...").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	if bad := forbidden(strings.Fields(string(out))); len(bad) > 0 {
		t.Fatalf("core imports forbidden packages:\n%s", strings.Join(bad, "\n"))
	}
}

func TestGuardSeesForbiddenImports(t *testing.T) {
	t.Parallel()
	in := []string{"fmt", "github.com/git-bug/git-bug/cache", "github.com/guilbep/git-board/internal/x", "github.com/guilbep/git-board/core"}
	if bad := forbidden(in); len(bad) != 2 {
		t.Fatalf("forbidden(%v) = %v, want the git-bug and internal packages", in, bad)
	}
}
