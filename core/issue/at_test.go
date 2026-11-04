package issue

import (
	"errors"
	"reflect"
	"testing"

	"github.com/guilbep/git-board/core/oplog"
)

// timedLog is a log of ops, each at its time, with ids o0, o1…
func timedLog(ops ...oplog.Op) *oplog.Log {
	l := &oplog.Log{Kind: "issue", ID: "x", Version: "v"}
	for i, op := range ops {
		l.Entries = append(l.Entries, oplog.Entry{Op: op, ID: "o" + string(rune('0'+i)), Clock: uint64(i + 1)})
	}
	return l
}

func timedOp(op oplog.Op, t int64) oplog.Op {
	op.Time = t
	return op
}

// At folds the operations made by then, in the log's order, by their own
// times; an edit of a text made later goes with it.
func TestAtFoldsWhatWasMadeByThen(t *testing.T) {
	t.Parallel()
	l := timedLog(
		timedOp(NewCreate("ann", "First", "draft"), 100),
		timedOp(ChangeLabels("ann", []string{"bug"}, nil), 200),
		timedOp(NewComment("bob", "seen"), 300),
		timedOp(EditText("ann", "o0", "final"), 400),
		timedOp(SetTitle("ann", "Second"), 250),           // a clock behind: made before o2, applied after
		timedOp(EditText("bob", "o2", "seen twice"), 290), // its comment is later
		timedOp(SetStatus("ann", Closed), 500),
	)
	for _, tc := range []struct {
		at       int64
		title    string
		body     string
		labels   []string
		comments int
		status   Status
		updated  int64
	}{
		{100, "First", "draft", []string{}, 0, Open, 100},
		{199, "First", "draft", []string{}, 0, Open, 100},
		{260, "Second", "draft", []string{"bug"}, 0, Open, 250},
		{295, "Second", "draft", []string{"bug"}, 0, Open, 250}, // the edit at 290 waits for its comment
		{300, "Second", "draft", []string{"bug"}, 1, Open, 290},
		{450, "Second", "final", []string{"bug"}, 1, Open, 290},
		{500, "Second", "final", []string{"bug"}, 1, Closed, 500},
	} {
		at, err := At(l, tc.at)
		if err != nil {
			t.Fatalf("at %d: %v", tc.at, err)
		}
		is, err := Apply(at)
		if err != nil {
			t.Fatalf("at %d: %v", tc.at, err)
		}
		got := []any{is.Title, is.Body, is.Labels, len(is.Comments), is.Status, is.Updated}
		want := []any{tc.title, tc.body, tc.labels, tc.comments, tc.status, tc.updated}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("at %d: %v, want %v", tc.at, got, want)
		}
		if tc.comments == 1 && is.Comments[0].Body != "seen twice" {
			t.Errorf("at %d: the comment is %q, without its edit", tc.at, is.Comments[0].Body)
		}
	}
	if _, err := At(l, 99); !errors.Is(err, ErrNotYet) {
		t.Errorf("before the create: %v, want ErrNotYet", err)
	}
	if len(l.Entries) != 7 {
		t.Error("At changed the log")
	}
}
