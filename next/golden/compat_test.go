package golden

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

const (
	root    = "../testdata/golden"
	allowed = "../testdata/compat/allowed.txt"
	// BinEnv names the binary TestCompat runs: make compat sets it, to
	// v1's git-board by default, to git-board-next with BIN=.
	BinEnv = "GIT_BOARD_COMPAT_BIN"
	// UpdateEnv set to 1 records the binary's outputs as the goldens
	// instead: make golden, with v1 alone.
	UpdateEnv = "GIT_BOARD_GOLDEN_UPDATE"
)

// checkout makes a checkout of b in a temporary folder, and answers it,
// a home beside it, and its cases.
func checkout(t *testing.T, b Board) (dir, home string, cases []Case) {
	t.Helper()
	ctx := context.Background()
	tmp := t.TempDir()
	dir, home = filepath.Join(tmp, "repo"), filepath.Join(tmp, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := b.Checkout(ctx, dir); err != nil {
		t.Fatal(err)
	}
	ids, err := b.IssueIDs(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	return dir, home, b.Cases(ids)
}

func boards(t *testing.T) []Board {
	t.Helper()
	bs, err := Boards(filepath.Join(root, "boards"))
	if err != nil {
		t.Fatal(err)
	}
	if len(bs) == 0 {
		t.Fatal("no golden board")
	}
	return bs
}

// TestCompat runs a binary over every golden board and compares each
// output with v1's, but for the differences allowed.txt allows.
func TestCompat(t *testing.T) {
	bin := os.Getenv(BinEnv)
	if bin == "" {
		t.Skip(BinEnv + " names the binary to run: make compat")
	}
	update := os.Getenv(UpdateEnv) == "1"
	allow, err := ReadAllowed(allowed)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range boards(t) {
		t.Run(b.Name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			dir, home, cases := checkout(t, b)
			out := filepath.Join(root, "cli", b.Name)
			if update {
				results := map[string]Result{}
				for _, c := range cases {
					r, err := Run(ctx, bin, dir, home, c)
					if err != nil {
						t.Fatalf("%s: %v", c.Line, err)
					}
					results[c.Name] = r
				}
				if err := Write(out, cases, results); err != nil {
					t.Fatal(err)
				}
				return
			}
			rec, err := Read(out)
			if err != nil {
				t.Fatalf("%v: make golden", err)
			}
			if !slices.Equal(rec.Cases, cases) {
				t.Fatalf("the cases recorded aren't the board's: make golden")
			}
			for _, c := range cases {
				got, err := Run(ctx, bin, dir, home, c)
				if err != nil {
					t.Fatalf("%s: %v", c.Line, err)
				}
				want := rec.Results[c.Name]
				for _, d := range []struct {
					what      string
					same      bool
					want, got []byte
				}{
					{"exit", want.Exit == got.Exit, []byte{byte(want.Exit)}, []byte{byte(got.Exit)}},
					{"stdout", bytes.Equal(want.Stdout, got.Stdout), want.Stdout, got.Stdout},
					{"stderr", bytes.Equal(want.Stderr, got.Stderr), want.Stderr, got.Stderr},
				} {
					if d.same {
						continue
					}
					if why, ok := allow.Allows(b.Name, c.Name, d.what); ok {
						t.Logf("%s (%s): %s differs, allowed: %s", c.Name, c.Line, d.what, why)
						continue
					}
					if d.what == "exit" {
						t.Errorf("%s (git-board %s): exit %d, want %d", c.Name, c.Line, got.Exit, want.Exit)
						continue
					}
					t.Errorf("%s (git-board %s): %s differs at %s", c.Name, c.Line, d.what, Diff(d.want, d.got))
				}
			}
		})
	}
}

// TestGoldensComplete checks, without a binary, that every board's cases
// are recorded: a board changed without make golden fails here.
func TestGoldensComplete(t *testing.T) {
	t.Parallel()
	for _, b := range boards(t) {
		t.Run(b.Name, func(t *testing.T) {
			t.Parallel()
			_, _, cases := checkout(t, b)
			rec, err := Read(filepath.Join(root, "cli", b.Name))
			if err != nil {
				t.Fatalf("%v: make golden", err)
			}
			if !slices.Equal(rec.Cases, cases) {
				t.Fatalf("the cases recorded aren't the board's: make golden")
			}
		})
	}
}

func TestAllowedReads(t *testing.T) {
	t.Parallel()
	if _, err := ReadAllowed(allowed); err != nil {
		t.Fatal(err)
	}
}

func TestSplit(t *testing.T) {
	t.Parallel()
	for line, want := range map[string][]string{
		`ls --json`:                        {"ls", "--json"},
		`ls "is:open is:closed" --json`:    {"ls", "is:open is:closed", "--json"},
		`ls "\"a phrase\"" --json`:         {"ls", `"a phrase"`, "--json"},
		`ls  ""  x`:                        {"ls", "", "x"},
		`ls "a\\b"`:                        {"ls", `a\b`},
		`ls --jq ".[].number" --json id,x`: {"ls", "--jq", ".[].number", "--json", "id,x"},
	} {
		got, err := Split(line)
		if err != nil || !slices.Equal(got, want) {
			t.Errorf("Split(%q) = %q, %v; want %q", line, got, err, want)
		}
	}
	if _, err := Split(`ls "open`); err == nil {
		t.Error(`Split("ls \"open") took a quote not closed`)
	}
}

func TestAllows(t *testing.T) {
	t.Parallel()
	a := Allowed{{"basic", "show-*", "stderr", "why"}, {"*", "ls", "*", "because"}}
	for _, c := range []struct {
		board, name, what string
		ok                bool
	}{
		{"basic", "show-26a4ba85e9ef", "stderr", true},
		{"basic", "show-26a4ba85e9ef", "stdout", false},
		{"merge", "ls", "exit", true},
		{"merge", "log-x", "exit", false},
	} {
		if _, ok := a.Allows(c.board, c.name, c.what); ok != c.ok {
			t.Errorf("Allows(%s, %s, %s) = %v", c.board, c.name, c.what, ok)
		}
	}
}
