package server

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/store/storetest"
)

// A PATCH naming no field is refused, 400, and writes nothing: {"":"x"}
// wrote a ":x" label (#89). fields.Apply refuses the name (8b0c425); the
// names go in sorted order, so a valid field beside it doesn't land either.
func TestPatchRefusesAnEmptyFieldName(t *testing.T) {
	t.Parallel()
	b := storetest.New(t)
	id := storetest.NewIssue(t, b, "x")
	ts := newTestServer(t, b, mustLoadConfig(t))
	for _, body := range []map[string]string{{"": "x"}, {" ": "x"}, {"": "x", "cat": "bug"}} {
		if code := call(t, "PATCH", ts.URL+"/api/issues/"+id, body, nil); code != http.StatusBadRequest {
			t.Errorf("PATCH %v: %d, want 400", body, code)
		}
	}
	is, err := b.GetIssue(id)
	if err != nil {
		t.Fatal(err)
	}
	if i := slices.IndexFunc(is.Labels, func(l string) bool { return strings.HasPrefix(l, ":") || l == "cat:bug" }); i >= 0 {
		t.Errorf("labels %v: %q written by a refused PATCH", is.Labels, is.Labels[i])
	}
}
