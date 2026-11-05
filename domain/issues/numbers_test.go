package issues

import (
	"strings"
	"testing"

	bsync "github.com/guilbep/git-board/sync"
)

// gh is a GitHub link of repo's issue n.
func gh(repo, n string) bsync.Record {
	return bsync.Record{LinkData: bsync.LinkData{Number: n, URL: "https://github.com/" + repo + "/issues/" + n}, Forge: bsync.StateID("github.com/" + repo)}
}

// Below start, an issue keeps its number on the project's one GitHub
// repository (ADR 0016; datastores.md, The counter): the counter's
// `github` line, else the board's GitHub forge, else the one repository
// the links are on. Two repositories and none named: no GitHub number
// counts, rather than two issues on one number.
func TestNumberBelowStartOneRepo(t *testing.T) {
	t.Parallel()
	x, y := strings.Repeat("a", 40), strings.Repeat("b", 40)
	recs := map[string][]bsync.Record{
		x: {gh("o/a", "5")},
		y: {gh("o/b", "5")},
	}
	number := func(c Counter, fallback string, id string) int {
		var is Issue
		is.forged(recs[id])
		svc := &Service{GitHub: fallback}
		c = svc.withRepo(c, recs)
		return c.NumberOf(id, is.Forges)
	}
	c := Counter{Prefix: "P", Start: 100, ByID: map[string]int{}}

	// two repos, none named: neither 5
	if a, b := number(c, "", x), number(c, "", y); a != 0 || b != 0 {
		t.Fatalf("two repos, none named: x %d, y %d; want none", a, b)
	}
	// the board's GitHub forge names it
	if a, b := number(c, "o/a", x), number(c, "o/a", y); a != 5 || b != 0 {
		t.Fatalf("o/a configured: x %d, y %d", a, b)
	}
	// the counter's line wins over the configuration, any case
	cb := c
	cb.GitHub = "O/B"
	if a, b := number(cb, "o/a", x), number(cb, "o/a", y); a != 0 || b != 5 {
		t.Fatalf("counter names o/b: x %d, y %d", a, b)
	}
	// Y later created on o/a too, as #9: its number doesn't move to 9
	recs[y] = []bsync.Record{gh("o/a", "9"), gh("o/b", "5")}
	if b := number(cb, "", y); b != 5 {
		t.Fatalf("y with a second link: %d, want 5", b)
	}
	// one repo on the whole board: its numbers, as before
	one := map[string][]bsync.Record{x: {gh("o/a", "5")}, y: {gh("o/a", "6")}}
	svc := &Service{}
	if r := svc.withRepo(c, one).GitHub; r != "o/a" {
		t.Fatalf("one repo: %q", r)
	}
	// a number at start or above isn't the issue's own
	var is Issue
	is.forged([]bsync.Record{gh("o/a", "100")})
	if n := (Counter{Start: 100, GitHub: "o/a"}).NumberOf(x, is.Forges); n != 0 {
		t.Fatalf("#100 at start 100: %d", n)
	}
}

// The counter's github line reads and writes back; v1 skips it.
func TestCounterGitHubLine(t *testing.T) {
	t.Parallel()
	c, err := ParseCounter([]byte("prefix P\nstart 100\ngithub o/a\n100 " + strings.Repeat("a", 40) + "\n"))
	if err != nil || c.GitHub != "o/a" {
		t.Fatalf("%+v, %v", c, err)
	}
	if got := string(c.Bytes()); got != "prefix P\nstart 100\ngithub o/a\n100 "+strings.Repeat("a", 40)+"\n" {
		t.Fatalf("bytes %q", got)
	}
	c.GitHub = ""
	if got := string(c.Bytes()); strings.Contains(got, "github") {
		t.Fatalf("no repo, bytes %q", got)
	}
	// one that isn't owner/repo is skipped, as a key a later version wrote
	if c, err := ParseCounter([]byte("prefix P\nstart 100\ngithub ../x\n")); err != nil || c.GitHub != "" {
		t.Fatalf("a github line that isn't owner/repo: %+v, %v", c, err)
	}
}
