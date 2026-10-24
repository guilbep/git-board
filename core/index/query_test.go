package index

import (
	"reflect"
	"slices"
	"testing"

	"github.com/guilbep/git-board/core/issue"
	"github.com/guilbep/git-board/core/people"
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
