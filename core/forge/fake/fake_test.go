package fake

import (
	"reflect"
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
	got, _ := f.Issues(t.Context())
	got[0].Labels[0], got[0].Comments[0].Body = "y", "y"
	again, _ := f.Issues(t.Context())
	if want := []forge.Issue{issue("1", "T")}; !reflect.DeepEqual(again, want) {
		t.Fatalf("issues = %+v, want %+v", again, want)
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
	got, _ := f.Issues(t.Context())
	if got[0].Status != "closed" || got[1].Status != "open" {
		t.Fatalf("statuses = %q, %q", got[0].Status, got[1].Status)
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
