package server

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/store"
)

// The HTTP API's golden transcripts (docs/rewrite/design.md, M0): v1's
// answers to next/testdata/golden/api/requests.txt on the golden board
// basic, which v2 must give again. GIT_BOARD_GOLDEN_UPDATE=1 records them
// (make golden).
const (
	goldenDir    = "../../next/testdata/golden"
	goldenUpdate = "GIT_BOARD_GOLDEN_UPDATE"
)

// TestAPITranscripts stays serial: it sets time.Local, as TZ=UTC would.
func TestAPITranscripts(t *testing.T) {
	local := time.Local
	time.Local = time.UTC
	t.Cleanup(func() { time.Local = local })
	start := time.Now().Add(-time.Second)
	update := os.Getenv(goldenUpdate) == "1"

	dir := filepath.Join(t.TempDir(), "repo")
	board, err := filepath.Abs(filepath.Join(goldenDir, "boards", "basic"))
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main", dir},
		{"-C", dir, "fetch", "-q", "--update-head-ok", filepath.Join(board, "board.bundle"), "+refs/*:refs/*"},
		{"-C", dir, "reset", "-q", "--hard", "main"},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
		}
	}
	toml, err := os.ReadFile(filepath.Join(board, "git-board.toml"))
	if err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(dir, ".git-board.toml")
	if err := os.WriteFile(cfgPath, toml, 0o644); err != nil {
		t.Fatal(err)
	}
	me, err := os.ReadFile(filepath.Join(board, "me"))
	if err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", dir, "config", "board.me", strings.TrimSpace(string(me))).CombinedOutput(); err != nil {
		t.Fatalf("git config: %v: %s", err, out)
	}
	known := map[string]bool{}
	// every object and ref name of the board: what a write didn't make
	for _, args := range [][]string{{"rev-list", "--all", "--objects"}, {"for-each-ref", "--format=%(refname)"}} {
		out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
		if err != nil {
			t.Fatal(err)
		}
		for _, h := range hex40.FindAllString(string(out), -1) {
			known[h] = true
		}
	}

	b, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b.Close() })
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	b.SetFlatKinds(cfg)
	b.SetHeld(cfg)
	s := &apiServer{cfg: cfg, cfgPath: cfgPath, b: b, gitDir: filepath.Join(dir, ".git"), root: dir}
	h := withCORS(newMux(s, http.NotFoundHandler()))

	reqs, err := os.ReadFile(filepath.Join(goldenDir, "api", "requests.txt"))
	if err != nil {
		t.Fatal(err)
	}
	n := transcriptNames{known: known, start: start}
	out := filepath.Join(goldenDir, "api")
	if update {
		old, _ := filepath.Glob(filepath.Join(out, "[0-9]*.txt"))
		for _, f := range old {
			os.Remove(f)
		}
	}
	i := 0
	for _, line := range strings.Split(string(reqs), "\n") {
		if line = strings.TrimSpace(line); line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		i++
		method, rest, _ := strings.Cut(line, " ")
		path, body, _ := strings.Cut(rest, " ")
		req := httptest.NewRequestWithContext(context.Background(), method, n.back(path), strings.NewReader(n.back(body)))
		req.Host = "localhost:8080" // as the page asks, through its own server (ADR 0013)
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		res := rec.Result()
		data, _ := io.ReadAll(res.Body)
		var got bytes.Buffer
		fmt.Fprintf(&got, "%s\n", line)
		fmt.Fprintf(&got, "\nHTTP %d\nContent-Type: %s\n\n", res.StatusCode, res.Header.Get("Content-Type"))
		got.Write(n.normalise(data))
		name := filepath.Join(out, fmt.Sprintf("%02d-%s.txt", i, transcriptSlug(method+" "+path)))
		if update {
			if err := os.WriteFile(name, got.Bytes(), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("%v: make golden", err)
		}
		if !bytes.Equal(want, got.Bytes()) {
			t.Errorf("%s: the answer differs from %s:\n%s", line, name, firstDiff(want, got.Bytes()))
		}
	}
	if !update {
		recorded, _ := filepath.Glob(filepath.Join(out, "[0-9]*.txt"))
		if len(recorded) != i {
			t.Errorf("%d transcripts recorded, %d requests: make golden", len(recorded), i)
		}
	}
}

var (
	hex40 = regexp.MustCompile(`\b[0-9a-f]{40}\b`)
	// an RFC 3339 time, and Unix seconds of this century's second quarter
	rfc3339  = regexp.MustCompile(`\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(\.\d+)?(Z|[+-]\d\d:\d\d)`)
	unix     = regexp.MustCompile(`\b1[7-9]\d{8}\b`)
	nonSlugT = regexp.MustCompile(`[^a-z0-9]+`)
)

// transcriptNames normalises what a write makes anew: the hashes the
// board didn't hold become <new1>, <new2>… in the order the answers name
// them, and a time from the test's start on <now>.
type transcriptNames struct {
	known map[string]bool
	start time.Time
	news  []string
}

func (n *transcriptNames) normalise(data []byte) []byte {
	s := hex40.ReplaceAllStringFunc(string(data), func(h string) string {
		if n.known[h] {
			return h
		}
		for i, x := range n.news {
			if x == h {
				return fmt.Sprintf("<new%d>", i+1)
			}
		}
		n.news = append(n.news, h)
		return fmt.Sprintf("<new%d>", len(n.news))
	})
	s = rfc3339.ReplaceAllStringFunc(s, func(v string) string {
		if t, err := time.Parse(time.RFC3339, v); err == nil && !t.Before(n.start) {
			return "<now>"
		}
		return v
	})
	s = unix.ReplaceAllStringFunc(s, func(v string) string {
		var sec int64
		fmt.Sscan(v, &sec)
		if sec >= n.start.Unix() {
			return "<now-unix>"
		}
		return v
	})
	return []byte(s)
}

// back puts the hashes <newN> stands for into a request.
func (n *transcriptNames) back(s string) string {
	for i, h := range n.news {
		s = strings.ReplaceAll(s, fmt.Sprintf("<new%d>", i+1), h)
	}
	return s
}

func transcriptSlug(s string) string {
	s = strings.Trim(nonSlugT.ReplaceAllString(strings.ToLower(s), "-"), "-")
	if len(s) > 50 {
		s = strings.TrimRight(s[:50], "-")
	}
	return s
}

// firstDiff is the first line where two texts part.
func firstDiff(want, got []byte) string {
	w, g := strings.Split(string(want), "\n"), strings.Split(string(got), "\n")
	for i := 0; i < max(len(w), len(g)); i++ {
		var wl, gl string
		if i < len(w) {
			wl = w[i]
		}
		if i < len(g) {
			gl = g[i]
		}
		if wl != gl {
			if len(wl) > 300 || len(gl) > 300 {
				for j := 0; j < min(len(wl), len(gl)); j++ {
					if wl[j] != gl[j] {
						lo := max(0, j-80)
						wl, gl = wl[lo:min(len(wl), j+120)], gl[lo:min(len(gl), j+120)]
						break
					}
				}
			}
			return fmt.Sprintf("line %d:\n  want %q\n  got  %q", i+1, wl, gl)
		}
	}
	return "the same lines"
}
