package issuesync

import (
	"context"
	"encoding/json"
	"github.com/guilbep/git-board/next/store"
	bsync "github.com/guilbep/git-board/next/sync"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	gosync "sync"
	"testing"
	"time"

	"github.com/guilbep/git-board/next/domain/issues"
	"github.com/guilbep/git-board/next/domain/people"
	"github.com/guilbep/git-board/next/forge"
	"github.com/guilbep/git-board/next/forge/fake"
	"github.com/guilbep/git-board/next/kernel"
	"github.com/guilbep/git-board/next/store/git"
	"github.com/guilbep/git-board/next/view"
)

const key = "example.com/p"

// personOfAccount is the person owning the forge user's account, on the fake's
// server.
func personOfAccount(t *testing.T, s *kernel.Board, userID string) *Person {
	t.Helper()
	d, err := loadDirectory(s)
	if err != nil {
		t.Fatal(err)
	}
	p, ok := d.ForAccount("example.com", userID)
	if !ok {
		t.Fatalf("no person for user %s", userID)
	}
	return p
}

func persons(t *testing.T, s *kernel.Board) []string {
	t.Helper()
	ids, err := ListPersons(s)
	if err != nil {
		t.Fatal(err)
	}
	return ids
}

// stamp checks an op's author and origin, and that its time is the pull's,
// between t0 and t1.
func stamp(t *testing.T, e kernel.Entry, author string, origin *kernel.Origin, t0, t1 int64) {
	t.Helper()
	if e.Author != author || !reflect.DeepEqual(e.Origin, origin) || e.Time < t0 || e.Time > t1 {
		t.Errorf("%s = author %s, origin %+v, time %d; want %s, %+v, in [%d, %d]",
			e.Kind, e.Author, e.Origin, e.Time, author, origin, t0, t1)
	}
}

func newRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	return dir
}

func open(t *testing.T, dir string) *kernel.Board {
	t.Helper()
	g, err := git.Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	v, err := view.Open(t.Context(), filepath.Join(t.TempDir(), "view.db"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := kernel.Open(t.Context(), g, v, issues.Kind, people.Kind)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func newStore(t *testing.T) *kernel.Board { return open(t, newRepo(t)) }

func ann() forge.Account { return forge.Account{ID: "7", Login: "ann"} }

func sample() forge.Issue {
	return forge.Issue{ID: "100", Number: "1", URL: "https://example.com/p/1", Author: ann(), Created: 1000,
		Title: "T", Body: "B", Status: "closed", Labels: []string{"bug", "assignee:bob"},
		Comments: []forge.Comment{{ID: "c1", Body: "hi", Author: forge.Account{ID: "8", Login: "bob"}, Time: 1100}}}
}

func pull(t *testing.T, s *kernel.Board, f forge.Forge) Report {
	t.Helper()
	r, err := Pull(context.Background(), s, Options{}, f)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func only(t *testing.T, s *kernel.Board) *Issue {
	t.Helper()
	ids, err := ListIssues(s)
	if err != nil || len(ids) != 1 {
		t.Fatalf("issues = %v, %v", ids, err)
	}
	is, err := LoadIssue(s, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	return is
}

func refs(t *testing.T, s *kernel.Board) map[string]string {
	t.Helper()
	m := map[string]string{}
	for _, kind := range []string{"issue", "person", "sync"} {
		heads, err := s.Store().Heads(t.Context(), kind)
		if err != nil {
			t.Fatal(err)
		}
		for _, h := range heads {
			m["refs/board/"+kind+"/"+h.ID] = h.Version
		}
	}
	return m
}

func TestPullCreatesIssue(t *testing.T) {
	t.Parallel()
	s, f := newStore(t), fake.New(key)
	f.Add(sample())
	t0 := time.Now().Unix()
	if r := pull(t, s, f); r != (Report{Created: 1}) {
		t.Fatalf("report = %+v", r)
	}
	t1 := time.Now().Unix()
	a, b := personOfAccount(t, s, "7").ID, personOfAccount(t, s, "8").ID
	is := only(t, s)
	issueOrigin := &kernel.Origin{Forge: key, Event: "issue:100"}
	if is.Title != "T" || is.Body != "B" || is.Status != Closed || !reflect.DeepEqual(is.Labels, []string{"bug"}) ||
		is.Author != a || is.Created != 1000 || !reflect.DeepEqual(is.Origin, issueOrigin) {
		t.Fatalf("issue = %+v", is)
	}
	if len(is.Comments) != 1 || is.Comments[0].Body != "hi" || is.Comments[0].Author != b || is.Comments[0].Time != 1100 ||
		!reflect.DeepEqual(is.Comments[0].Origin, &kernel.Origin{Forge: key, Event: "comment:c1"}) {
		t.Fatalf("comments = %+v", is.Comments)
	}
	var kinds []string
	for _, e := range mustLog(t, s, is.ID) {
		kinds = append(kinds, e.Kind)
		if e.Kind == "set-status" || e.Kind == "change-labels" {
			// The forge's time of the change (the fake's Updated is
			// its Created), not the pull's.
			stamp(t, e, a, issueOrigin, 1000, 1000)
		}
	}
	if want := []string{"create", "set-status", "change-labels", "comment"}; !reflect.DeepEqual(kinds, want) {
		t.Fatalf("issue ops = %v, want %v", kinds, want)
	}
	st, err := loadState(s, key)
	if err != nil {
		t.Fatal(err)
	}
	if st.links["100"] != is.ID {
		t.Fatalf("links = %v", st.links)
	}
	want := map[string]string{"title": "T", "body": "B", "status": "closed", "labels": "bug", "comment:c1": "hi"}
	if !reflect.DeepEqual(st.seen[is.ID], want) {
		t.Fatalf("seen = %v", st.seen[is.ID])
	}
	// The sync entity: the link, then each seen, in sync.md's order.
	var got []string
	for _, e := range stateLog(t, s, key) {
		stamp(t, e, a, nil, t0, t1)
		if e.Kind == "link" {
			var d bsync.LinkData
			if err := json.Unmarshal(e.Data, &d); err != nil || d != (bsync.LinkData{Issue: is.ID, ForgeID: "100", Number: "1", URL: "https://example.com/p/1"}) {
				t.Fatalf("link = %+v, %v", d, err)
			}
			got = append(got, "link")
			continue
		}
		var d bsync.SeenData
		if err := json.Unmarshal(e.Data, &d); err != nil || d.Issue != is.ID {
			t.Fatalf("seen = %+v, %v", d, err)
		}
		got = append(got, d.Field)
	}
	if want := []string{"link", "title", "body", "status", "labels", "comment:c1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("sync ops = %v, want %v", got, want)
	}
}

// A pulled issue's authors are persons, made with the forge account and the
// login as name, in one pack with empty authors.
func TestPullCreatesPeople(t *testing.T) {
	t.Parallel()
	s, f := newStore(t), fake.New(key)
	f.Add(sample())
	pull(t, s, f)
	for user, login := range map[string]string{"7": "ann", "8": "bob"} {
		p := personOfAccount(t, s, user)
		if want := []Account{{Server: "example.com", ID: user, Login: login}}; p.Name != login || p.Email != "" || !reflect.DeepEqual(p.Accounts, want) {
			t.Fatalf("person = %+v", p)
		}
		l, err := loadLog(s, "person", p.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range l.Entries {
			if e.Author != "" || string(e.ID)[:40] != p.ID {
				t.Fatalf("op %s by %q, want the first pack's, with no author", e.ID, e.Author)
			}
		}
	}
	if n := len(persons(t, s)); n != 2 {
		t.Fatalf("persons = %d, want 2", n)
	}
}

// Two issues by one account, and its comments on both, give one person: later
// authors in the same pull find the person made for the first.
func TestPullReusesPerson(t *testing.T) {
	t.Parallel()
	s, f := newStore(t), fake.New(key)
	f.Add(sample())
	two := sample()
	two.ID, two.Number = "200", "2"
	f.Add(two)
	if r := pull(t, s, f); r != (Report{Created: 2}) {
		t.Fatalf("report = %+v", r)
	}
	if n := len(persons(t, s)); n != 2 {
		t.Fatalf("persons = %d, want 2", n)
	}
	ids, _ := ListIssues(s)
	a := personOfAccount(t, s, "7").ID
	for _, id := range ids {
		if is, err := LoadIssue(s, id); err != nil || is.Author != a {
			t.Fatalf("issue = %+v, %v; want author %s", is, err, a)
		}
	}
}

// A login renamed on the forge keeps its person, whose account takes the new
// login, written by the person itself.
func TestPullLoginRename(t *testing.T) {
	t.Parallel()
	s, f := newStore(t), fake.New(key)
	f.Add(sample())
	pull(t, s, f)
	before := personOfAccount(t, s, "7")
	f.Edit("1", func(i *forge.Issue) { i.Author.Login = "ann2" })
	pull(t, s, f)
	p := personOfAccount(t, s, "7")
	if want := []Account{{Server: "example.com", ID: "7", Login: "ann2"}}; p.ID != before.ID || p.Name != "ann" || !reflect.DeepEqual(p.Accounts, want) {
		t.Fatalf("person = %+v, was %+v", p, before)
	}
	if n := len(persons(t, s)); n != 2 {
		t.Fatalf("persons = %d, want 2", n)
	}
	l, _ := loadLog(s, "person", p.ID)
	if last := l.Entries[len(l.Entries)-1]; last.Kind != "add-account" || last.Author != p.ID {
		t.Fatalf("last op = %s by %q", last.Kind, last.Author)
	}
	if is := only(t, s); is.Author != p.ID {
		t.Fatalf("author = %s", is.Author)
	}
	refsBefore := refs(t, s)
	if r := pull(t, s, f); r != (Report{Unchanged: 1}) || !reflect.DeepEqual(refsBefore, refs(t, s)) {
		t.Fatalf("the pull after the rename wrote: %+v", r)
	}
}

// A person that fails to load stops the pull before it writes anything.
func TestPullStopsOnBrokenPerson(t *testing.T) {
	t.Parallel()
	s, f := newStore(t), fake.New(key)
	if _, err := rawCreate(t, s, "person", "", []kernel.Op{{Kind: "bogus", Time: 1}}); err != nil {
		t.Fatal(err)
	}
	f.Add(sample())
	before := refs(t, s)
	if _, err := Pull(t.Context(), s, Options{}, f); err == nil {
		t.Fatal("want an error")
	}
	if !reflect.DeepEqual(before, refs(t, s)) {
		t.Fatal("wrote")
	}
}

// The sync entity's ops need an author: a write without one is refused, and
// one already stored breaks the entity.
func TestStateEmptyAuthor(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	st, _ := loadState(s, key)
	if err := st.write(t.Context(), s, key, []kernel.Op{linkOp("", 1, "i", "100", "1", "")}); err == nil {
		t.Fatal("a write with no author landed")
	}
	if len(refs(t, s)) != 0 {
		t.Fatal("wrote")
	}
	if _, err := rawCreate(t, s, "sync", bsync.StateID(key), []kernel.Op{seenOp("", 1, "i", "title", "T")}); err != nil {
		t.Fatal(err)
	}
	if _, err := loadState(s, key); err == nil {
		t.Fatal("an op with no author loaded")
	}
}

func TestPullTwiceWritesNothing(t *testing.T) {
	t.Parallel()
	s, f := newStore(t), fake.New(key)
	f.Add(sample())
	pull(t, s, f)
	before := refs(t, s)
	if r := pull(t, s, f); r != (Report{Unchanged: 1}) {
		t.Fatalf("report = %+v", r)
	}
	if !reflect.DeepEqual(before, refs(t, s)) {
		t.Fatal("refs moved")
	}
}

func TestPullForgeChange(t *testing.T) {
	t.Parallel()
	s, f := newStore(t), fake.New(key)
	f.Add(sample())
	pull(t, s, f)
	f.Edit("1", func(i *forge.Issue) { i.Title = "T2"; i.Labels = append(i.Labels, "ui"); i.Comments[0].Body = "hi!" })
	t0 := time.Now().Unix()
	if r := pull(t, s, f); r != (Report{Updated: 1}) {
		t.Fatalf("report = %+v", r)
	}
	is := only(t, s)
	if is.Title != "T2" || !reflect.DeepEqual(is.Labels, []string{"bug", "ui"}) || is.Comments[0].Body != "hi!" {
		t.Fatalf("issue = %+v", is)
	}
	st, _ := loadState(s, key)
	if st.seen[is.ID]["title"] != "T2" || st.seen[is.ID]["labels"] != "bug\nui" {
		t.Fatalf("seen = %v", st.seen[is.ID])
	}
	t1 := time.Now().Unix()
	a, b := personOfAccount(t, s, "7").ID, personOfAccount(t, s, "8").ID
	var kinds []string
	for _, e := range mustLog(t, s, is.ID)[4:] { // after the first pull's
		kinds = append(kinds, e.Kind)
		switch e.Kind {
		case "set-title", "change-labels":
			stamp(t, e, a, &kernel.Origin{Forge: key, Event: "issue:100"}, t0, t1)
		case "edit-text":
			stamp(t, e, b, &kernel.Origin{Forge: key, Event: "comment:c1"}, t0, t1)
		}
	}
	if want := []string{"set-title", "change-labels", "edit-text"}; !reflect.DeepEqual(kinds, want) {
		t.Fatalf("ops = %v, want %v", kinds, want)
	}
	before := refs(t, s)
	if r := pull(t, s, f); r != (Report{Unchanged: 1}) || !reflect.DeepEqual(before, refs(t, s)) {
		t.Fatalf("second pull: %+v, or refs moved", r)
	}
}

func TestPullKeepsLocalChange(t *testing.T) {
	t.Parallel()
	s, f := newStore(t), fake.New(key)
	f.Add(sample())
	pull(t, s, f)
	is := only(t, s)
	if _, err := DoIssue(t.Context(), s, is.ID, SetTitle("me", "mine")); err != nil {
		t.Fatal(err)
	}
	before := refs(t, s)
	if r := pull(t, s, f); r != (Report{Unchanged: 1}) {
		t.Fatalf("report = %+v", r)
	}
	if only(t, s).Title != "mine" || !reflect.DeepEqual(before, refs(t, s)) {
		t.Fatal("local change not kept, or something written")
	}
	// both changed, the forge earlier: git-board's is the later, left for
	// the push.
	f.Edit("1", func(i *forge.Issue) { i.Title = "theirs"; i.Changed["title"] = 1 })
	if r := pull(t, s, f); r != (Report{Unchanged: 1}) {
		t.Fatalf("conflict report = %+v", r)
	}
	if only(t, s).Title != "mine" || !reflect.DeepEqual(before, refs(t, s)) {
		t.Fatal("conflict wrote something")
	}
	// the forge later: it wins.
	f.Edit("1", func(i *forge.Issue) { i.Title = "later"; i.Changed["title"] = time.Now().Unix() + 100 })
	if r := pull(t, s, f); r != (Report{Updated: 1}) || only(t, s).Title != "later" {
		t.Fatalf("report = %+v, title %q", r, only(t, s).Title)
	}
}

func TestPullNewComment(t *testing.T) {
	t.Parallel()
	s, f := newStore(t), fake.New(key)
	f.Add(sample())
	pull(t, s, f)
	f.Edit("1", func(i *forge.Issue) {
		i.Comments = append(i.Comments, forge.Comment{ID: "c2", Body: "two", Author: ann(), Time: 1200})
	})
	if r := pull(t, s, f); r != (Report{Updated: 1}) {
		t.Fatalf("report = %+v", r)
	}
	if n := len(only(t, s).Comments); n != 2 {
		t.Fatalf("comments = %d", n)
	}
	before := refs(t, s)
	pull(t, s, f)
	if !reflect.DeepEqual(before, refs(t, s)) || len(only(t, s).Comments) != 2 {
		t.Fatal("second pull wrote")
	}
}

// Two first writes of a project's sync entity, by two Stores on one
// repository that both found none, both land: the second appends.
func TestStateFirstWritesBothLand(t *testing.T) {
	t.Parallel()
	dir := newRepo(t)
	var stores [2]*kernel.Board
	var states [2]*state
	for i := range stores {
		stores[i] = open(t, dir)
		st, err := loadState(stores[i], key)
		if err != nil || st.exists {
			t.Fatalf("state = %+v, %v", st, err)
		}
		states[i] = st
	}
	for i, fid := range []string{"100", "200"} {
		if err := states[i].write(t.Context(), stores[i], key, []kernel.Op{linkOp("a", 1, "issue"+fid, fid, fid, "")}); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}
	st, err := loadState(open(t, dir), key)
	if want := map[string]string{"100": "issue100", "200": "issue200"}; err != nil || !reflect.DeepEqual(st.links, want) {
		t.Fatalf("links = %v, %v; want %v", st.links, err, want)
	}
}

func TestPullBadStatus(t *testing.T) {
	t.Parallel()
	s, f := newStore(t), fake.New(key)
	bad := sample()
	bad.Status = "merged"
	f.Add(bad)
	if _, err := Pull(context.Background(), s, Options{}, f); err == nil {
		t.Fatal("want an error")
	}
	if len(refs(t, s)) != 0 {
		t.Fatal("wrote")
	}
}

func loadIssueLog(t *testing.T, s *kernel.Board, id string) ([]kernel.Entry, error) {
	t.Helper()
	l, err := loadLog(s, "issue", id)
	if err != nil {
		t.Fatal(err)
	}
	return l.Entries, nil
}

func labels(t *testing.T, s *kernel.Board) []string {
	t.Helper()
	return only(t, s).Labels
}

func TestPullLabelsMerge(t *testing.T) {
	t.Parallel()
	s, f := newStore(t), fake.New(key)
	one := sample()
	one.Labels = []string{"bug"}
	f.Add(one)
	pull(t, s, f)
	is := only(t, s)
	// ui added in git-board, docs on the forge.
	if _, err := DoIssue(t.Context(), s, is.ID, ChangeLabels("me", []string{"ui"}, nil)); err != nil {
		t.Fatal(err)
	}
	f.Edit("1", func(i *forge.Issue) { i.Labels = []string{"bug", "docs"} })
	if r := pull(t, s, f); r != (Report{Updated: 1}) {
		t.Fatalf("report = %+v", r)
	}
	if got := labels(t, s); !reflect.DeepEqual(got, []string{"bug", "docs", "ui"}) {
		t.Fatalf("labels = %v", got)
	}
	st, _ := loadState(s, key)
	if st.seen[is.ID]["labels"] != "bug\ndocs" {
		t.Fatalf("seen = %q", st.seen[is.ID]["labels"])
	}
	// the forge drops bug (git-board has it): removed. It drops docs after
	// git-board dropped it too: nothing to remove.
	if _, err := DoIssue(t.Context(), s, is.ID, ChangeLabels("me", nil, []string{"docs"})); err != nil {
		t.Fatal(err)
	}
	f.Edit("1", func(i *forge.Issue) { i.Labels = nil })
	pull(t, s, f)
	if got := labels(t, s); !reflect.DeepEqual(got, []string{"ui"}) {
		t.Fatalf("labels = %v", got)
	}
	st, _ = loadState(s, key)
	if st.seen[is.ID]["labels"] != "" {
		t.Fatalf("seen = %q", st.seen[is.ID]["labels"])
	}
}

// Labels both sides already agree on write no operation, only a seen.
func TestPullLabelsAgreedWriteNoOp(t *testing.T) {
	t.Parallel()
	s, f := newStore(t), fake.New(key)
	one := sample()
	one.Labels = []string{"bug", "ui"}
	f.Add(one)
	pull(t, s, f)
	is := only(t, s)
	// git-board drops ui and adds docs; the forge drops ui and adds docs.
	if _, err := DoIssue(t.Context(), s, is.ID, ChangeLabels("me", []string{"docs"}, []string{"ui"})); err != nil {
		t.Fatal(err)
	}
	f.Edit("1", func(i *forge.Issue) { i.Labels = []string{"bug", "docs"} })
	n := len(mustLog(t, s, is.ID))
	if r := pull(t, s, f); r != (Report{Updated: 1}) {
		t.Fatalf("report = %+v", r)
	}
	if got := len(mustLog(t, s, is.ID)); got != n {
		t.Fatalf("ops = %d, want %d", got, n)
	}
	if got := labels(t, s); !reflect.DeepEqual(got, []string{"bug", "docs"}) {
		t.Fatalf("labels = %v", got)
	}
	st, _ := loadState(s, key)
	if st.seen[is.ID]["labels"] != "bug\ndocs" {
		t.Fatalf("seen = %q", st.seen[is.ID]["labels"])
	}
}

func TestPullLocalAssigneeIsNotALabel(t *testing.T) {
	t.Parallel()
	s, f := newStore(t), fake.New(key)
	one := sample()
	one.Labels = []string{"bug"}
	f.Add(one)
	pull(t, s, f)
	is := only(t, s)
	if _, err := DoIssue(t.Context(), s, is.ID, ChangeLabels("me", []string{"assignee:x", "Assignee:y"}, nil)); err != nil {
		t.Fatal(err)
	}
	// The forge drops bug and adds docs. The assignee labels stay and
	// seen has none of them.
	f.Edit("1", func(i *forge.Issue) { i.Labels = []string{"docs"} })
	pull(t, s, f)
	if got := labels(t, s); !reflect.DeepEqual(got, []string{"Assignee:y", "assignee:x", "docs"}) {
		t.Fatalf("labels = %v", got)
	}
	st, _ := loadState(s, key)
	if st.seen[is.ID]["labels"] != "docs" {
		t.Fatalf("seen = %q", st.seen[is.ID]["labels"])
	}
	// Nothing changed since: no write.
	before := refs(t, s)
	if r := pull(t, s, f); r != (Report{Unchanged: 1}) || !reflect.DeepEqual(before, refs(t, s)) {
		t.Fatalf("report = %+v", r)
	}
}

func TestPullBothChangedToTheSame(t *testing.T) {
	t.Parallel()
	s, f := newStore(t), fake.New(key)
	f.Add(sample())
	pull(t, s, f)
	is := only(t, s)
	if _, err := DoIssue(t.Context(), s, is.ID, SetTitle("me", "same")); err != nil {
		t.Fatal(err)
	}
	f.Edit("1", func(i *forge.Issue) { i.Title = "same" })
	n := len(mustLog(t, s, is.ID))
	if r := pull(t, s, f); r != (Report{Updated: 1}) {
		t.Fatalf("report = %+v", r)
	}
	st, _ := loadState(s, key)
	if st.seen[is.ID]["title"] != "same" {
		t.Fatalf("seen = %v", st.seen[is.ID])
	}
	if len(mustLog(t, s, is.ID)) != n {
		t.Fatal("an issue op was written")
	}
}

func mustLog(t *testing.T, s *kernel.Board, id string) []kernel.Entry {
	t.Helper()
	l, _ := loadIssueLog(t, s, id)
	return l
}

// A link whose issue isn't an id is skipped, like a link to an issue not
// here yet: the pull goes on and writes nothing for it.
func TestPullSkipsLinkWithBadIssueID(t *testing.T) {
	t.Parallel()
	s, f := newStore(t), fake.New(key)
	st, err := loadState(s, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.write(t.Context(), s, key, []kernel.Op{linkOp("a", 1, "notanid", "100", "1", "")}); err != nil {
		t.Fatal(err)
	}
	f.Add(sample())
	before := refs(t, s)
	r, err := Pull(t.Context(), s, Options{}, f)
	if err != nil || r != (Report{Unchanged: 1}) {
		t.Fatalf("report = %+v, %v; want one unchanged", r, err)
	}
	if !reflect.DeepEqual(before, refs(t, s)) {
		t.Fatal("refs moved")
	}
}

// A forge assignee who wrote nothing on the issue gets a person, so the
// assignee:<login> label pulled for them names someone, and the next sync
// keeps them assigned on the forge instead of unassigning them.
func TestPulledAssigneeWithoutPersonStaysAssigned(t *testing.T) {
	t.Parallel()
	s, f := newStore(t), fake.New(key)
	f.Add(forge.Issue{ID: "100", Number: "1", URL: "https://example.com/p/1", Author: ann(), Created: 1000,
		Title: "T", Body: "B", Status: "open", Assignees: []forge.Account{{ID: "9", Login: "carol"}}})
	var warns []string
	opt := Options{Warn: func(s string) { warns = append(warns, s) }}
	for range 2 {
		if _, err := Sync(t.Context(), s, opt, f); err != nil {
			t.Fatal(err)
		}
	}
	if fi, _ := f.Get("1"); len(fi.Assignees) != 1 || fi.Assignees[0].Login != "carol" {
		t.Errorf("forge assignees = %v, want carol", fi.Assignees)
	}
	if got := only(t, s).Labels; !slices.Contains(got, "assignee:carol") {
		t.Errorf("labels = %v, want assignee:carol", got)
	}
	if len(warns) > 0 {
		t.Errorf("warnings: %q", warns)
	}
}

// countingPacks counts the reads of each kind's entities.
type countingPacks struct {
	store.Datastore
	mu    *gosync.Mutex
	reads map[string]int
}

func (c countingPacks) Load(ctx context.Context, refs []store.Ref, each func(store.Ref, [][]byte, error) error) error {
	c.mu.Lock()
	for _, r := range refs {
		c.reads[r.Kind]++
	}
	c.mu.Unlock()
	return c.Datastore.Load(ctx, refs, each)
}

// A run reads each person once.
func TestAuthorsReadEachPersonOnce(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	for _, name := range []string{"ann", "bob", "carol"} {
		if _, err := CreatePerson(t.Context(), s, NewPerson("", name, "")); err != nil {
			t.Fatal(err)
		}
	}
	c := countingPacks{s.Store(), &gosync.Mutex{}, map[string]int{}}
	v, err := view.Open(t.Context(), filepath.Join(t.TempDir(), "view.db"))
	if err != nil {
		t.Fatal(err)
	}
	counted, err := kernel.Open(t.Context(), c, v, issues.Kind, people.Kind)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	if _, err := loadAuthors(counted); err != nil {
		t.Fatal(err)
	}
	if got := c.reads["person"]; got != 3 {
		t.Errorf("read persons %d times, want 3", got)
	}
}
