package conf

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// GIT_BOARD_GOLDEN_UPDATE=1 writes the migrations' expected files again.
var update = os.Getenv("GIT_BOARD_GOLDEN_UPDATE") != ""

func TestMigrationsInOrder(t *testing.T) {
	t.Parallel()
	for i, m := range Migrations {
		if m.From != i+1 || m.To != i+2 || m.Desc == "" || m.Up == nil || m.Down == nil {
			t.Errorf("Migrations[%d] = %d to %d %q: the i-th goes from i+1 to i+2, described, both ways", i, m.From, m.To, m.Desc)
		}
	}
	if last := Migrations[len(Migrations)-1].To; last != Current {
		t.Errorf("Current = %d, the last migration goes to %d", Current, last)
	}
}

// Each testdata/schema/<name>.v1.toml is upgraded to every version, its
// <name>.v<N>.toml, and downgraded back from each: byte for byte the
// file it was, unless a migration on the way is lossy.
func TestMigrateBothWays(t *testing.T) {
	t.Parallel()
	files, _ := filepath.Glob("testdata/schema/*.v1.toml")
	if len(files) == 0 {
		t.Fatal("no files")
	}
	for _, path := range files {
		name := strings.TrimSuffix(filepath.Base(path), ".v1.toml")
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			v1, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			lossless := true
			for to := 2; to <= Current; to++ {
				got, steps, err := Migrate(string(v1), to)
				if err != nil {
					t.Fatalf("up to %d: %v", to, err)
				}
				if len(steps) != to-1 {
					t.Errorf("up to %d: %d steps", to, len(steps))
				}
				if v, err := Version(got); v != to || err != nil {
					t.Errorf("up to %d: the file says %d, %v", to, v, err)
				}
				lossless = lossless && Migrations[to-2].Lossy == ""
				want := filepath.Join("testdata/schema", name+".v"+strconv.Itoa(to)+".toml")
				if update {
					if err := os.WriteFile(want, []byte(got), 0o644); err != nil {
						t.Fatal(err)
					}
				}
				if w, err := os.ReadFile(want); err != nil || string(w) != got {
					t.Errorf("up to %d: %v\n%s", to, err, Diff(want, "got", string(w), got))
				}
				back, _, err := Migrate(got, 1)
				if err != nil {
					t.Fatalf("down from %d: %v", to, err)
				}
				if lossless && back != string(v1) {
					t.Errorf("down from %d isn't the file:\n%s", to, Diff("v1", "back", string(v1), back))
				}
				if f, err := Parse(FileName, v1); err != nil || f.Version() != 1 || string(f.Source()) != string(v1) {
					t.Errorf("Parse: %v", err)
				} else if strings.Count(string(f.Text()), "\n") != strings.Count(string(v1), "\n") {
					t.Error("the read in memory moved the file's lines: a message would name the wrong one")
				}
			}
		})
	}
}

func TestMigration2(t *testing.T) {
	t.Parallel()
	const v1 = "[sprints]\nlength_days = 14\nstart_weekday = \"monday\"  # the team's\n\n[repos]\nlist = [\"a/b\"]\nassignees = \"labels\"\n"
	const v2 = "schema = 2\n\n[sprints]\nlength_days = 14\n# start_weekday = \"monday\"  # the team's  # " + unread + "\n\n[repos]\nlist = [\"a/b\"]\n# assignees = \"labels\"  # " + unread + "\n"
	got, _, err := Migrate(v1, 2)
	if err != nil || got != v2 {
		t.Fatalf("up: %v\n%s", err, Diff("want", "got", v2, got))
	}
	if back, _, err := Migrate(got, 1); err != nil || back != v1 {
		t.Errorf("down: %v\n%s", err, Diff("want", "got", v1, back))
	}
	// read: the keys aren't there, the lines are
	f, err := Parse(FileName, []byte(v1))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(f.Text()), "\nstart_weekday") || f.Version() != 1 {
		t.Errorf("read in memory:\n%s", f.Text())
	}
	// a form the edit doesn't change is refused, never missed
	if _, _, err := Migrate("sprints = { start_weekday = \"monday\" }\n", 2); err == nil || !strings.Contains(err.Error(), "schema 1 to 2: sprints.start_weekday: written in a form") {
		t.Errorf("inline: %v", err)
	}
	// a key set again by hand, then a downgrade: the file would set it twice
	if _, _, err := Migrate("schema = 2\n[repos]\nassignees = \"native\"\n# assignees = \"labels\"  # "+unread+"\n", 1); err == nil {
		t.Error("down to a key set twice")
	}
}

func TestVersion(t *testing.T) {
	t.Parallel()
	for text, want := range map[string]int{"": 1, "[a]\nb = 1\n": 1, "schema = 2\n": 2, "schema=7 # x\n[a]\n": 7} {
		if v, err := Version(text); v != want || err != nil {
			t.Errorf("Version(%q) = %d, %v; want %d", text, v, err, want)
		}
	}
	for _, text := range []string{"schema = \"2\"\n", "schema = 0\n", "schema = 1.5\n", "[a\n"} {
		if _, err := Version(text); err == nil {
			t.Errorf("Version(%q) takes it", text)
		}
	}
	if _, err := Parse(FileName, []byte("schema = \"two\"\n")); err == nil || !strings.Contains(err.Error(), "schema = \"two\": the file's schema version is a whole number from 1") {
		t.Errorf("Parse: %v", err)
	}
	if f, err := Load(filepath.Join(t.TempDir(), FileName)); err != nil || f.Version() != 0 {
		t.Errorf("no file: version %d, %v", f.Version(), err)
	}
}

// The key goes after the file's opening comment, else first, and taking
// it out gives the file back.
func TestSetVersion(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"":                       "schema = 2\n",
		"[a]\nb = 1\n":           "schema = 2\n\n[a]\nb = 1\n",
		"[a]\nb = 1":             "schema = 2\n\n[a]\nb = 1",
		"# the board\n\n[a]\n":   "# the board\n\nschema = 2\n\n[a]\n",
		"# a's\n[a]\n":           "schema = 2\n\n# a's\n[a]\n",
		"# only a comment\n":     "schema = 2\n\n# only a comment\n",
		"# h\n#\n\n\n[a]\n":      "# h\n#\n\nschema = 2\n\n\n[a]\n",
		"schema = 1  # kept\n":   "schema = 2  # kept\n",
		"top = 1\n[a]\n":         "schema = 2\n\ntop = 1\n[a]\n",
		"# h\n\nschema = 3\n[a]": "# h\n\nschema = 2\n[a]",
	} {
		got := setVersion(in, 2)
		if got != want {
			t.Errorf("setVersion(%q) = %q, want %q", in, got, want)
		}
		if !strings.Contains(in, "schema") {
			if back := setVersion(got, 1); back != in {
				t.Errorf("taken out of %q: %q", got, back)
			}
		}
	}
}

func TestNewer(t *testing.T) {
	t.Parallel()
	const text = "schema = 3\n[sprints]\nlength_days = 14\n"
	var newer *NewerError
	if _, _, err := Migrate(text, 2); !errors.As(err, &newer) || newer.File != 3 {
		t.Errorf("Migrate: %v", err)
	}
	if _, from, err := ForWrite(text); !errors.As(err, &newer) || from != 3 ||
		!strings.Contains(err.Error(), ".git-board.toml is schema 3, and this git-board knows schema 2 at most") {
		t.Errorf("ForWrite: %v", err)
	}
	f, err := Parse(FileName, []byte(text)) // read as it is
	if err != nil || f.Version() != 3 || string(f.Text()) != text {
		t.Errorf("Parse: %v, %d", err, f.Version())
	}
	if out, from, err := ForWrite("[a]\n"); err != nil || from != 1 || out != "schema = 2\n\n[a]\n" {
		t.Errorf("ForWrite of v1: %q, %d, %v", out, from, err)
	}
}

func TestSteps(t *testing.T) {
	t.Parallel()
	if s, err := Steps(1, Current); err != nil || len(s) != Current-1 || s[0].From != 1 {
		t.Errorf("up: %v, %v", s, err)
	}
	if s, err := Steps(Current, 1); err != nil || len(s) != Current-1 || s[0].To != Current {
		t.Errorf("down: %v, %v", s, err)
	}
	for _, v := range []int{0, Current + 1} {
		if _, err := Steps(1, v); err == nil {
			t.Errorf("Steps(1, %d) takes it", v)
		}
	}
}

// Each edit, then its inverse, gives the file back.
func TestEdits(t *testing.T) {
	t.Parallel()
	const file = "# top\n[a]  # a's\nx = 1  # one\ny = \"s\"\n\n[a.sub]\nz = true\n[[a.list]]\nw = 2\n\n[b]\nv = 3\n"
	for _, c := range []struct {
		name     string
		up, down Edit
		want     string
	}{
		{"rename a key", RenameKey("a", "x", "ex"), RenameKey("a", "ex", "x"),
			"# top\n[a]  # a's\nex = 1  # one\ny = \"s\"\n\n[a.sub]\nz = true\n[[a.list]]\nw = 2\n\n[b]\nv = 3\n"},
		{"move a key", MoveKey("b", "a.sub", "v"), MoveKey("a.sub", "b", "v"),
			"# top\n[a]  # a's\nx = 1  # one\ny = \"s\"\n\n[a.sub]\nz = true\nv = 3\n[[a.list]]\nw = 2\n\n[b]\n"},
		{"rename a table", RenameTable("a", "c"), RenameTable("c", "a"),
			"# top\n[c]  # a's\nx = 1  # one\ny = \"s\"\n\n[c.sub]\nz = true\n[[c.list]]\nw = 2\n\n[b]\nv = 3\n"},
		{"map a value", MapValue("a", "x", func(v string) (string, error) { return v + "0", nil }),
			MapValue("a", "x", func(v string) (string, error) { return strings.TrimSuffix(v, "0"), nil }),
			"# top\n[a]  # a's\nx = 10  # one\ny = \"s\"\n\n[a.sub]\nz = true\n[[a.list]]\nw = 2\n\n[b]\nv = 3\n"},
		{"comment a key out", CommentKey("a", "y", "why"), UncommentKey("a", "y", "why"),
			"# top\n[a]  # a's\nx = 1  # one\n# y = \"s\"  # why\n\n[a.sub]\nz = true\n[[a.list]]\nw = 2\n\n[b]\nv = 3\n"},
		{"nothing to do", RenameKey("b", "nope", "n"), RenameKey("b", "n", "nope"), file},
	} {
		got, err := c.up(file)
		if err != nil || got != c.want {
			t.Errorf("%s: %v\n%s", c.name, err, Diff("want", "got", c.want, got))
			continue
		}
		if back, err := c.down(got); err != nil || back != file {
			t.Errorf("%s, undone: %v\n%s", c.name, err, Diff("file", "back", file, back))
		}
	}
	if got, err := MoveKey("b", "new", "v")(file); err != nil || !strings.HasSuffix(got, "[b]\n\n[new]\nv = 3\n") {
		t.Errorf("move to a new table: %v %q", err, got)
	}
	if got, err := DropKey("b", "v")(file); err != nil || !strings.HasSuffix(got, "[b]\n") {
		t.Errorf("drop: %v %q", err, got)
	}
	// a key in a form the edits don't change: an error, never a miss
	for name, e := range map[string]Edit{
		"rename": RenameKey("a", "k", "kk"), "move": MoveKey("a", "b", "k"), "map": MapValue("a", "k", func(v string) (string, error) { return v, nil }),
		"drop": DropKey("a", "k"), "comment": CommentKey("a", "k", "w"), "table": RenameTable("a", "c"),
	} {
		if _, err := e("a.k = 1\n"); err == nil {
			t.Errorf("%s of a dotted key: no error", name)
		}
	}
	if _, err := MapValue("a", "x", func(v string) (string, error) { return "[", nil })(file); err == nil {
		t.Error("a value that breaks the file: no error")
	}
}

func TestDiff(t *testing.T) {
	t.Parallel()
	a := "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12\n13\n14\n15\n"
	b := "1\n2\nthree\n4\n5\n6\n7\n8\n9\n10\n11\n12\n13\n15\nsixteen\n"
	want := "--- a\n+++ b\n@@ -1,6 +1,6 @@\n 1\n 2\n-3\n+three\n 4\n 5\n 6\n@@ -11,5 +11,5 @@\n 11\n 12\n 13\n-14\n 15\n+sixteen\n"
	if got := Diff("a", "b", a, b); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	if got := Diff("a", "b", "", "x\n"); got != "--- a\n+++ b\n@@ -0,0 +1 @@\n+x\n" {
		t.Errorf("from nothing: %q", got)
	}
	if Diff("a", "b", a, a) != "" {
		t.Error("the same: a diff")
	}
}
