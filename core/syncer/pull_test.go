package syncer

import (
	"context"
	"encoding/json"
	"maps"
	"os/exec"
	"reflect"
	"testing"
	"time"

	"github.com/guilbep/git-board/core/forge"
	"github.com/guilbep/git-board/core/forge/fake"
	"github.com/guilbep/git-board/core/gitstore"
	"github.com/guilbep/git-board/core/issue"
	"github.com/guilbep/git-board/core/oplog"
	"github.com/guilbep/git-board/core/people"
)

const key = "example.com/p"

// personOf is the person owning the forge user's account, on the fake's
// server.
func personOf(t *testing.T, s *gitstore.Store, userID string) *people.Person {
	t.Helper()
	d, err := people.LoadDirectory(s)
	if err != nil {
		t.Fatal(err)
	}
	p, ok := d.ForAccount("example.com", userID)
	if !ok {
		t.Fatalf("no person for user %s", userID)
	}
	return p
}

func persons(t *testing.T, s *gitstore.Store) []string {
	t.Helper()
	ids, err := people.List(s)
	if err != nil {
		t.Fatal(err)
	}
	return ids
}

// stamp checks an op's author and origin, and that its time is the pull's,
// between t0 and t1.
func stamp(t *testing.T, e oplog.Entry, author string, origin *oplog.Origin, t0, t1 int64) {
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

func open(t *testing.T, dir string) *gitstore.Store {
	t.Helper()
	s, err := gitstore.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func newStore(t *testing.T) *gitstore.Store { return open(t, newRepo(t)) }

func ann() forge.Account { return forge.Account{ID: "7", Login: "ann"} }

func sample() forge.Issue {
	return forge.Issue{ID: "100", Number: "1", URL: "https://example.com/p/1", Author: ann(), Created: 1000,
		Title: "T", Body: "B", Status: "closed", Labels: []string{"bug", "assignee:bob"},
		Comments: []forge.Comment{{ID: "c1", Body: "hi", Author: forge.Account{ID: "8", Login: "bob"}, Time: 1100}}}
}

func pull(t *testing.T, s *gitstore.Store, f forge.Forge) Report {
	t.Helper()
	r, err := Pull(context.Background(), s, f)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func only(t *testing.T, s *gitstore.Store) *issue.Issue {
	t.Helper()
	ids, err := issue.List(s)
	if err != nil || len(ids) != 1 {
		t.Fatalf("issues = %v, %v", ids, err)
	}
	is, err := issue.Load(s, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	return is
}

func refs(t *testing.T, s *gitstore.Store) map[string]gitstore.Hash {
	t.Helper()
	m, err := s.Refs("refs/board/")
	if err != nil {
		t.Fatal(err)
	}
	return maps.Clone(m)
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
	a, b := personOf(t, s, "7").ID, personOf(t, s, "8").ID
	is := only(t, s)
	issueOrigin := &oplog.Origin{Forge: key, Event: "issue:100"}
	if is.Title != "T" || is.Body != "B" || is.Status != issue.Closed || !reflect.DeepEqual(is.Labels, []string{"bug"}) ||
		is.Author != a || is.Created != 1000 || !reflect.DeepEqual(is.Origin, issueOrigin) {
		t.Fatalf("issue = %+v", is)
	}
	if len(is.Comments) != 1 || is.Comments[0].Body != "hi" || is.Comments[0].Author != b || is.Comments[0].Time != 1100 ||
		!reflect.DeepEqual(is.Comments[0].Origin, &oplog.Origin{Forge: key, Event: "comment:c1"}) {
		t.Fatalf("comments = %+v", is.Comments)
	}
	var kinds []string
	for _, e := range mustLog(t, s, is.ID) {
		kinds = append(kinds, e.Kind)
		if e.Kind == "set-status" || e.Kind == "change-labels" {
			stamp(t, e, a, issueOrigin, t0, t1)
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
	for _, e := range st.log.Entries {
		stamp(t, e, a, nil, t0, t1)
		if e.Kind == "link" {
			var d linkData
			if err := json.Unmarshal(e.Data, &d); err != nil || d != (linkData{is.ID, "100", "1", "https://example.com/p/1"}) {
				t.Fatalf("link = %+v, %v", d, err)
			}
			got = append(got, "link")
			continue
		}
		var d seenData
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
		p := personOf(t, s, user)
		if want := []people.Account{{Server: "example.com", ID: user, Login: login}}; p.Name != login || p.Email != "" || !reflect.DeepEqual(p.Accounts, want) {
			t.Fatalf("person = %+v", p)
		}
		l, err := oplog.Load(s, "person", p.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range l.Entries {
			if e.Author != "" || e.ID[:40] != p.ID {
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
	ids, _ := issue.List(s)
	a := personOf(t, s, "7").ID
	for _, id := range ids {
		if is, err := issue.Load(s, id); err != nil || is.Author != a {
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
	before := personOf(t, s, "7")
	f.Edit("1", func(i *forge.Issue) { i.Author.Login = "ann2" })
	pull(t, s, f)
	p := personOf(t, s, "7")
	if want := []people.Account{{Server: "example.com", ID: "7", Login: "ann2"}}; p.ID != before.ID || p.Name != "ann" || !reflect.DeepEqual(p.Accounts, want) {
		t.Fatalf("person = %+v, was %+v", p, before)
	}
	if n := len(persons(t, s)); n != 2 {
		t.Fatalf("persons = %d, want 2", n)
	}
	l, _ := oplog.Load(s, "person", p.ID)
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
	if _, err := oplog.Create(t.Context(), s, "person", "", []oplog.Op{{Kind: "bogus", Time: 1}}); err != nil {
		t.Fatal(err)
	}
	f.Add(sample())
	before := refs(t, s)
	if _, err := Pull(t.Context(), s, f); err == nil {
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
	if err := st.write(t.Context(), s, key, []oplog.Op{linkOp("", 1, "i", "100", "1", "")}); err == nil {
		t.Fatal("a write with no author landed")
	}
	if len(refs(t, s)) != 0 {
		t.Fatal("wrote")
	}
	if _, err := oplog.Create(t.Context(), s, "sync", StateID(key), []oplog.Op{seenOp("", 1, "i", "title", "T")}); err != nil {
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
	a, b := personOf(t, s, "7").ID, personOf(t, s, "8").ID
	var kinds []string
	for _, e := range mustLog(t, s, is.ID)[4:] { // after the first pull's
		kinds = append(kinds, e.Kind)
		switch e.Kind {
		case "set-title", "change-labels":
			stamp(t, e, a, &oplog.Origin{Forge: key, Event: "issue:100"}, t0, t1)
		case "edit-text":
			stamp(t, e, b, &oplog.Origin{Forge: key, Event: "comment:c1"}, t0, t1)
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
	if _, err := issue.Do(t.Context(), s, is.ID, issue.SetTitle("me", "mine")); err != nil {
		t.Fatal(err)
	}
	before := refs(t, s)
	if r := pull(t, s, f); r != (Report{Unchanged: 1}) {
		t.Fatalf("report = %+v", r)
	}
	if only(t, s).Title != "mine" || !reflect.DeepEqual(before, refs(t, s)) {
		t.Fatal("local change not kept, or something written")
	}
	// both changed: still nothing written.
	f.Edit("1", func(i *forge.Issue) { i.Title = "theirs" })
	if r := pull(t, s, f); r != (Report{Unchanged: 1}) {
		t.Fatalf("conflict report = %+v", r)
	}
	if only(t, s).Title != "mine" || !reflect.DeepEqual(before, refs(t, s)) {
		t.Fatal("conflict wrote something")
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
	var stores [2]*gitstore.Store
	var states [2]*state
	for i := range stores {
		stores[i] = open(t, dir)
		st, err := loadState(stores[i], key)
		if err != nil || st.log != nil {
			t.Fatalf("state = %+v, %v", st, err)
		}
		states[i] = st
	}
	for i, fid := range []string{"100", "200"} {
		if err := states[i].write(t.Context(), stores[i], key, []oplog.Op{linkOp("a", 1, "issue"+fid, fid, fid, "")}); err != nil {
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
	if _, err := Pull(context.Background(), s, f); err == nil {
		t.Fatal("want an error")
	}
	if len(refs(t, s)) != 0 {
		t.Fatal("wrote")
	}
}

func loadIssueLog(t *testing.T, s *gitstore.Store, id string) ([]oplog.Entry, error) {
	t.Helper()
	l, err := oplog.Load(s, "issue", id)
	if err != nil {
		t.Fatal(err)
	}
	return l.Entries, nil
}

func labels(t *testing.T, s *gitstore.Store) []string {
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
	if _, err := issue.Do(t.Context(), s, is.ID, issue.ChangeLabels("me", []string{"ui"}, nil)); err != nil {
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
	if _, err := issue.Do(t.Context(), s, is.ID, issue.ChangeLabels("me", nil, []string{"docs"})); err != nil {
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
	if _, err := issue.Do(t.Context(), s, is.ID, issue.ChangeLabels("me", []string{"docs"}, []string{"ui"})); err != nil {
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
	if _, err := issue.Do(t.Context(), s, is.ID, issue.ChangeLabels("me", []string{"assignee:x", "Assignee:y"}, nil)); err != nil {
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
	if _, err := issue.Do(t.Context(), s, is.ID, issue.SetTitle("me", "same")); err != nil {
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

func mustLog(t *testing.T, s *gitstore.Store, id string) []oplog.Entry {
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
	if err := st.write(t.Context(), s, key, []oplog.Op{linkOp("a", 1, "notanid", "100", "1", "")}); err != nil {
		t.Fatal(err)
	}
	f.Add(sample())
	before := refs(t, s)
	r, err := Pull(t.Context(), s, f)
	if err != nil || r != (Report{Unchanged: 1}) {
		t.Fatalf("report = %+v, %v; want one unchanged", r, err)
	}
	if !reflect.DeepEqual(before, refs(t, s)) {
		t.Fatal("refs moved")
	}
}
