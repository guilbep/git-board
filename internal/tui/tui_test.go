package tui

import (
	"testing"

	"github.com/guilbep/git-board/internal/gitbugio"
)

func TestToItemsPreservesOrderAndFields(t *testing.T) {
	t.Parallel()
	issues := []gitbugio.Issue{
		{ID: "abcdef1234", Title: "first", Status: "open", Labels: []string{"prio:p1"}},
		{ID: "b", Title: "second", Status: "closed", Labels: nil},
	}

	items := ToItems(issues)
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}

	first, ok := items[0].(item)
	if !ok {
		t.Fatalf("items[0] is not tui.item: %T", items[0])
	}
	if first.Title() != "first" {
		t.Errorf("Title() = %q, want %q", first.Title(), "first")
	}
	if got, want := first.Description(), "abcdef12  open  prio:p1"; got != want {
		t.Errorf("Description() = %q, want %q", got, want)
	}
	if got, want := first.FilterValue(), "first prio:p1"; got != want {
		t.Errorf("FilterValue() = %q, want %q", got, want)
	}

	second := items[1].(item)
	if got, want := second.Description(), "b  closed  "; got != want {
		t.Errorf("Description() = %q, want %q", got, want)
	}
}

func TestItemShowsItsNumberBesideTheTitleWithoutIt(t *testing.T) {
	t.Parallel()
	it := ToItems([]gitbugio.Issue{{ID: "abcdef1234", Title: "Wrong example (#2518)", Status: "open",
		Forges: []gitbugio.Forge{{Name: "GitHub", Number: "2518"}}}})[0].(item)
	if got, want := it.Title(), "Wrong example"; got != want {
		t.Errorf("Title() = %q, want %q", got, want)
	}
	if got, want := it.Description(), "#2518  abcdef12  open  "; got != want {
		t.Errorf("Description() = %q, want %q", got, want)
	}
}

func TestShortIDTruncatesLongIDs(t *testing.T) {
	t.Parallel()
	if got, want := shortID("abcdefghijklmnop"), "abcdefgh"; got != want {
		t.Errorf("shortID() = %q, want %q", got, want)
	}
	if got, want := shortID("short"), "short"; got != want {
		t.Errorf("shortID() = %q, want %q", got, want)
	}
}
