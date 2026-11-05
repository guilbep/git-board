package branches

import (
	"reflect"
	"testing"

	"github.com/guilbep/git-board/domain/issues"
	"github.com/guilbep/git-board/tools/code"
)

// One Ancestry for many issues reads their base's history once, and
// lists their branches as Linked does, each on its own.
func TestLinkedInSharesTheBase(t *testing.T) {
	t.Parallel()
	dir := repo(t)
	commit := func(msg string) { run(t, dir, "commit", "-q", "--allow-empty", "-m", msg) }
	run(t, dir, "switch", "-q", "-c", "main")
	commit("start")
	commit("base")
	run(t, dir, "switch", "-q", "-c", "fix/2518-a")
	commit("a")
	run(t, dir, "switch", "-q", "-c", "fix/2519-b", "main")
	commit("b")
	commit("b again")
	run(t, dir, "switch", "-q", "main")
	commit("main moves on")
	b := code.Repo{Dir: dir}

	anc := NewAncestry(b)
	for _, is := range []issues.Issue{issue("A", "2518"), issue("B", "2519"), issue("C", "2520")} {
		shared, err := LinkedIn(t.Context(), anc, is, "main")
		if err != nil {
			t.Fatal(err)
		}
		alone, err := Linked(t.Context(), b, is, "main")
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(shared, alone) {
			t.Errorf("%s: shared %+v, alone %+v", is.Title, shared, alone)
		}
		if is.Title == "B" && (len(shared) != 1 || shared[0].Ahead != 2 || shared[0].Behind != 1) {
			t.Errorf("B: %+v, want fix/2519-b 2 ahead, 1 behind", shared)
		}
	}
	// main once, and each branch: 3, where an Ancestry per issue reads 4
	if anc.reads != 3 {
		t.Errorf("read %d histories, want 3", anc.reads)
	}
}
