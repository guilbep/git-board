package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/guilbep/git-board/next/store"
	"github.com/guilbep/git-board/next/store/storetest"
)

var ctx = context.Background()

// newRepo is a new repository's board.
func newRepo(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	s, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func run(s *Store, stdin string, args ...string) error {
	var in []byte
	if stdin != "" {
		in = []byte(stdin)
	}
	_, err := s.git(ctx, in, args...)
	return err
}

// churn is what other git processes do to a board: refs moved in
// transactions (a lock file made and renamed over each), and every ref
// packed, again and again.
func churn(ctx context.Context, ds store.Datastore) error {
	s := ds.(*Store)
	c1, err := s.writePack(storetest.Pack(1), nil)
	if err != nil {
		return err
	}
	c2, err := s.writePack(storetest.Pack(2), []string{c1})
	if err != nil {
		return err
	}
	others := []string{refName("issue", storetest.ID(100)), refName("issue", storetest.ID(101)), refName("person", storetest.ID(102))}
	at := map[string]string{}
	for i := 0; ctx.Err() == nil; i++ {
		var in strings.Builder
		for _, r := range others {
			from, to := at[r], c1
			if from == c1 {
				to = c2
			}
			if from == "" {
				from = zero
			}
			fmt.Fprintf(&in, "start\nupdate %s %s %s\ncommit\n", r, to, from)
			at[r] = to
		}
		if err := run(s, in.String(), "update-ref", "--stdin"); err != nil {
			return stoppedErr(ctx, err)
		}
		if err := run(s, "", "pack-refs", "--all"); err != nil {
			return stoppedErr(ctx, err)
		}
	}
	return nil
}

func stoppedErr(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return nil
	}
	return err
}

func TestConformance(t *testing.T) {
	t.Parallel()
	storetest.Run(t, storetest.Options{
		Open:  func(t *testing.T) store.Datastore { return newRepo(t) },
		Churn: churn,
		Junk: func(t *testing.T, ds store.Datastore) {
			s := ds.(*Store)
			c, err := s.writePack(storetest.Pack(1), nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range []string{"refs/board/issue/notes", "refs/board/issue/a/b", "refs/board/issue/" + strings.ToUpper(storetest.ID(0xabc))} {
				if err := run(s, "", "update-ref", r, c); err != nil {
					t.Fatal(err)
				}
			}
		},
	})
}

// Every ref of a board, loose or packed, is listed while git packs them
// and moves them: the bug fbda0b3 fixed in v1, on many refs at once.
func TestHeadsWhileGitPacksAndMoves(t *testing.T) {
	t.Parallel()
	s := newRepo(t)
	c1, err := s.writePack(storetest.Pack(1), nil)
	if err != nil {
		t.Fatal(err)
	}
	c2, err := s.writePack(storetest.Pack(2), []string{c1})
	if err != nil {
		t.Fatal(err)
	}
	const n = 200
	var in strings.Builder
	for i := range n {
		fmt.Fprintf(&in, "create %s %s\n", refName("issue", storetest.ID(i)), c1)
	}
	if err := run(s, in.String(), "update-ref", "--stdin"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	t.Cleanup(func() { cancel(); <-done; done <- nil }) // git is done before the repository goes
	go func() {
		done <- func() error {
			for round := 0; round < 20 && ctx.Err() == nil; round++ {
				// half the refs moved loose, one transaction each, then all packed
				var in strings.Builder
				from, to := c1, c2
				if round%2 == 1 {
					from, to = c2, c1
				}
				for i := 0; i < n; i += 2 {
					fmt.Fprintf(&in, "start\nupdate %s %s %s\ncommit\n", refName("issue", storetest.ID(i)), to, from)
				}
				if err := run(s, in.String(), "update-ref", "--stdin"); err != nil {
					return err
				}
				if err := run(s, "", "pack-refs", "--all"); err != nil {
					return err
				}
			}
			return nil
		}()
		cancel()
	}()
	reads := 0
	for ctx.Err() == nil {
		heads, err := s.Heads(ctx, "issue")
		if err != nil {
			t.Fatal(err)
		}
		if len(heads) != n {
			t.Fatalf("read %d: %d refs listed, want %d", reads, len(heads), n)
		}
		reads++
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	done <- nil
	t.Logf("%d listings", reads)
}

// A packed-refs whose refs aren't sorted (no "sorted" trait) is read
// whole, and its listing sorted.
func TestHeadsUnsortedPackedRefs(t *testing.T) {
	t.Parallel()
	s := newRepo(t)
	c, err := s.writePack(storetest.Pack(1), nil)
	if err != nil {
		t.Fatal(err)
	}
	packed := fmt.Sprintf("# pack-refs with: peeled\n%s refs/board/person/%s\n%s refs/board/issue/%s\n%s refs/board/issue/%s\n%s refs/board/issue/notes\n",
		c, storetest.ID(3), c, storetest.ID(2), c, storetest.ID(1), c)
	if err := writeFile(s.commonDir+"/packed-refs", packed); err != nil {
		t.Fatal(err)
	}
	heads, err := s.Heads(ctx, "person", "issue")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, h := range heads {
		got = append(got, h.Kind+"/"+h.ID[38:])
	}
	if strings.Join(got, " ") != "issue/01 issue/02 person/03" {
		t.Fatalf("Heads = %v", got)
	}
}

// seek lands on the first ref of a prefix in a sorted packed-refs, past
// its header and peeled lines, or at the end when none is there.
func TestSeek(t *testing.T) {
	t.Parallel()
	h := strings.Repeat("a", 40)
	packed := "# pack-refs with: peeled fully-peeled sorted \n" +
		h + " refs/board/issue/1\n" + h + " refs/board/person/1\n^" + h + "\n" + h + " refs/board/person/2\n" + h + " refs/tags/v1\n"
	for prefix, want := range map[string]string{
		"refs/board/issue/":  "refs/board/issue/1",
		"refs/board/person/": "refs/board/person/1",
		"refs/board/sync/":   "refs/tags/v1",
		"refs/zzz/":          "",
		"refs/":              "refs/board/issue/1",
	} {
		rest := packed[seek(packed, prefix):]
		got := ""
		if rest != "" {
			got = strings.TrimSpace(strings.SplitN(rest, "\n", 2)[0][41:])
		}
		if got != want {
			t.Errorf("seek(%q) lands on %q, want %q", prefix, got, want)
		}
	}
}

func writeFile(path, s string) error { return os.WriteFile(path, []byte(s), 0o644) }
