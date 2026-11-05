package issuesync

import (
	"reflect"
	"testing"
)

// The marker carries the protocol's keys in their order, by short name; a
// forge edit restores what its keys say, or, without the marker, clears
// the thread's and the spec's.
func TestMeanings(t *testing.T) {
	t.Parallel()
	meta := map[string]string{"thread:anchor": "body@v1/expected#3", "cycle:step": "1", "thread:kind": "a b", "other": "x"}
	if got := Carry(meta); got != "step=1 anchor=body@v1/expected#3" {
		t.Errorf("Carry = %q", got)
	}
	if got := Restore("step=2 anchor=body@v1/expected#3 resolved=true", true, meta); !reflect.DeepEqual(got,
		map[string]string{"cycle:step": "2", "thread:resolved": "true"}) {
		t.Errorf("Restore marked = %v", got)
	}
	if got := Restore("", false, meta); !reflect.DeepEqual(got, map[string]string{"thread:anchor": "", "thread:kind": ""}) {
		t.Errorf("Restore unmarked = %v", got)
	}
	if ReplyTo(map[string]string{"thread:reply-to": "abc:1"}) != "abc:1" {
		t.Error("ReplyTo")
	}
}
