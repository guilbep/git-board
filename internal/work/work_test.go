package work

import (
	"slices"
	"testing"
	"time"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/cycle"
	"github.com/guilbep/git-board/internal/inbox"
	"github.com/guilbep/git-board/internal/store"
)

var (
	alice = inbox.Me{ID: "ida", Login: "alice"}
	bob   = store.Person{ID: "idb", Login: "bob"}
	carol = store.Person{ID: "idc", Login: "carol"}
	t0    = time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
)

func day(n int) time.Time { return t0.Add(time.Duration(n) * 24 * time.Hour) }

func comment(by store.Person, n int, body string, meta map[string]string) store.Event {
	return store.Event{Kind: "comment", ID: "c" + body[:1], Author: by, At: day(n), Body: body, Meta: meta}
}

func thread(id string, author store.Person, labels []string, events ...store.Event) store.Thread {
	th := store.Thread{Author: author, Events: append([]store.Event{{Kind: "created", Author: author, At: day(0)}}, events...)}
	th.ID, th.Labels = id, labels
	return th
}

func at(id string, since int, login string, anyone, involved bool) Row {
	return Row{Issue: store.Issue{ID: id}, Step: 2, Waits: cycle.Waits{Login: login, Anyone: anyone, What: "set the risk class"}, Since: day(since), Involved: involved}
}

func ids(rows []Row) []string {
	var out []string
	for _, r := range rows {
		out = append(out, r.Issue.ID)
	}
	return out
}

func TestGroupMineAndTeam(t *testing.T) {
	rows := []Row{
		at("mine-late", 3, "alice", false, true),
		at("mine-early", 1, "Alice", false, false),
		at("anyone", 2, "", true, true),
		at("anyone-other", 2, "", true, false),
		at("bob-on-mine", 4, "bob", false, true),
		at("bob-on-carols", 5, "bob", false, false),
	}
	done := at("done", 0, "alice", false, true)
	done.Done = true
	rows = append(rows, done)
	g := Group(rows, "alice", false)
	if !slices.Equal(ids(g.Needs), []string{"mine-early", "mine-late"}) || !slices.Equal(ids(g.Waiting), []string{"anyone", "bob-on-mine"}) {
		t.Errorf("mine: needs %v, waiting %v", ids(g.Needs), ids(g.Waiting))
	}
	g = Group(rows, "alice", true)
	if !slices.Equal(ids(g.Needs), []string{"mine-early", "anyone", "anyone-other", "mine-late"}) || !slices.Equal(ids(g.Waiting), []string{"bob-on-mine", "bob-on-carols"}) {
		t.Errorf("team: needs %v, waiting %v", ids(g.Needs), ids(g.Waiting))
	}
}

func TestGroupQuestionWins(t *testing.T) {
	r := at("q", 1, "bob", false, false)
	r.Question = &Question{ID: "c1", Body: "Which?"}
	g := Group([]Row{r}, "alice", false)
	if len(g.Needs) != 1 || len(g.Waiting) != 0 {
		t.Errorf("needs %v, waiting %v", ids(g.Needs), ids(g.Waiting))
	}
}

func TestQuestionAsk(t *testing.T) {
	th := thread("i", bob, nil, comment(bob, 1, "Tolerance is 2mm", map[string]string{"ask": "1"}), comment(bob, 2, "Sure?", nil))
	if q := QuestionOf(th, alice); q == nil || q.Body != "Tolerance is 2mm" || q.At != day(1) || q.By.Login != "bob" {
		t.Errorf("ask metadata: %+v", q)
	}
	me := store.Person{ID: "ida"}
	if q := QuestionOf(thread("i", bob, nil, comment(me, 1, "Mine", map[string]string{"ask": "1"})), alice); q != nil {
		t.Errorf("the viewer's own ask: %+v", q)
	}
}

func TestQuestionMark(t *testing.T) {
	th := thread("i", bob, nil, comment(bob, 1, "Which tolerance?", nil), comment(carol, 2, "No idea.", nil))
	if q := QuestionOf(th, alice); q == nil || q.Body != "Which tolerance?" {
		t.Errorf("newest ?: %+v", q)
	}
	if q := QuestionOf(thread("i", bob, nil, comment(bob, 1, "Done.", nil)), alice); q != nil {
		t.Errorf("no ?: %+v", q)
	}
}

func TestQuestionAnswered(t *testing.T) {
	me := store.Person{ID: "ida", Login: "alice"}
	th := thread("i", bob, nil, comment(bob, 1, "Which tolerance?", map[string]string{"ask": "1"}), comment(me, 2, "2mm.", nil))
	if q := QuestionOf(th, alice); q != nil {
		t.Errorf("answered: %+v", q)
	}
}

func TestSinceLastStateChange(t *testing.T) {
	th := thread("i", bob, nil, comment(bob, 1, "Hi", nil), store.Event{Kind: "labels", At: day(2)}, comment(carol, 3, "Yo", nil))
	if got := Since(th); got != day(2) {
		t.Errorf("Since = %v, want the labels' %v", got, day(2))
	}
	th.Events = append(th.Events, comment(carol, 4, "Signed", map[string]string{"signoff:status": "qa"}))
	if got := Since(th); got != day(4) {
		t.Errorf("Since = %v, want the sign-off's %v", got, day(4))
	}
	if got := Since(thread("i", bob, nil)); got != day(0) {
		t.Errorf("Since = %v, want the creation's", got)
	}
}

func TestInvolved(t *testing.T) {
	me := store.Person{ID: "ida", Login: "alice"}
	for _, c := range []struct {
		th   store.Thread
		want bool
	}{
		{thread("a", me, nil), true},
		{thread("b", bob, []string{"assignee:alice"}), true},
		{thread("c", bob, []string{"pm:Alice"}), true},
		{thread("d", bob, []string{"status:ready"}), false},
	} {
		if got := Involved(c.th, alice); got != c.want {
			t.Errorf("%s: Involved = %v", c.th.ID, got)
		}
	}
}

func TestRowOf(t *testing.T) {
	th := thread("i", bob, []string{"assignee:alice"}, store.Event{Kind: "labels", At: day(1)}, comment(bob, 2, "Which?", nil))
	c := cycle.Cycle{Step: 5, At: "code", Of: 7, Name: "Code", Waits: cycle.Waits{Login: "alice", What: "commit and ask for review"}, Mine: true}
	c.Evidence.Branch = "fix/1-x"
	r := RowOf(th, c, alice)
	if r.Step != 5 || !r.Involved || r.Question == nil || r.Since != day(2) || r.Branch != "fix/1-x" || r.Waits.Login != "alice" {
		t.Errorf("RowOf = %+v", r)
	}
	th.Events = th.Events[:2]
	if r := RowOf(th, c, alice); r.Question != nil || r.Since != day(1) {
		t.Errorf("without the question: %+v", r)
	}
}

// The row counts in the project's list, not in the seven.
func TestRowStepIsTheListsOrdinal(t *testing.T) {
	t.Parallel()
	in := cycle.Input{Status: "in-review", Stages: config.Stages{InReview: []string{"in-review"},
		Steps: []config.Step{config.DefaultSteps[4], config.DefaultSteps[5]}}}
	r := RowOf(thread("i", bob, nil), cycle.Compute(in), alice)
	if r.Step != 2 || r.Of != 2 || r.Name != "Review" {
		t.Errorf("row: step %d of %d, %q", r.Step, r.Of, r.Name)
	}
}
