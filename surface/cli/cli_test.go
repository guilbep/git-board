package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guilbep/git-board/domain/issues"
	"github.com/guilbep/git-board/domain/people"
	"github.com/guilbep/git-board/kernel"
	"github.com/guilbep/git-board/store/git"
	"github.com/guilbep/git-board/view"
)

var ctx = context.Background()

func op(kind string, time int64, author kernel.ID, data string) kernel.Op {
	return kernel.Op{Kind: kind, Author: string(author), Time: time, Data: json.RawMessage(data)}
}

// newBoard is a repository with a board written through the kernel, as M2's
// commands will write one: two people, three issues, one deleted.
func newBoard(t *testing.T) (dir string, ids []kernel.ID) {
	t.Helper()
	dir = t.TempDir()
	if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	ds, err := git.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	v, err := view.Open(ctx, filepath.Join(t.TempDir(), "view.db"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := kernel.Open(ctx, ds, v, issues.Kind, people.Kind)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	ada, err := people.Kind.Create(ctx, b, "", op("create", 1700000000, "", `{"name":"Ada","email":"ada@example.com"}`),
		op("add-account", 1700000000, "", `{"server":"github.com","id":"42","login":"ada"}`))
	if err != nil {
		t.Fatal(err)
	}
	a := ada.ID
	for i, title := range []string{"Login fails", "Export crashes", "Gone"} {
		e, err := issues.Kind.Create(ctx, b, "", op("create", 1700000100+int64(i), a, fmt.Sprintf(`{"title":%q,"body":"Body %d"}`, title, i)),
			op("change-labels", 1700000100+int64(i), a, `{"add":["prio:p`+fmt.Sprint(i)+`","bug"]}`))
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, e.ID)
	}
	if _, err := issues.Kind.Do(ctx, b, ids[0], func(kernel.Entity[issues.State]) ([]kernel.Op, error) {
		return []kernel.Op{op("comment", 1700000200, a, `{"body":"Seen on staging"}`), op("set-status", 1700000300, a, `{"status":"closed"}`)}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := issues.Kind.Do(ctx, b, ids[2], func(e kernel.Entity[issues.State]) ([]kernel.Op, error) {
		return []kernel.Op{op("delete", 1700000400, a, `{"target":"`+string(e.State.BodyOp)+`"}`)}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", dir, "config", "board.me", string(a)).CombinedOutput(); err != nil {
		t.Fatalf("git config: %v: %s", err, out)
	}
	return dir, ids
}

func run(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	var out, errs bytes.Buffer
	code := Main(ctx, args, &out, &errs)
	return out.String(), errs.String(), code
}

// The commands, end to end: they chdir (-C), so they run one at a time.
func TestCommands(t *testing.T) {
	dir, ids := newBoard(t)
	t.Chdir(dir) // and back after: -C leaves the process there
	short := func(i int) string { return string(ids[i][:8]) }
	for _, c := range []struct {
		args []string
		code int
		has  []string
		not  []string
	}{
		{[]string{"ls"}, 0, []string{"id  ", "Export crashes", "open", "bug"}, []string{"Login fails", "Gone"}},
		{[]string{"ls", ""}, 0, []string{"Login fails", "closed", "Export crashes"}, []string{"Gone"}},
		{[]string{"ls", "is:closed", "--json", "id,comments"}, 0, []string{`"comments": 1`, string(ids[0])}, []string{"title"}},
		{[]string{"ls", "seen staging"}, 0, nil, []string{"Export"}},
		{[]string{"ls", "seen staging is:closed"}, 0, []string{"Login fails"}, nil},
		{[]string{"ls", "author:ada prio:p1"}, 0, []string{"Export crashes"}, []string{"Login"}},
		{[]string{"ls", "assignee:@me"}, 0, nil, []string{"Export"}},
		{[]string{"ls", "", "--sort", "prio desc", "--jq", ".[].title"}, 0, []string{"Export crashes\nLogin fails\n"}, nil},
		{[]string{"ls", "--sort", "nope"}, 2, nil, nil},
		{[]string{"ls", "--json=nope"}, 2, nil, nil},
		{[]string{"ls", "--nope"}, 2, nil, nil},
		{[]string{"show", short(0)}, 0, []string{"Login fails\n", "closed · opened by Ada on ", "1 comment", "Prio:", "labels:", "Body 0", "--- Ada, "}, nil},
		{[]string{"show", short(0), "--json", "--jq", ".participants[0].avatar_url"}, 0, []string{"https://avatars.githubusercontent.com/u/42?s=88"}, nil},
		{[]string{"show", short(2)}, 3, nil, nil},
		{[]string{"show", "0000000"}, 3, nil, nil},
		{[]string{"show", "#12"}, 3, nil, nil},
		{[]string{"log", short(0)}, 0, []string{`created "Login fails"`, "+prio:p0 +bug", "commented [", "closed"}, nil},
		{[]string{"log", short(0), "--field", "prio", "--json", "added"}, 0, []string{`"prio:p0"`}, []string{"bug"}},
		{[]string{"log", short(0), "--since", "nope"}, 2, nil, nil},
	} {
		out, errs, code := run(t, append([]string{"-C", dir}, c.args...)...)
		if code != c.code {
			t.Errorf("%v: exit %d, want %d: %s", c.args, code, c.code, errs)
			continue
		}
		for _, h := range c.has {
			if !strings.Contains(out, h) {
				t.Errorf("%v: no %q in\n%s", c.args, h, out)
			}
		}
		for _, n := range c.not {
			if strings.Contains(out, n) {
				t.Errorf("%v: %q in\n%s", c.args, n, out)
			}
		}
	}
}

// writeColumns writes what text/tabwriter writes.
func TestColumnsAsTabwriter(t *testing.T) {
	t.Parallel()
	for _, rows := range [][][]string{
		{{"id", "title", "other"}, {"a", "café crème", ""}, {"bb", "x", "bug, z"}},
		{{"a", "b"}, {"with\ttab", "x"}},
		{{"a", "b", "c"}, {"short"}},
		{{"bad \xff byte", "b"}, {"c", "d"}},
	} {
		var got, want bytes.Buffer
		writeColumns(&got, rows)
		tabs(&want, rows)
		if got.String() != want.String() {
			t.Errorf("writeColumns(%q) =\n%q\ntabwriter:\n%q", rows, got.String(), want.String())
		}
	}
}
