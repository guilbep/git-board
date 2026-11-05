package golden

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
)

const (
	root    = "../testdata/golden"
	allowed = "../testdata/compat/allowed.txt"
	pending = "../testdata/compat/pending.txt"
	// PendingEnv set to 1 holds the binary to pending.txt too: v2's
	// differences that a later milestone closes. make compat sets it with
	// BIN=, v1 having none.
	PendingEnv = "GIT_BOARD_COMPAT_PENDING"
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
	t.Cleanup(func() { b.Drop(context.Background(), dir) })
	if err := b.Checkout(ctx, dir); errors.Is(err, ErrNoPostgres) {
		t.Skipf("%s: %v", b.Name, err)
	} else if err != nil {
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
	var wait Pending
	if os.Getenv(PendingEnv) == "1" {
		if wait, err = ReadPending(pending); err != nil {
			t.Fatal(err)
		}
	}
	var mu sync.Mutex
	held := make([]bool, len(wait)) // the entries a difference needed
	ran := map[string]bool{}
	t.Cleanup(func() { // after every board's run
		for i, p := range wait {
			if !held[i] && (p.Board == "*" || ran[p.Board]) {
				t.Errorf("pending.txt:%d (%s %s %s, %s) passes now: remove it from pending", p.Line, p.Board, p.Case, p.What, p.Milestone)
			}
		}
	})
	for _, b := range boards(t) {
		t.Run(b.Name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			dir, home, cases := checkout(t, b) // a board skipped didn't run: its pending entries wait
			mu.Lock()
			ran[b.Name] = true
			mu.Unlock()
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
					if i := wait.Waits(b.Name, c.Name, d.what); i >= 0 {
						mu.Lock()
						held[i] = true
						mu.Unlock()
						t.Logf("%s (%s): %s differs, pending %s: %s", c.Name, c.Line, d.what, wait[i].Milestone, wait[i].Why)
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
			var cases []Case
			if b.Datastore == postgres { // its cases are known without a server
				ids, err := b.IssueIDs(context.Background(), "")
				if err != nil {
					t.Fatal(err)
				}
				cases = b.Cases(ids)
			} else {
				_, _, cases = checkout(t, b)
			}
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
	if _, err := ReadPending(pending); err != nil {
		t.Fatal(err)
	}
}

func TestPendingReads(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for text, ok := range map[string]bool{
		"basic ls stdout M4 forges come with sync records\n": true,
		"basic ls stdout forges come with sync\n":            false,
		"basic ls out M4 why\n":                              false,
		"# nothing\n":                                        true,
	} {
		p := filepath.Join(dir, "p.txt")
		os.WriteFile(p, []byte(text), 0o644)
		if _, err := ReadPending(p); (err == nil) != ok {
			t.Errorf("ReadPending(%q) = %v", text, err)
		}
	}
	p := Pending{{Board: "basic", Case: "cmd*", What: "*", Milestone: "M2"}}
	if p.Waits("basic", "cmd19-x", "stdout") != 0 || p.Waits("merge", "cmd19-x", "stdout") != -1 {
		t.Error("Waits doesn't match as Allows does")
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
