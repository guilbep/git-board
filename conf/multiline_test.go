package conf

import (
	"strings"
	"testing"
)

// A value over several lines, an array or a multi-line string, is set
// aside whole and given back whole: the file stays TOML both ways, byte
// for byte (review: CommentKey commented the first line alone).
func TestMultilineValues(t *testing.T) {
	t.Parallel()
	for name, v1 := range map[string]string{
		"array":          "[repos]\nlist = [\"a/b\"]\nassignees = [\n  \"alice\",  # first\n  \"bob\",\n]\nother = 1\n",
		"nested array":   "[repos]\nassignees = [\n  [1, 2],\n  [3],\n]\nlist = [\"a/b\"]\n",
		"basic string":   "[repos]\nassignees = \"\"\"\nlabels\n[not] a header\nlist = \"not a key\"\n# not a comment\n\"\"\"\nlist = [\"a/b\"]\n",
		"literal string": "[sprints]\nstart_weekday = '''\nmonday\n'''  # the team's\nlength_days = 14\n",
		"two keys":       "[sprints]\nstart_weekday = [\n\"monday\",\n]\n\n[repos]\nassignees = [\n\"x\"\n]\n",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			up, _, err := Migrate(v1, 2)
			if err != nil {
				t.Fatalf("up: %v", err)
			}
			for _, l := range strings.Split(up, "\n") {
				if strings.Contains(v1, l) && l != "" && !strings.HasPrefix(l, "[") && !strings.HasPrefix(l, "list") && !strings.HasPrefix(l, "other") && !strings.HasPrefix(l, "length_days") {
					t.Errorf("line %q left as it was:\n%s", l, up)
				}
			}
			if f, err := Parse(FileName, []byte(up)); err != nil || f.Version() != 2 {
				t.Fatalf("up isn't a schema 2 file: %v\n%s", err, up)
			}
			back, _, err := Migrate(up, 1)
			if err != nil || back != v1 {
				t.Fatalf("down: %v\n%s", err, Diff("v1", "back", v1, back))
			}
			// the read in memory keeps the lines where they are
			f, err := Parse(FileName, []byte(v1))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Count(string(f.Text()), "\n") != strings.Count(v1, "\n") {
				t.Errorf("the read moved lines:\n%s", f.Text())
			}
		})
	}
}

// The other edits take a multi-line value whole too, or refuse it.
func TestMultilineEdits(t *testing.T) {
	t.Parallel()
	const file = "[a]\nx = [\n  1,\n  2,\n]\ny = 1\n\n[b]\nz = \"\"\"\nq = 1\n\"\"\"\n"
	if got, err := DropKey("a", "x")(file); err != nil || got != "[a]\ny = 1\n\n[b]\nz = \"\"\"\nq = 1\n\"\"\"\n" {
		t.Errorf("drop: %v %q", err, got)
	}
	moved, err := MoveKey("a", "b", "x")(file)
	if err != nil || moved != "[a]\ny = 1\n\n[b]\nz = \"\"\"\nq = 1\n\"\"\"\nx = [\n  1,\n  2,\n]\n" {
		t.Fatalf("move: %v %q", err, moved)
	}
	if back, err := MoveKey("b", "a", "x")(moved); err != nil || back != "[a]\ny = 1\nx = [\n  1,\n  2,\n]\n\n[b]\nz = \"\"\"\nq = 1\n\"\"\"\n" {
		t.Errorf("move back: %v %q", err, back)
	}
	if got, err := RenameKey("a", "x", "w")(file); err != nil || !strings.HasPrefix(got, "[a]\nw = [\n  1,\n") {
		t.Errorf("rename: %v %q", err, got)
	}
	if _, err := MapValue("a", "x", func(v string) (string, error) { return v, nil })(file); err == nil || !strings.Contains(err.Error(), "several lines") {
		t.Errorf("map of a multi-line value: %v", err)
	}
	// q inside z's string is no key of [b]
	if _, err := DropKey("b", "q")(file); err != nil {
		t.Errorf("a key-like line in a string: %v", err)
	}
	if got, _ := DropKey("b", "q")(file); got != file {
		t.Errorf("dropped a line of a string: %q", got)
	}
	// a value that never ends: an error, never a line left
	if _, err := CommentKey("a", "x", "w")("[a]\nx = [\n  1,\n"); err == nil {
		t.Error("an array that doesn't end: no error")
	}
}
