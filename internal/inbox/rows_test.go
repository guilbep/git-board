package inbox_test

import (
	"slices"
	"testing"
	"time"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio"
	"github.com/guilbep/git-board/internal/inbox"
)

var (
	now   = time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
	since = now.Add(-inbox.Window)
	old   = since.Add(-48 * time.Hour) // before the window
	me    = inbox.Me{ID: "me1", Login: "guilbep"}
	meP   = gitbugio.Person{ID: "me1", Name: "Guilbert Pierre", Login: "guilbep"}
	me2   = gitbugio.Person{ID: "me2", Name: "Guilbert (GitHub)", Login: "GuilBep"} // a second identity of yours
	ann   = gitbugio.Person{ID: "ann", Name: "Ann", Login: "ann"}
	cfg   = &config.Config{}
)

func ev(kind, id string, who gitbugio.Person, at time.Time) gitbugio.Event {
	return gitbugio.Event{Kind: kind, ID: id, Author: who, At: at}
}

func comment(id string, who gitbugio.Person, at time.Time, body string) gitbugio.Event {
	e := ev("comment", id, who, at)
	e.Body = body
	return e
}

func labels(id string, who gitbugio.Person, at time.Time, added, removed []string) gitbugio.Event {
	e := ev("labels", id, who, at)
	e.Added, e.Removed = added, removed
	return e
}

func status(id string, who gitbugio.Person, at time.Time, s string) gitbugio.Event {
	e := ev("status", id, who, at)
	e.Status = s
	return e
}

// thread is an issue by author, created before the window, with labels
// and events after its creation.
func thread(id string, author gitbugio.Person, lbls []string, events ...gitbugio.Event) gitbugio.Thread {
	created := ev("created", id+"-0", author, old)
	return gitbugio.Thread{
		Issue:  gitbugio.Issue{ID: id, Title: id, Status: "open", Labels: lbls},
		Author: author,
		Events: append([]gitbugio.Event{created}, events...),
	}
}

func TestRows(t *testing.T) {
	for _, tc := range []struct {
		name    string
		th      gitbugio.Thread
		st      inbox.State
		views   []string
		news    []string // nil: no row
		reasons []string
	}{
		{name: "someone's comment on your assigned issue shows, yours doesn't",
			th:   thread("a", ann, []string{"assignee:guilbep"}, comment("a1", ann, now.Add(-time.Hour), "done?"), comment("a2", meP, now.Add(-30*time.Minute), "yes")),
			news: []string{"1 comment"}, reasons: []string{"assignee", "commented"}},
		{name: "a late arrival shows after you opened the issue",
			th:   thread("b", ann, []string{"assignee:guilbep"}, comment("b1", ann, now.Add(-26*time.Hour), "written offline yesterday")),
			st:   inbox.State{Seen: map[string][]string{"b": {"b-0"}}},
			news: []string{"1 comment"}, reasons: []string{"assignee"}},
		{name: "seen events don't show",
			th: thread("c", ann, []string{"assignee:guilbep"}, comment("c1", ann, now.Add(-time.Hour), "x")),
			st: inbox.State{Seen: map[string][]string{"c": {"c-0", "c1"}}}},
		{name: "added to a person field shows",
			th:   thread("d", ann, []string{"reviewer:guilbep"}, labels("d1", ann, now.Add(-2*time.Hour), []string{"reviewer:guilbep"}, nil)),
			news: []string{"added you as reviewer"}, reasons: []string{"reviewer"}},
		{name: "removed from a person field shows though you no longer follow",
			th:   thread("e", ann, nil, labels("e1", ann, now.Add(-2*time.Hour), nil, []string{"assignee:guilbep"})),
			news: []string{"removed you as assignee"}},
		{name: "a mention shows, and makes you follow",
			th:   thread("f", ann, nil, comment("f1", ann, now.Add(-time.Hour), "ping @guilbep")),
			news: []string{"1 comment", "mentioned you"}, reasons: []string{"mentioned"}},
		{name: "Unfollow hides activity",
			th: thread("g", ann, []string{"assignee:guilbep"}, comment("g1", ann, now.Add(-time.Hour), "x")),
			st: inbox.State{Unfollow: []string{"g"}}},
		{name: "a mention goes through Unfollow",
			th:   thread("h", ann, []string{"assignee:guilbep"}, comment("h1", ann, now.Add(-time.Hour), "@guilbep look")),
			st:   inbox.State{Unfollow: []string{"h"}},
			news: []string{"mentioned you"}, reasons: []string{"assignee", "mentioned"}},
		{name: "a sprint change is hidden with fields off",
			th: thread("i", ann, []string{"assignee:guilbep"}, labels("i1", ann, now.Add(-time.Hour), []string{"sprint:s43"}, []string{"sprint:s42"}))},
		{name: "a sprint change shows with fields on",
			th:   thread("j", ann, []string{"assignee:guilbep"}, labels("j1", ann, now.Add(-time.Hour), []string{"sprint:s43"}, []string{"sprint:s42"})),
			st:   inbox.State{Fields: true},
			news: []string{"1 field change"}, reasons: []string{"assignee"}},
		{name: "a title edit never shows",
			th: thread("k", ann, []string{"assignee:guilbep"}, ev("title", "k1", ann, now.Add(-time.Hour)))},
		{name: "Follow brings an issue in",
			th:   thread("l", ann, nil, comment("l1", ann, now.Add(-time.Hour), "x")),
			st:   inbox.State{Follow: []string{"l"}},
			news: []string{"1 comment"}, reasons: []string{"followed"}},
		{name: "a closed issue shows for a followed view",
			th:    thread("m", ann, nil, status("m1", ann, now.Add(-time.Hour), "closed")),
			views: []string{"backend"},
			news:  []string{"closed"}, reasons: []string{"view: backend"}},
		{name: "outside the window is ignored",
			th: thread("n", ann, []string{"assignee:guilbep"}, comment("n1", ann, since.Add(-time.Hour), "x"))},
		{name: "a second identity carrying your login is you",
			th: thread("o", ann, []string{"assignee:guilbep"}, comment("o1", me2, now.Add(-time.Hour), "from GitHub"))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows := inbox.Rows(me, tc.st, []gitbugio.Thread{tc.th}, map[string][]string{tc.th.ID: tc.views}, cfg, since)
			if tc.news == nil {
				if len(rows) != 0 {
					t.Fatalf("got %+v, want no row", rows)
				}
				return
			}
			if len(rows) != 1 {
				t.Fatalf("got %d rows, want 1", len(rows))
			}
			if !slices.Equal(rows[0].News, tc.news) || !slices.Equal(rows[0].Reasons, tc.reasons) {
				t.Errorf("news %v reasons %v, want %v %v", rows[0].News, rows[0].Reasons, tc.news, tc.reasons)
			}
		})
	}
}

func TestRowsOrderAndDetails(t *testing.T) {
	older := thread("p", ann, []string{"assignee:guilbep"}, comment("p1", ann, now.Add(-3*time.Hour), "x"), comment("p2", ann, now.Add(-2*time.Hour), "y"))
	newer := thread("q", ann, []string{"assignee:guilbep"}, comment("q1", ann, now.Add(-time.Hour), "z"))
	rows := inbox.Rows(me, inbox.State{}, []gitbugio.Thread{older, newer}, nil, cfg, since)
	if len(rows) != 2 || rows[0].Issue.ID != "q" || rows[1].Issue.ID != "p" {
		t.Fatalf("order: %+v, want q then p", rows)
	}
	p := rows[1]
	if p.First != "p1" || !p.Latest.Equal(now.Add(-2*time.Hour)) || len(p.Who) != 1 || p.Who[0].ID != "ann" || !slices.Equal(p.News, []string{"2 comments"}) {
		t.Errorf("p: first %s latest %v who %v news %v", p.First, p.Latest, p.Who, p.News)
	}
}

func TestFollows(t *testing.T) {
	th := thread("o", ann, []string{"assignee:guilbep"}, comment("o1", me2, now.Add(-time.Hour), "from GitHub"))
	following, reasons, set := inbox.Follows(me, inbox.State{}, th, nil, cfg)
	if !following || !slices.Equal(reasons, []string{"assignee", "commented"}) || set != nil {
		t.Errorf("Follows = %v %v %v, want true [assignee commented] nil", following, reasons, set)
	}
	following, _, set = inbox.Follows(me, inbox.State{Unfollow: []string{"o"}}, th, nil, cfg)
	if following || set == nil || *set {
		t.Errorf("unfollowed: %v %v", following, set)
	}
}
