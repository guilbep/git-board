package mdfiles_test

import (
	"strings"
	"testing"

	"github.com/guilbep/git-board/next/domain/issues"
	"github.com/guilbep/git-board/next/domain/issues/mdfiles"
)

// Name, which export --md and a Markdown folder both name their files
// by, joins a number and an id read from board data to a folder; the
// sync records' end of it is next/app's TestExportNumberTraversal.
// Name never answers anything but one clean file name, whatever the
// issue holds.
func TestNameIsOneElement(t *testing.T) {
	t.Parallel()
	id := strings.Repeat("ab", 20)
	for _, number := range []string{"../../escaped", "a/b", `a\b`, "..", ".", "1/2", "", "12x", "-1"} {
		is := issues.Issue{ID: id, Title: "../../t/../", Forges: []issues.Forge{{Name: "GitHub", Number: number}}}
		if name, err := mdfiles.Name(is); err != nil || name != id[:7]+"-t.md" {
			t.Fatalf("number %q: named %q, %v", number, name, err)
		}
	}
	if name, err := mdfiles.Name(issues.Issue{ID: id, Title: "t", Forges: []issues.Forge{{Number: "42"}}}); err != nil || name != "42-t.md" {
		t.Fatalf("named %q, %v", name, err)
	}
	for _, bad := range []string{"../../../../../../../x", "ab/cdef0123456789", "short", ""} {
		if name, err := mdfiles.Name(issues.Issue{ID: bad, Title: "t"}); err == nil {
			t.Fatalf("id %q named %q, want refused", bad, name)
		}
	}
}
