package issuesync

import (
	"strings"
	"testing"

	"github.com/guilbep/git-board/next/forge/fake"
)

// A forge-only copy that is lost and pulled again gets back every issue
// and comment, those sync posted with its markers too, once; a second
// machine's copy does the same. Without ForgeOnly, a marker naming an id
// that isn't here is skipped, waiting for its fetch.
func TestForgeOnlyPullsMarkedTextAsNew(t *testing.T) {
	t.Parallel()
	f := fake.New(key)
	first := newStore(t)
	p, err := CreatePerson(t.Context(), first, NewPerson("", "dev", ""))
	if err != nil {
		t.Fatal(err)
	}
	is, err := CreateIssue(t.Context(), first, NewCreate(p.ID, "made here", "its body"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DoIssue(t.Context(), first, is.ID, NewComment(p.ID, "said here")); err != nil {
		t.Fatal(err)
	}
	opt := Options{ForgeOnly: true}
	if _, err := Sync(t.Context(), first, opt, f); err != nil {
		t.Fatal(err)
	}
	all := f.All()
	if len(all) != 1 || len(all[0].Comments) != 1 || !strings.Contains(all[0].Body, "git-board issue "+is.ID) {
		t.Fatalf("forge %+v; want the issue, marked, and its comment", all)
	}

	// a copy without the rule skips what its markers name
	waits := newStore(t)
	if _, err := Pull(t.Context(), waits, Options{}, f); err != nil {
		t.Fatal(err)
	}
	if ids, _ := ListIssues(waits); len(ids) != 0 {
		t.Fatalf("issues %v; want none, waiting for a fetch", ids)
	}

	// the lost copy, pulled again
	again := newStore(t)
	writes := len(f.Writes())
	for range 2 {
		if _, err := Sync(t.Context(), again, opt, f); err != nil {
			t.Fatal(err)
		}
	}
	// pulled as anyone's text is: as the forge shows it, sync's header and
	// marker in it
	got := only(t, again)
	if got.Title != "made here" || !strings.Contains(got.Body, "its body") || len(got.Comments) != 1 ||
		!strings.Contains(got.Comments[0].Body, "said here") {
		t.Fatalf("pulled %+v; want the issue and its comment", got)
	}
	if w := f.Writes()[writes:]; len(w) != 0 {
		t.Fatalf("forge writes %+v; want nothing sent back", w)
	}
}
