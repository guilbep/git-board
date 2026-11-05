package issuesync

import (
	"reflect"
	"testing"

	"github.com/guilbep/git-board/kernel"
)

// Links holds each project's effective links: the first wins.
func TestLinks(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	a, b := must(loadState(s, "example.com/p")), must(loadState(s, "example.com/q"))
	if err := a.write(t.Context(), s, "example.com/p", []kernel.Op{linkOp("a", 1, "i1", "100", "1", "https://p/1"), linkOp("a", 1, "i2", "100", "1", "https://p/1x")}); err != nil {
		t.Fatal(err)
	}
	if err := b.write(t.Context(), s, "example.com/q", []kernel.Op{linkOp("a", 1, "i1", "7", "3", "https://q/3")}); err != nil {
		t.Fatal(err)
	}
	got, err := Links(s)
	want := map[string][]Link{"i1": {{Issue: "i1", ForgeID: "100", Number: "1", URL: "https://p/1"}, {Issue: "i1", ForgeID: "7", Number: "3", URL: "https://q/3"}}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("links = %v, %v; want %v", got, err, want)
	}
}
