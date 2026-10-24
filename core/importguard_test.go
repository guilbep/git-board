package core

import (
	"errors"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

var forbiddenPrefixes = []string{
	"github.com/git-bug/git-bug",
	"github.com/guilbep/git-board/internal",
}

// forbidden returns the packages in go list's output that core must not
// import. With -test, a line can be "pkg [x.test]": pkg, built for x's
// tests.
func forbidden(out string) []string {
	var bad []string
	for _, p := range strings.Fields(out) {
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
	out, err := exec.Command("go", "list", "-deps", "-test", "./...").Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			t.Fatalf("go list: %v\n%s", err, ee.Stderr)
		}
		t.Fatalf("go list: %v", err)
	}
	if bad := forbidden(string(out)); len(bad) > 0 {
		t.Fatalf("core imports forbidden packages:\n%s", strings.Join(bad, "\n"))
	}
}

func TestGuardSeesForbiddenImports(t *testing.T) {
	t.Parallel()
	out := `fmt
github.com/git-bug/git-bug/cache
github.com/guilbep/git-board/internal/x [github.com/guilbep/git-board/core/issue.test]
github.com/guilbep/git-board/core/issue [github.com/guilbep/git-board/core/issue.test]
github.com/guilbep/git-board/core/issue.test
`
	want := []string{"github.com/git-bug/git-bug/cache", "github.com/guilbep/git-board/internal/x"}
	if bad := forbidden(out); !slices.Equal(bad, want) {
		t.Fatalf("forbidden = %v, want %v", bad, want)
	}
}
