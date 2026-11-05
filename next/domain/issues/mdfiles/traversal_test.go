package mdfiles_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/guilbep/git-board/next/domain/issues"
	"github.com/guilbep/git-board/next/domain/issues/mdfiles"
	"github.com/guilbep/git-board/next/kernel"
	bsync "github.com/guilbep/git-board/next/sync"
)

// link writes a sync record linking id to a forge issue numbered number,
// as anyone who can push refs/board/sync/* can.
func link(t *testing.T, b *board, id, number string) {
	t.Helper()
	data, _ := json.Marshal(bsync.LinkData{Issue: id, ForgeID: "I_1", Number: number, URL: "https://github.com/o/r/issues/7"})
	if _, err := bsync.Kind.Create(ctx, b.svc.Board, bsync.StateID("github.com/o/r"), kernel.Op{Kind: "link", Author: string(b.svc.Me), Time: 2, Data: data}); err != nil {
		t.Fatal(err)
	}
}

// outside lists the files under root that aren't under in.
func outside(t *testing.T, root, in string) []string {
	t.Helper()
	var out []string
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".md") && !strings.HasPrefix(p, in+string(filepath.Separator)) {
			out = append(out, p)
		}
		return nil
	})
	return out
}

// A sync record's number that isn't digits never names a file: export
// --md and a Markdown folder write inside their folder, and the issue
// takes the number of its URL.
func TestNumberTraversal(t *testing.T) {
	t.Parallel()
	b := newBoard(t)
	var notices []string
	b.svc.Notice = func(format string, args ...any) { notices = append(notices, fmtf(format, args...)) }
	is, err := b.svc.New(ctx, issues.NewReq{Title: "x"})
	if err != nil {
		t.Fatal(err)
	}
	link(t, b, is.ID, "../../escaped")

	root := t.TempDir()
	dir := filepath.Join(root, "export", "sub")
	written, err := mdfiles.Export(ctx, b.svc, dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(written, []string{"7-x.md"}) {
		t.Fatalf("written %v, want [7-x.md]", written)
	}
	if out := outside(t, root, dir); len(out) != 0 {
		t.Fatalf("wrote outside the folder: %v", out)
	}
	if !slices.ContainsFunc(notices, func(n string) bool { return strings.Contains(n, `"../../escaped"`) }) {
		t.Fatalf("no warning of the number, notices %q", notices)
	}

	// the folder board writes its files on the next write
	if _, _, err := b.svc.Comment(ctx, is.ID, "a write"); err != nil {
		t.Fatal(err)
	}
	// the test's temporary folders all sit in one: ../../ of the board's
	// folder is in it
	filepath.WalkDir(filepath.Dir(filepath.Dir(b.dir)), func(p string, d os.DirEntry, err error) error {
		if err == nil && strings.Contains(d.Name(), "escaped") {
			t.Errorf("the folder wrote outside itself: %s", p)
		}
		return nil
	})
	b.file(t, "7-x.md")
}

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
