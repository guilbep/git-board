package issues

import (
	"errors"
	"testing"
)

// The table's columns sort: by who opened it, by id, open or closed. A
// key that is none of the sort's is a usage error, never an empty list.
func TestSortTableColumns(t *testing.T) {
	t.Parallel()
	s := &Schema{}
	list := func() []Issue {
		return []Issue{{ID: "b", Author: "zoé", Status: "open"}, {ID: "c", Author: "Ada", Status: "closed"}, {ID: "a", Author: "bob", Status: "open"}}
	}
	ids := func(is []Issue) string {
		out := ""
		for _, i := range is {
			out += i.ID
		}
		return out
	}
	for spec, want := range map[string]string{"author": "cab", "author desc": "bac", "id": "abc", "is": "cba"} {
		is := list()
		if err := s.Sort(is, spec); err != nil || ids(is) != want {
			t.Errorf("Sort %q = %s, %v; want %s", spec, ids(is), err, want)
		}
	}
	if err := s.Sort(list(), "links"); !errors.Is(err, ErrUsage) {
		t.Errorf("Sort links = %v, want a usage error", err)
	}
}
