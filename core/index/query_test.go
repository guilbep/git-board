package index

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/guilbep/git-board/core/issue"
	"github.com/guilbep/git-board/core/oplog"
	"github.com/guilbep/git-board/core/people"
	"github.com/guilbep/git-board/core/pgstore"
	"github.com/guilbep/git-board/core/pgstore/pgtest"
)

func TestIndexQueries(t *testing.T) {
	t.Parallel()
	_, s := newRepo(t)
	a := mkIssue(t, s, at(issue.NewCreate("ann", "Crashes on save", "the editor dies"), 100),
		at(issue.NewComment("bob", "me too"), 150),
		at(issue.ChangeLabels("ann", []string{"ui", "bug"}, nil), 500))
	b := mkIssue(t, s, at(issue.NewCreate("bob", "Slow startup", "takes ages"), 200),
		at(issue.ChangeLabels("bob", []string{"perf"}, nil), 250),
		at(issue.SetStatus("bob", issue.Closed), 300))
	c := mkIssue(t, s, at(issue.NewCreate("ann", "Save button misaligned", ""), 300),
		at(issue.ChangeLabels("ann", []string{"ui"}, nil), 350),
		at(issue.NewComment("bob", "a crash here too"), 400))
	d := mkIssue(t, s, at(issue.NewCreate("bob", "Docs typo", "in the readme"), 400),
		at(issue.SetStatus("bob", issue.Closed), 400))
	ix := openIndex(t, s)

	// c and d were both updated at 400: they go by id.
	cd := sorted(c.ID, d.ID)
	byUpdated := []string{b.ID, cd[0], cd[1], a.ID}
	for _, tc := range []struct {
		name string
		q    Query
		want []string
	}{
		{"all", Query{}, byUpdated},
		{"open", Query{Status: issue.Open}, []string{c.ID, a.ID}},
		{"closed", Query{Status: issue.Closed}, []string{b.ID, d.ID}},
		{"label", Query{Labels: []string{"ui"}}, []string{c.ID, a.ID}},
		{"labels", Query{Labels: []string{"bug", "ui"}}, []string{a.ID}},
		{"labels none has", Query{Labels: []string{"ui", "perf"}}, nil},
		{"label bytes", Query{Labels: []string{"UI"}}, nil},
		{"author", Query{Author: "bob"}, []string{b.ID, d.ID}},
		{"text title", Query{Text: "startup"}, []string{b.ID}},
		{"text body", Query{Text: "readme"}, []string{d.ID}},
		{"text comment", Query{Text: "crash"}, []string{c.ID, a.ID}},
		{"combined", Query{Status: issue.Open, Labels: []string{"ui"}, Author: "ann", Text: "button"}, []string{c.ID}},
		{"combined none", Query{Status: issue.Open, Author: "bob"}, nil},
		{"created", Query{Sort: "created"}, []string{a.ID, b.ID, c.ID, d.ID}},
		{"created desc", Query{Sort: "created", Desc: true}, []string{d.ID, c.ID, b.ID, a.ID}},
		{"updated", Query{Sort: "updated"}, byUpdated},
		{"updated desc", Query{Desc: true}, []string{a.ID, cd[1], cd[0], b.ID}},
		{"limit", Query{Limit: 2}, byUpdated[:2]},
		{"limit desc", Query{Desc: true, Limit: 1}, []string{a.ID}},
		{"limit over", Query{Limit: 9}, byUpdated},
	} {
		if got := ids(t, ix, tc.q); !slices.Equal(got, tc.want) {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
	}

	got, err := ix.Issues(t.Context(), Query{Desc: true, Limit: 1})
	want := Summary{ID: a.ID, Title: "Crashes on save", Status: issue.Open, Labels: []string{"bug", "ui"},
		Author: "ann", Created: 100, Updated: 500, Comments: 1}
	if err != nil || len(got) != 1 || !reflect.DeepEqual(got[0], want) {
		t.Fatalf("summary = %+v, %v; want %+v", got, err, want)
	}
	got, err = ix.Issues(t.Context(), Query{Text: "typo"})
	if err != nil || len(got) != 1 || got[0].Labels == nil || len(got[0].Labels) != 0 || got[0].Status != issue.Closed {
		t.Fatalf("summary without labels = %+v, %v", got, err)
	}

	for _, q := range []Query{{Status: "pending"}, {Sort: "title"}, {Limit: -1}} {
		if _, err := ix.Issues(t.Context(), q); err == nil {
			t.Errorf("query %+v: no error", q)
		}
	}
}

func TestSearchTextIsPlainWords(t *testing.T) {
	t.Parallel()
	_, s := newRepo(t)
	crash := mkIssue(t, s, issue.NewCreate("ann", "Crashes on save", ""))
	cafe := mkIssue(t, s, issue.NewCreate("ann", "Café menu", "the title: crashed"))
	ix := openIndex(t, s)
	for _, text := range []string{"crash", `"crash`, "crash*", "-crash", "title:crash", "crash AND", "(crash", "crash OR save", "NOT"} {
		if _, err := ix.Issues(t.Context(), Query{Text: text}); err != nil {
			t.Errorf("text %q: %v", text, err)
		}
	}
	both := sorted(crash.ID, cafe.ID)
	for _, tc := range []struct {
		text string
		want []string
	}{
		{"crash", both},
		{`"crash`, both},
		{"crash*", both},
		{"-crash", both},
		{"crash save", []string{crash.ID}}, // every word
		{"crash menu save", nil},
		{"rash", nil}, // a prefix of a word, not any part of it
		{"cafe", []string{cafe.ID}},
		{"CAFÉ", []string{cafe.ID}},
		{"title:crash", []string{cafe.ID}}, // its tokens in a row
		{"crash:title", nil},
		{"", both},
		{"-", both},
		{`" * -`, both},
	} {
		if got := sorted(ids(t, ix, Query{Text: tc.text})...); !slices.Equal(got, tc.want) {
			t.Errorf("text %q: %v, want %v", tc.text, got, tc.want)
		}
	}
}

func TestIndexAccountOwner(t *testing.T) {
	t.Parallel()
	_, s := newRepo(t)
	acct := people.Account{Server: "github.com", ID: "42", Login: "ann"}
	var pids []string
	for range 2 {
		p, err := people.Create(t.Context(), s, people.NewCreate("", "ann", ""), people.AddAccount("", acct))
		if err != nil {
			t.Fatal(err)
		}
		pids = append(pids, p.ID)
	}
	slices.Sort(pids)
	ix := openIndex(t, s)
	if id, ok, err := ix.ForAccount(t.Context(), "github.com", "42"); err != nil || !ok || id != pids[0] {
		t.Fatalf("owner = %q, %v, %v; want %q", id, ok, err, pids[0])
	}
	if id, ok, err := ix.ForAccount(t.Context(), "github.com", "43"); err != nil || ok || id != "" {
		t.Fatalf("unheld account: %q, %v, %v", id, ok, err)
	}
	if _, err := people.Do(t.Context(), s, pids[0], people.RemoveAccount(pids[0], "github.com", "42")); err != nil {
		t.Fatal(err)
	}
	if id, ok, err := ix.ForAccount(t.Context(), "github.com", "42"); err != nil || !ok || id != pids[1] {
		t.Fatalf("owner after removal = %q, %v, %v; want %q", id, ok, err, pids[1])
	}
}

// Labels lists each label in use once; Resolve finds an id by its prefix.
func TestLabelsAndResolve(t *testing.T) {
	t.Parallel()
	_, s := newRepo(t)
	ix := openIndex(t, s)
	a := mkIssue(t, s, issue.NewCreate("p", "A", ""), issue.ChangeLabels("p", []string{"bug", "ui"}, nil))
	b := mkIssue(t, s, issue.NewCreate("p", "B", ""), issue.ChangeLabels("p", []string{"bug", "gone"}, nil), issue.ChangeLabels("p", nil, []string{"gone"}))
	got, err := ix.Labels(t.Context())
	if err != nil || !slices.Equal(got, []string{"bug", "ui"}) {
		t.Fatalf("labels = %v, %v", got, err)
	}
	for _, id := range []string{a.ID, b.ID} {
		if got, err := ix.Resolve(t.Context(), "issue", id[:12]); err != nil || got != id {
			t.Fatalf("resolve %s = %q, %v", id[:12], got, err)
		}
		if got, err := ix.Resolve(t.Context(), "issue", id); err != nil || got != id {
			t.Fatalf("resolve whole %s = %q, %v", id, got, err)
		}
	}
	common := 0
	for common < 40 && a.ID[common] == b.ID[common] {
		common++
	}
	if _, err := ix.Resolve(t.Context(), "issue", a.ID[:common]); common > 0 && !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("shared prefix %q: err = %v", a.ID[:common], err)
	}
	if _, err := ix.Resolve(t.Context(), "issue", ""); err == nil {
		t.Fatal("empty prefix: want an error")
	}
	for _, bad := range []string{"%", "a_", "A1", strings.Repeat("a", 41)} {
		if _, err := ix.Resolve(t.Context(), "issue", bad); err == nil || errors.Is(err, ErrNoMatch) {
			t.Fatalf("prefix %q: err = %v, want a bad prefix", bad, err)
		}
	}
	missing := "0000000000"
	if a.ID[:10] == missing || b.ID[:10] == missing {
		missing = "ffffffffff"
	}
	if _, err := ix.Resolve(t.Context(), "issue", missing); !errors.Is(err, ErrNoMatch) {
		t.Fatalf("missing: err = %v", err)
	}
	if _, err := ix.Resolve(t.Context(), "sync", "ab"); err == nil {
		t.Fatal("kind sync: want an error")
	}
}

// Labels sorts byte for byte (queries.md), on a PostgreSQL database whose
// collation isn't C too.
func TestLabelsByteOrder(t *testing.T) {
	t.Parallel()
	_, s := newRepo(t)
	mkIssue(t, s, issue.NewCreate("p", "A", ""), issue.ChangeLabels("p", []string{"b", "B", "_z", "a-b", "ab", "Z"}, nil))
	want := []string{"B", "Z", "_z", "a-b", "ab", "b"}
	if got, err := openIndex(t, s).Labels(t.Context()); err != nil || !slices.Equal(got, want) {
		t.Errorf("labels = %q, %v, want %q", got, err, want)
	}
	t.Run("pg collated", func(t *testing.T) {
		t.Parallel()
		p, err := pgstore.Open(t.Context(), pgtest.DSNCollated(t), t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { p.Close() })
		ix, err := Open(t.Context(), gitIndexedOnPG{s, p})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { ix.Close() })
		if got, err := ix.Labels(t.Context()); err != nil || !slices.Equal(got, want) {
			t.Errorf("labels = %q, %v, want %q", got, err, want)
		}
	})
}

// An issue whose text is bigger than a tsvector takes fails no query: its
// text is indexed up to a bound, and the other issues still answer.
func TestHugeTextFailsNoQuery(t *testing.T) {
	t.Parallel()
	_, s := newRepo(t)
	ix := openIndex(t, s)
	mkIssue(t, s, issue.NewCreate("p", "small", "fine"))
	var b strings.Builder
	b.WriteString("needle ")
	for i := range 150000 {
		fmt.Fprintf(&b, "%08x ", uint32(i)*2654435761)
	}
	mkIssue(t, s, issue.NewCreate("p", "log dump", b.String()))
	if got, err := ix.Issues(t.Context(), Query{}); err != nil || len(got) != 2 {
		t.Fatalf("issues = %d, %v, want 2", len(got), err)
	}
	if got, err := ix.Issues(t.Context(), Query{Text: "needle"}); err != nil || len(got) != 1 || got[0].Title != "log dump" {
		t.Fatalf("search = %+v, %v", got, err)
	}
}

// in: narrows where each word is looked for, and MatchText, the search
// without the index, answers what the index answers.
func TestSearchInAgreesWithMatchText(t *testing.T) {
	t.Parallel()
	_, s := newRepo(t)
	all := []*issue.Issue{
		mkIssue(t, s, issue.NewCreate("ann", "Crash on save", "the editor dies"), issue.NewComment("bob", "slow too")),
		mkIssue(t, s, issue.NewCreate("bob", "Slow startup", "a crash in the logs"), issue.NewComment("ann", "Café closed")),
		mkIssue(t, s, issue.NewCreate("ann", "Docs", ""), issue.NewComment("bob", "crash here"), issue.NewComment("bob", "save me")),
	}
	a, b, c := all[0].ID, all[1].ID, all[2].ID
	ix := openIndex(t, s)
	for _, tc := range []struct {
		text string
		in   []string
		want []string
	}{
		{"crash", nil, sorted(a, b, c)},
		{"crash", []string{"title"}, []string{a}},
		{"crash", []string{"body"}, []string{b}},
		{"crash", []string{"comments"}, []string{c}},
		{"crash", []string{"title", "comments"}, sorted(a, c)},
		{"crash save", []string{"title"}, []string{a}},
		{"crash save", []string{"comments"}, []string{c}}, // each word in one comment or another
		{"slow", []string{"body", "comments"}, []string{a}},
		{"cafe", []string{"comments"}, []string{b}},
		{"in the", []string{"body"}, []string{b}},
		{"the logs", []string{"title"}, nil},
		{"crash-here", []string{"comments"}, []string{c}}, // tokens in a row
		{"crash-here", []string{"title", "body"}, nil},
		{"-", []string{"title"}, sorted(a, b, c)},
	} {
		got := sorted(ids(t, ix, Query{Text: tc.text, In: tc.in})...)
		if !slices.Equal(got, tc.want) {
			t.Errorf("%q in %v: %v, want %v", tc.text, tc.in, got, tc.want)
		}
		var mem []string
		for _, is := range all {
			if MatchText(tc.text, texts(is, tc.in)...) {
				mem = append(mem, is.ID)
			}
		}
		if mem = sorted(mem...); !slices.Equal(mem, got) {
			t.Errorf("%q in %v: MatchText %v, the index %v", tc.text, tc.in, mem, got)
		}
	}
	if _, err := ix.Issues(t.Context(), Query{Text: "x", In: []string{"labels"}}); err == nil {
		t.Error("in labels: no error")
	}
}

// texts are an issue's texts in names (all with none), as MatchText takes them.
func texts(is *issue.Issue, in []string) []string {
	if len(in) == 0 {
		in = Texts
	}
	var comments []string
	for _, c := range is.Comments {
		comments = append(comments, c.Body)
	}
	byName := map[string]string{"title": is.Title, "body": is.Body, "comments": strings.Join(comments, "\n")}
	var res []string
	for _, n := range in {
		res = append(res, byName[n])
	}
	return res
}

// States answers each issue as issue.Apply folds it, at the head it was
// indexed at, and the broken ones apart; cached ones aren't read.
func TestStatesAreTheFolds(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	_, s := newRepo(t)
	a := mkIssue(t, s, issue.NewCreate("ann", "Crash on save", "It crashes."), issue.ChangeLabels("ann", []string{"status:ready"}, nil))
	c := issue.NewCommentMeta("bob", "Seen too", map[string]string{"k": "v"})
	c.Origin = &oplog.Origin{Forge: "github.com/o/r", Event: "comment:1"}
	if _, err := issue.Do(ctx, s, a.ID, c, issue.SetStatus("bob", issue.Closed)); err != nil {
		t.Fatal(err)
	}
	b := mkIssue(t, s, issue.NewCreate("bob", "Gone", ""))
	if _, err := issue.Do(ctx, s, b.ID, issue.Delete("bob", b.BodyOp)); err != nil {
		t.Fatal(err)
	}
	bad, err := oplog.Create(ctx, s, "issue", "", []oplog.Op{{Kind: "bogus", Author: "ann", Time: 1}})
	if err != nil {
		t.Fatal(err)
	}
	ix := openIndex(t, s)
	none := func(string, string) *issue.Issue { return nil }
	states, broken, err := ix.States(ctx, none)
	if err != nil {
		t.Fatal(err)
	}
	if len(broken) != 1 || broken[0].ID != bad.ID {
		t.Fatalf("broken = %+v, want %s", broken, bad.ID)
	}
	if len(states) != 1 {
		t.Fatalf("states = %v, want %s alone", states, a.ID)
	}
	head, err := s.Ref(oplog.RefName("issue", a.ID))
	if err != nil {
		t.Fatal(err)
	}
	want, err := issue.Load(s, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := states[a.ID]; got.Head != head.String() || !reflect.DeepEqual(got.Issue, want) {
		t.Fatalf("state = %s %+v\nwant %s %+v", got.Head, got.Issue, head, want)
	}
	mine := &issue.Issue{ID: a.ID}
	states, _, err = ix.States(ctx, func(id, h string) *issue.Issue {
		if id == a.ID && h == head.String() {
			return mine
		}
		return nil
	})
	if err != nil || states[a.ID].Issue != mine {
		t.Fatalf("cached: %+v, %v; want the caller's", states[a.ID], err)
	}
}
