package store_test

import (
	"testing"

	"github.com/guilbep/git-board/internal/store/storetest"
)

// Everything written in one second still comes in the order it was
// written: an edit after its comment, the comment after the labels.
func TestOpOrderIsTheLogsOrder(t *testing.T) {
	t.Parallel()
	b := storetest.New(t)
	id := storetest.NewIssue(t, b, "x")
	if _, err := b.ApplyLabelChange(id, []string{"prio:p1"}, nil); err != nil {
		t.Fatal(err)
	}
	cid, err := b.AddComment(id, "one")
	if err != nil {
		t.Fatal(err)
	}
	if err := b.EditComment(id, cid, "two", "one"); err != nil {
		t.Fatal(err)
	}
	th, err := b.Thread(id)
	if err != nil {
		t.Fatal(err)
	}
	order, err := b.OpOrder(id[:7])
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, e := range th.Events {
		ids = append(ids, e.ID)
	}
	ids = append(ids, th.Events[len(th.Events)-1].History[1].ID) // the edit
	for i := 1; i < len(ids); i++ {
		a, okA := order[ids[i-1]]
		c, okC := order[ids[i]]
		if !okA || !okC || a >= c {
			t.Errorf("order[%s] = %d, %v; order[%s] = %d, %v: want ascending", ids[i-1], a, okA, ids[i], c, okC)
		}
	}
	if _, err := b.OpOrder("0000000"); err == nil {
		t.Error("OpOrder of no issue: want an error")
	}
}
