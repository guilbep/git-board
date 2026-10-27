package store_test

import (
	"maps"
	"strings"
	"testing"

	"github.com/guilbep/git-board/core/forge/fake"
	"github.com/guilbep/git-board/core/issue"
	"github.com/guilbep/git-board/internal/store"
	"github.com/guilbep/git-board/internal/store/storetest"
)

// 6a's description
const sixA = "Seen in the clone of the git-bug fork.\n\n## Releases page\n\n- The header reads \". · published on GitHub\".\n\n" +
	"## Expected\n\n- [ ] The authority remote is chosen, not assumed.\n- [ ] Pre-release tags count.\n" +
	"- [ ] A missing identity sends the user to Settings › identity, not to a CLI command.\n- [ ] The header names the repository.\n\n" +
	"## Workaround\n\nNot tried yet."

func sixAIssue(t *testing.T) (*store.Backend, string, store.Person) {
	t.Helper()
	b := storetest.New(t)
	alice := storetest.NewPerson(t, b, "Person A", "alice")
	return b, storetest.IssueAs(t, b, alice, 100, "The releases page", sixA), alice
}

// 6f's quote, the intro, a section without an item, a slug v1 lacks.
func TestQuote(t *testing.T) {
	t.Parallel()
	b, id, _ := sixAIssue(t)
	q := store.Meanings(b.Store()).Quote
	for anchor, want := range map[string]string{
		"body@v1/expected#3": "> **On v1 · § Expected · 3** · _A missing identity sends the user to Settings › identity, not to a CLI command._",
		"body@v1/intro":      "> **On v1 · § intro**",
		"body@v1/workaround": "> **On v1 · § Workaround**",
		"body@v1/gone#2":     "> **On v1 · § gone · 2**",
		"body@v2/expected":   "> **On v2 · § expected**",
		"plan@a1f0/steps#4":  "",
	} {
		if got := q(id, issue.Comment{Meta: map[string]string{"thread:anchor": anchor}}); got != want {
			t.Errorf("%s: %q, want %q", anchor, got, want)
		}
	}
}

// A version published names its changed sections; a tick or no change
// publishes nothing.
func TestPublished(t *testing.T) {
	t.Parallel()
	b, id, alice := sixAIssue(t)
	pub := store.Meanings(b.Store()).Published
	if got, _ := pub(id, sixA, sixA); got != "" {
		t.Errorf("v1 alone published %q", got)
	}
	ticked := strings.Replace(sixA, "- [ ] Pre-release", "- [x] Pre-release", 1)
	v2 := strings.Replace(ticked, "not to a CLI command", "never to a CLI command", 1)
	is, _ := issue.Load(b.Store(), id)
	storetest.Do(t, b, id, issue.EditText(alice.ID, is.BodyOp, ticked), issue.EditText(alice.ID, is.BodyOp, v2))
	if got, _ := pub(id, sixA, ticked); got != "" {
		t.Errorf("a tick published %q", got)
	}
	edit, note := pub(id, sixA, v2)
	if note != "**Published v2** · § Expected changed" || edit == "" || edit == is.BodyOp {
		t.Errorf("published %q %q", edit, note)
	}
}

// Marked, the keys the forge's marker says; deleted, the thread's and the
// spec's keys cleared, the step kept.
func TestRestore(t *testing.T) {
	t.Parallel()
	b := storetest.New(t)
	opt := store.Meanings(b.Store())
	meta := map[string]string{"cycle:step": "1", "thread:anchor": "body@v1/expected#3", "thread:kind": "suggestion", "review:path": "x"}
	if got := opt.Carry(meta); got != "step=1 anchor=body@v1/expected#3 kind=suggestion" {
		t.Errorf("carry %q", got)
	}
	if got, want := opt.Restore("step=1 anchor=body@v1/expected#3 resolved=v2", true, meta), map[string]string{"thread:kind": "", "thread:resolved": "v2"}; !maps.Equal(got, want) {
		t.Errorf("marked: %v, want %v", got, want)
	}
	if got, want := opt.Restore("", false, meta), map[string]string{"thread:anchor": "", "thread:kind": ""}; !maps.Equal(got, want) {
		t.Errorf("unmarked: %v, want %v", got, want)
	}
}

// A thread comment goes to the forge with its quote and keys; a review
// comment stays here.
func TestThreadCommentSyncs(t *testing.T) {
	t.Parallel()
	b, id, alice := sixAIssue(t)
	f := fake.New("github.com/o/r")
	if _, err := b.SyncForges(t.Context(), run(f)); err != nil {
		t.Fatal(err)
	}
	storetest.Do(t, b, id,
		issue.NewCommentMeta(alice.ID, "Which screen?", map[string]string{"cycle:step": "1", "thread:anchor": "body@v1/expected#3"}),
		issue.NewCommentMeta(alice.ID, "a line note", map[string]string{"review:path": "a.go", "review:line": "3"}))
	if _, err := b.SyncForges(t.Context(), run(f)); err != nil {
		t.Fatal(err)
	}
	all := f.All()
	if len(all) != 1 || len(all[0].Comments) != 1 {
		t.Fatalf("forge = %+v", all)
	}
	body := all[0].Comments[0].Body
	is, _ := issue.Load(b.Store(), id)
	for _, want := range []string{
		"\n\n> **On v1 · § Expected · 3** · _A missing identity",
		"\n\nWhich screen?\n\n<!-- git-board comment " + is.Comments[0].ID + " quoted step=1 anchor=body@v1/expected#3 -->",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("posted %q, want it holding %q", body, want)
		}
	}
}
