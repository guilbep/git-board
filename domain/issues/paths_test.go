package issues

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guilbep/git-board/kernel"
	bsync "github.com/guilbep/git-board/sync"
)

// A record's number is board data: one that isn't digits is passed over
// for the URL's.
func TestForgedNumberDigitsOnly(t *testing.T) {
	t.Parallel()
	for number, want := range map[string]string{
		"12": "12", "": "7", "../../escaped": "7", "7/../x": "7", "-1": "7", "1e3": "7", " 7": "7",
	} {
		var is Issue
		is.forged([]bsync.Record{{LinkData: bsync.LinkData{Number: number, URL: "https://github.com/o/r/issues/7"}}})
		if got := is.Forges[0].Number; got != want {
			t.Errorf("number %q: %q, want %q", number, got, want)
		}
	}
	var is Issue
	is.forged([]bsync.Record{{LinkData: bsync.LinkData{Number: "..", URL: "https://github.com/o/r/issues/.."}}})
	if is.Forges[0].Number != "" || is.BareNumber() != "" {
		t.Fatalf("a URL ending in .. numbered it %q", is.Forges[0].Number)
	}
}

// An inbox's file is named by a person's id: anything else is refused,
// never a path.
func TestInboxIDNamesNoPath(t *testing.T) {
	t.Parallel()
	folder := t.TempDir()
	for _, id := range []kernel.ID{"../../x", "", "ABABABABABABABABABABABABABABABABABABABAB", kernel.ID(strings.Repeat("a", 39) + "/")} {
		if err := SaveInbox(folder, id, Inbox{}); err == nil {
			t.Fatalf("saved an inbox as %q", id)
		}
		if _, err := LoadInbox(folder, id); err == nil {
			t.Fatalf("read an inbox as %q", id)
		}
	}
	id := kernel.ID(strings.Repeat("ab", 20))
	if err := SaveInbox(folder, id, Inbox{Fields: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(folder, "git-board", "inbox", string(id)+".json")); err != nil {
		t.Fatal(err)
	}
}
