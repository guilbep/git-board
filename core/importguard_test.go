package core

import (
	"errors"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// gitBug is git-bug's modules: nothing in git-board may import them.
const gitBug = "github.com/git-bug/"

// forbidden returns the packages in go list's output that start with one
// of prefixes. With -test, a line can be "pkg [x.test]": pkg, built for
// x's tests.
func forbidden(out string, prefixes ...string) []string {
	var bad []string
	for _, p := range strings.Fields(out) {
		for _, prefix := range prefixes {
			if strings.HasPrefix(p, prefix) {
				bad = append(bad, p)
				break
			}
		}
	}
	return bad
}

// deps is go list's packages of patterns and all they import, tests too.
func deps(t *testing.T, patterns ...string) string {
	t.Helper()
	out, err := exec.Command("go", append([]string{"list", "-deps", "-test"}, patterns...)...).Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			t.Fatalf("go list: %v\n%s", err, ee.Stderr)
		}
		t.Fatalf("go list: %v", err)
	}
	return string(out)
}

func TestCoreImportsNoGitBugNorInternal(t *testing.T) {
	t.Parallel()
	if bad := forbidden(deps(t, "./..."), gitBug, "github.com/guilbep/git-board/internal"); len(bad) > 0 {
		t.Fatalf("core imports forbidden packages:\n%s", strings.Join(bad, "\n"))
	}
}

// Nothing in the module imports git-bug: git-board left it in M5.
func TestModuleImportsNoGitBug(t *testing.T) {
	t.Parallel()
	if bad := forbidden(deps(t, "github.com/guilbep/git-board/..."), gitBug); len(bad) > 0 {
		t.Fatalf("the module imports git-bug:\n%s", strings.Join(bad, "\n"))
	}
}

func TestGuardSeesForbiddenImports(t *testing.T) {
	t.Parallel()
	out := `fmt
github.com/git-bug/git-bug/cache
github.com/git-bug/gitconfig [github.com/guilbep/git-board/internal/x.test]
github.com/guilbep/git-board/internal/x [github.com/guilbep/git-board/core/issue.test]
github.com/guilbep/git-board/core/issue [github.com/guilbep/git-board/core/issue.test]
github.com/guilbep/git-board/core/issue.test
`
	want := []string{"github.com/git-bug/git-bug/cache", "github.com/git-bug/gitconfig", "github.com/guilbep/git-board/internal/x"}
	if bad := forbidden(out, gitBug, "github.com/guilbep/git-board/internal"); !slices.Equal(bad, want) {
		t.Fatalf("forbidden = %v, want %v", bad, want)
	}
}
