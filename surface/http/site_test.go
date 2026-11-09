package httpapi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guilbep/git-board/app"
)

// The read-only site (#102): an index and a page an issue, the text
// cleaned (no script from an issue runs), a deleted issue left out.
func TestWriteSite(t *testing.T) {
	t.Parallel()
	dir, ids := newBoard(t)
	a := newAPI(t, dir)
	want[map[string]any](t, a.do("POST", "/api/issues/"+string(ids[1])+"/comments", map[string]any{"body": "Look <script>alert(1)</script> **here**"}), 201)
	b, err := app.Open(t.Context(), dir, app.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	out := t.TempDir()
	written, err := WriteSite(t.Context(), b, out, "o/r issues")
	if err != nil {
		t.Fatal(err)
	}
	if len(written) != 3 || written[len(written)-1] != "index.html" {
		t.Fatalf("written %q, want two issues and the index", written)
	}
	index, _ := os.ReadFile(filepath.Join(out, "index.html"))
	for _, w := range []string{"<title>o/r issues</title>", "Export crashes", "Login fails", "1 open, 1 closed"} {
		if !strings.Contains(string(index), w) {
			t.Errorf("the index has no %q:\n%s", w, index)
		}
	}
	if strings.Contains(string(index), "Gone") {
		t.Error("the deleted issue is listed")
	}
	var page []byte
	for _, f := range written[:2] {
		if p, _ := os.ReadFile(filepath.Join(out, f)); strings.Contains(string(p), "Export crashes") {
			page = p
		}
	}
	if !strings.Contains(string(page), "<strong>here</strong>") || strings.Contains(string(page), "<script>") {
		t.Errorf("the comment, rendered and cleaned:\n%s", page)
	}
}
