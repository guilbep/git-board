package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

// A page's slow interaction is logged with its trace; one that isn't one
// is refused, named. Every reply says what the server took
// (Server-Timing), the board's work counted.
func TestSlowInteraction(t *testing.T) {
	t.Parallel()
	dir, _ := newBoard(t)
	a := newAPI(t, dir)
	res := a.do("POST", "/api/slow", map[string]any{"name": "open /issues/:id", "ms": 340, "trace": "0af7651916cd43dd8448eb211c80319c", "span": "b7ad6b7169203331"})
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("slow: %d", res.StatusCode)
	}
	if l := a.logs(); !strings.Contains(l, "slow interaction") || !strings.Contains(l, "0af7651916cd43dd8448eb211c80319c") {
		t.Errorf("log lacks the interaction and its trace:\n%s", l)
	}
	if res := a.do("POST", "/api/slow", map[string]any{"name": "", "ms": 1}); res.StatusCode != http.StatusBadRequest {
		t.Errorf("no name: %d", res.StatusCode)
	}
	res = a.do("GET", "/api/issues", nil)
	if st := res.Header.Get("Server-Timing"); !strings.Contains(st, "app;dur=") || !strings.Contains(st, `work;desc="`) {
		t.Errorf("Server-Timing %q", st)
	}
}
