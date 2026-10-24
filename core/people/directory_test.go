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

func TestDirectoryPut(t *testing.T) {
	t.Parallel()
	d, err := LoadDirectory(newStore(t))
	if err != nil {
		t.Fatal(err)
	}
	owner := func() *Person { p, _ := d.ForAccount("github.com", "4"); return p }
	acc := Account{"github.com", "4", "ann"}
	b := &Person{ID: "b", Accounts: []Account{acc}}
	d.Put(b)
	d.Put(&Person{ID: "c", Accounts: []Account{acc}}) // a higher id doesn't own it
	if owner() != b {
		t.Fatalf("owner = %+v, want b", owner())
	}
	renamed := &Person{ID: "b", Accounts: []Account{{"github.com", "4", "ann2"}}}
	d.Put(renamed)
	if owner() != renamed {
		t.Fatalf("owner = %+v, want b renamed", owner())
	}
	a := &Person{ID: "a", Accounts: []Account{acc}}
	d.Put(a)
	if owner() != a {
		t.Fatalf("owner = %+v, want a", owner())
	}
}
