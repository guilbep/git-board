package app

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	gosync "sync"
	"testing"

	"github.com/guilbep/git-board/domain/issues"
	"github.com/guilbep/git-board/domain/issues/issuesync"
	"github.com/guilbep/git-board/domain/issues/mdfiles"
	"github.com/guilbep/git-board/kernel"
	"github.com/guilbep/git-board/sync"
)

// A sync record's number is board data: anyone who can push
// refs/board/sync/* writes it. One that isn't digits never names a file:
// export --md and a Markdown folder's render write inside their folder,
// the issue taking its URL's number, and the read says so (review
// finding 1: "../../escaped" wrote <dir>/../../escaped-x.md).
func TestExportNumberTraversal(t *testing.T) {
	t.Parallel()
	for _, folder := range []bool{false, true} {
		root := t.TempDir()
		dir := filepath.Join(root, "repo")
		args := [][]string{{"init", "-q", dir}}
		if folder {
			args = append(args, []string{"-C", dir, "config", "board.datastore", "markdown:issues"})
		}
		for _, a := range args {
			if out, err := exec.Command("git", a...).CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v\n%s", a, err, out)
			}
		}
		var mu gosync.Mutex
		var notices []string
		b, err := Open(t.Context(), dir, Options{Notice: func(format string, args ...any) {
			mu.Lock()
			defer mu.Unlock()
			notices = append(notices, strings.TrimSpace(fmt.Sprintf(format, args...)))
		}})
		if err != nil {
			t.Fatal(err)
		}
		defer b.Close()
		me, err := issuesync.CreatePerson(t.Context(), b.Kernel, issuesync.NewPerson("", "me", ""))
		if err != nil {
			t.Fatal(err)
		}
		b.Issues.Me = kernel.ID(me.ID)
		is, err := b.Issues.New(t.Context(), issues.NewReq{Title: "x"})
		if err != nil {
			t.Fatal(err)
		}
		data, _ := json.Marshal(sync.LinkData{Issue: is.ID, ForgeID: "I_1", Number: "../../escaped", URL: "https://github.com/o/r/issues/7"})
		if _, err := sync.Kind.Create(t.Context(), b.Kernel, sync.StateID("github.com/o/r"), kernel.Op{Kind: "link", Author: me.ID, Time: 2, Data: data}); err != nil {
			t.Fatal(err)
		}
		out := filepath.Join(root, "export", "sub")
		written, err := mdfiles.Export(t.Context(), b.Issues, out, nil)
		if err != nil || !slices.Equal(written, []string{"7-x.md"}) {
			t.Fatalf("folder %t: written %v, %v; want [7-x.md]", folder, written, err)
		}
		mu.Lock()
		said := slices.Clone(notices)
		mu.Unlock()
		if !slices.ContainsFunc(said, func(n string) bool { return strings.Contains(n, `"../../escaped"`) }) {
			t.Errorf("folder %t: no notice of the number: %q", folder, said)
		}
		if folder { // a write renders the folder
			if _, _, err := b.Issues.Comment(t.Context(), is.ID, "a write"); err != nil {
				t.Fatal(err)
			}
		}
		filepath.WalkDir(filepath.Dir(root), func(p string, d fs.DirEntry, err error) error {
			if err == nil && strings.Contains(d.Name(), "escaped") {
				t.Errorf("folder %t: wrote outside: %s", folder, p)
			}
			return nil
		})
		if folder {
			if files, _ := filepath.Glob(filepath.Join(dir, "issues", "7-x.md")); len(files) != 1 {
				t.Errorf("the folder's file isn't 7-x.md")
			}
		}
	}
}
