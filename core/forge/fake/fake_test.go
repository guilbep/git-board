package fake

import (
	"errors"
	"testing"

	"github.com/guilbep/git-board/core/forge"
)

func issue(number, title string) forge.Issue {
	return forge.Issue{ID: "id" + number + title, Number: number, Title: title, Status: "open",
		Labels: []string{"bug"}, Comments: []forge.Comment{{ID: "c", Body: "hi"}}}
}

func TestFakeServer(t *testing.T) {
	t.Parallel()
	if got := New("example.com/a/b").Server(); got != "example.com" {
		t.Fatalf("server = %q", got)
	}
}

// Changing a returned or an added issue's labels or comments doesn't change
// the fake.
func TestFakeReturnsCopies(t *testing.T) {
	t.Parallel()
	f := New("example.com/p")
	added := issue("1", "T")
	f.Add(added)
	added.Labels[0], added.Comments[0].Body = "x", "x"
	got, _ := f.Issues(t.Context(), 0)
	got[0].Labels[0], got[0].Comments[0].Body = "y", "y"
	again, _ := f.Issues(t.Context(), 0)
	if again[0].Labels[0] != "bug" || again[0].Comments[0].Body != "hi" {
		t.Fatalf("issue = %+v", again[0])
	}
}

func TestFakeEditFirstOnly(t *testing.T) {
	t.Parallel()
	f := New("example.com/p")
	f.Add(issue("1", "A"))
	f.Add(issue("1", "B"))
	if !f.Edit("1", func(is *forge.Issue) { is.Status = "closed" }) {
		t.Fatal("Edit found no issue")
	}
	got, _ := f.Issues(t.Context(), 0)
	if got[0].Status != "closed" || got[1].Status != "open" || got[0].Changed["status"] == 0 {
		t.Fatalf("statuses = %q, %q; changed %v", got[0].Status, got[1].Status, got[0].Changed)
	}
}

func TestFakeEditUnknown(t *testing.T) {
	t.Parallel()
	f := New("example.com/p")
	f.Add(issue("1", "A"))
	if f.Edit("2", func(is *forge.Issue) { t.Fatal("changed an issue") }) {
		t.Fatal("Edit of an unknown number is true")
	}
}

// Issues lists by update time, without deleted or confidential ones;
// reading one alone tells them apart.
func TestFakeListing(t *testing.T) {
	t.Parallel()
	f := New("example.com/p")
	old := f.Add(forge.Issue{Title: "old", Created: 10})
	gone := f.Add(forge.Issue{Title: "gone"})
	secret := f.Add(forge.Issue{Title: "secret"})
	f.Delete(gone.Number)
	f.SetConfidential(secret.Number, true)
	if got, _ := f.Issues(t.Context(), 0); len(got) != 1 || got[0].Title != "old" {
		t.Fatalf("all = %+v", got)
	}
	if got, _ := f.Issues(t.Context(), 11); len(got) != 0 {
		t.Fatalf("since 11 = %+v", got)
	}
	if _, err := f.Issue(t.Context(), gone.ID); !errors.Is(err, forge.ErrNotFound) {
		t.Fatalf("deleted: %v", err)
	}
	if _, err := f.Issue(t.Context(), secret.ID); !errors.Is(err, forge.ErrConfidential) {
		t.Fatalf("confidential: %v", err)
	}
	if _, err := f.As(t.Context(), forge.Account{ID: "9"}); !errors.Is(err, forge.ErrNoToken) {
		t.Fatalf("as a stranger: %v", err)
	}
	_ = old
}
