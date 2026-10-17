package gitbugio_test

import (
	"testing"

	"github.com/guilbep/git-board/internal/gitbugio"
)

// The numbers file: a prefix, where the counter starts, then one line per
// numbered issue. GitHub numbers below start are the issues' own.
func TestCounter(t *testing.T) {
	c, err := gitbugio.ParseCounter([]byte("prefix CO2\nstart 3052\n3053 bbb\n3052 aaa\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Prefix != "CO2" || c.Start != 3052 || c.ByID["aaa"] != 3052 || c.ByID["bbb"] != 3053 {
		t.Fatalf("parsed %+v", c)
	}
	if got, want := string(c.Bytes()), "prefix CO2\nstart 3052\n3052 aaa\n3053 bbb\n"; got != want {
		t.Errorf("Bytes() = %q, want %q", got, want)
	}
	if c.Next() != 3054 {
		t.Errorf("Next() = %d, want 3054", c.Next())
	}
	empty, _ := gitbugio.ParseCounter([]byte("prefix CO2\nstart 3052\n"))
	if empty.Next() != 3052 {
		t.Errorf("Next() of an empty counter = %d, want its start", empty.Next())
	}

	github := func(n string) []gitbugio.Forge { return []gitbugio.Forge{{Name: "GitHub", Number: n}} }
	for _, tc := range []struct {
		name   string
		id     string
		forges []gitbugio.Forge
		want   int
	}{
		{"its line", "aaa", nil, 3052},
		{"its line over its GitHub number", "bbb", github("2518"), 3053},
		{"GitHub, below start", "ccc", github("2518"), 2518},
		{"GitHub, at start: not yet", "ddd", github("3052"), 0},
		{"GitLab's never counts", "eee", []gitbugio.Forge{{Name: "GitLab", Number: "34"}}, 0},
		{"nowhere", "fff", nil, 0},
	} {
		if got := c.NumberOf(tc.id, tc.forges); got != tc.want {
			t.Errorf("%s: NumberOf = %d, want %d", tc.name, got, tc.want)
		}
	}
	if c.Format(2518) != "CO2-2518" {
		t.Errorf("Format(2518) = %q", c.Format(2518))
	}

	for _, bad := range []string{"", "start 3052\n", "prefix CO2\n", "prefix 2CO\nstart 1\n", "prefix CO2\nstart x\n", "prefix CO2\nstart 1\nnot-a-line\n"} {
		if _, err := gitbugio.ParseCounter([]byte(bad)); err == nil {
			t.Errorf("ParseCounter(%q): no error", bad)
		}
	}
}
