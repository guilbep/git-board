package people

import (
	"slices"
	"testing"
)

func TestAccountOwnerIsFirstID(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	var ids []string
	// Random ids: go on until the lowest is neither the first nor the last made.
	for len(ids) < 3 || slices.Min(ids) == ids[0] || slices.Min(ids) == ids[len(ids)-1] {
		p := mk(t, s, NewCreate("", "ann", ""), AddAccount("", Account{"github.com", "4", "ann"}))
		ids = append(ids, p.ID)
	}
	d, err := LoadDirectory(s)
	if err != nil {
		t.Fatal(err)
	}
	p, ok := d.ForAccount("github.com", "4")
	if !ok || p.ID != slices.Min(ids) {
		t.Fatalf("owner = %v, %v, want %s", p, ok, slices.Min(ids))
	}
	if _, ok := d.ForAccount("github.com", "5"); ok {
		t.Fatal("unknown account found")
	}
}
