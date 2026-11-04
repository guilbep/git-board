package main

import (
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/store"
)

func TestCommentAddsAndEdits(t *testing.T) {
	newTestRepo(t)
	id := openIssueID(t)

	out, err := execute(t, []string{"comment", id, "-b", "one"})
	if err != nil {
		t.Fatalf("comment: %v", err)
	}
	ref := strings.TrimSpace(out)
	opened := editorWrites(t, "two\n")
	if _, err := execute(t, []string{"comment", id}); err != nil { // no flag: the editor
		t.Fatalf("comment in the editor: %v", err)
	}
	if *opened != "" {
		t.Errorf("a new comment's editor opened on %q", *opened)
	}

	editorWrites(t, "two, edited\n")
	if _, err := execute(t, []string{"comment", id, "--edit-last"}); err != nil {
		t.Fatalf("comment --edit-last: %v", err)
	}
	if _, err := execute(t, []string{"comment", id, "--edit", ref, "-b", "one, edited"}); err != nil {
		t.Fatalf("comment --edit %s: %v", ref, err)
	}
	th := threadOf(t, id)
	if got := comments(th); !slices.Equal(got, []string{"one, edited", "two, edited"}) {
		t.Errorf("comments = %q", got)
	}
	if i := slices.IndexFunc(th.Events, func(e store.Event) bool { return e.Kind == "comment" }); !th.Events[i].Edited {
		t.Errorf("the edited comment isn't marked edited: %+v", th.Events[i])
	}
	if _, err := execute(t, []string{"comment", id, "--edit", "fffffff:9", "-b", "x"}); exitCode(err) != exitNotFound {
		t.Errorf("comment --edit on no comment = %v, want exit %d", err, exitNotFound)
	}
	if _, err := execute(t, []string{"comment", id, "-b", "  "}); !errors.Is(err, errAborted) {
		t.Errorf("an empty comment = %v, want aborted", err)
	}
}

// Someone edits the comment while it's in your editor: your edit is
// refused, exit 5, and your text kept.
func TestCommentEditRefusesAConcurrentEdit(t *testing.T) {
	newTestRepo(t)
	id := openIssueID(t)
	out, err := execute(t, []string{"comment", id, "-b", "original"})
	if err != nil {
		t.Fatal(err)
	}
	ref := strings.TrimSpace(out)
	was := runEditor
	runEditor = func(_, initial string) (string, error) {
		if _, err := execute(t, []string{"comment", id, "--edit", ref, "-b", "theirs"}); err != nil {
			t.Errorf("the other edit: %v", err)
		}
		return "mine", nil
	}
	t.Cleanup(func() { runEditor = was })

	_, err = execute(t, []string{"comment", id, "--edit", ref})
	if exitCode(err) != exitConflict {
		t.Fatalf("a concurrent edit = %v, want exit %d", err, exitConflict)
	}
	path := err.Error()[strings.LastIndex(err.Error(), " ")+1 : len(err.Error())-1]
	if data, rerr := os.ReadFile(path); rerr != nil || strings.TrimSpace(string(data)) != "mine" {
		t.Errorf("kept text in %q = %q, %v", path, data, rerr)
	}
	os.Remove(path)
	if got := comments(threadOf(t, id)); !slices.Equal(got, []string{"theirs"}) {
		t.Errorf("comments = %q, want theirs", got)
	}
}
