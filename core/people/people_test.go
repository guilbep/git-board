package people

import (
	"errors"
	"os/exec"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/guilbep/git-board/core/gitstore"
	"github.com/guilbep/git-board/core/oplog"
)

func newStore(t *testing.T) *gitstore.Store {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.Command("git", "init", "-q")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	s, err := gitstore.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func mk(t *testing.T, s *gitstore.Store, ops ...oplog.Op) *Person {
	t.Helper()
	p, err := Create(t.Context(), s, ops...)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func do(t *testing.T, s *gitstore.Store, id string, ops ...oplog.Op) *Person {
	t.Helper()
	p, err := Do(t.Context(), s, id, ops...)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPersonEveryOp(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	p := mk(t, s, NewCreate("", "ann", "a@x"))
	id := p.ID
	if want := (&Person{ID: id, Name: "ann", Email: "a@x", Accounts: []Account{}}); !reflect.DeepEqual(p, want) {
		t.Fatalf("create = %+v", p)
	}
	p = do(t, s, id, SetName(id, "Ann"))
	if p.Name != "Ann" || p.Email != "a@x" {
		t.Fatalf("set-name = %+v", p)
	}
	p = do(t, s, id, SetEmail(id, "b@x"))
	if p.Name != "Ann" || p.Email != "b@x" {
		t.Fatalf("set-email = %+v", p)
	}
	p = do(t, s, id, SetEmail(id, ""))
	if p.Name != "Ann" || p.Email != "" {
		t.Fatalf("set-email = %+v", p)
	}
	p = do(t, s, id, AddAccount(id, Account{"gitlab.com", "9", "z"}), AddAccount(id, Account{"github.com", "4", "ann"}))
	want := []Account{{"github.com", "4", "ann"}, {"gitlab.com", "9", "z"}}
	if !reflect.DeepEqual(p.Accounts, want) {
		t.Fatalf("add-account = %+v", p.Accounts)
	}
	p = do(t, s, id, RemoveAccount(id, "gitlab.com", "9"))
	if !reflect.DeepEqual(p.Accounts, want[:1]) {
		t.Fatalf("remove-account = %+v", p.Accounts)
	}
	g, err := Load(s, id)
	if err != nil || !reflect.DeepEqual(g, p) {
		t.Fatalf("load = %+v, %v", g, err)
	}
	if ids, err := List(s); err != nil || !slices.Equal(ids, []string{id}) {
		t.Fatalf("list = %v, %v", ids, err)
	}
}

func TestAddAccountRenamesLogin(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	p := mk(t, s, NewCreate("", "ann", ""))
	do(t, s, p.ID, AddAccount(p.ID, Account{"github.com", "4", "old"}))
	p = do(t, s, p.ID, AddAccount(p.ID, Account{"github.com", "4", "new"}))
	if !reflect.DeepEqual(p.Accounts, []Account{{"github.com", "4", "new"}}) {
		t.Fatalf("accounts = %+v", p.Accounts)
	}
}

func TestRemoveAbsentAccountIgnored(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	p := mk(t, s, NewCreate("", "ann", ""))
	p = do(t, s, p.ID, RemoveAccount(p.ID, "github.com", "4"))
	if len(p.Accounts) != 0 {
		t.Fatalf("accounts = %+v", p.Accounts)
	}
}

func TestPersonBadValues(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	for _, op := range []oplog.Op{NewCreate("", "", "")} {
		if _, err := Create(t.Context(), s, op); !errors.Is(err, ErrBadValue) {
			t.Fatalf("create err = %v", err)
		}
	}
	if ids, _ := List(s); len(ids) != 0 {
		t.Fatalf("written: %v", ids)
	}
	p := mk(t, s, NewCreate("", "ann", ""))
	for _, op := range []oplog.Op{
		SetName(p.ID, ""),
		AddAccount(p.ID, Account{"", "1", "l"}),
		AddAccount(p.ID, Account{"s", "", "l"}),
		AddAccount(p.ID, Account{"s", "1", ""}),
		RemoveAccount(p.ID, "", "1"),
		RemoveAccount(p.ID, "s", ""),
	} {
		if _, err := Do(t.Context(), s, p.ID, op); !errors.Is(err, ErrBadValue) {
			t.Fatalf("%s err = %v", op.Kind, err)
		}
	}
	l, err := oplog.Load(s, "person", p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Entries) != 1 {
		t.Fatalf("written: %d ops", len(l.Entries))
	}
}

func TestPersonUnknownOp(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	p := mk(t, s, NewCreate("", "ann", ""))
	_, err := Do(t.Context(), s, p.ID, oplog.Op{Kind: "frobnicate", Author: p.ID, Time: 1})
	if !errors.Is(err, ErrUnknownOp) || !strings.Contains(err.Error(), "frobnicate") {
		t.Fatalf("err = %v", err)
	}
}

func TestPersonAuthors(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	// A pulled person is one write: create and add-account, both authorless.
	p := mk(t, s, NewCreate("", "ann", ""), AddAccount("", Account{"github.com", "4", "ann"}))
	if len(p.Accounts) != 1 {
		t.Fatalf("person = %+v", p)
	}
	// A later op needs an author.
	if _, err := Do(t.Context(), s, p.ID, SetName("", "Ann")); err == nil {
		t.Fatal("authorless later op accepted")
	}
	if l, err := oplog.Load(s, "person", p.ID); err != nil || len(l.Entries) != 2 {
		t.Fatalf("log = %v, %v", l, err)
	}
	p = do(t, s, p.ID, AddAccount(p.ID, Account{"github.com", "4", "renamed"}))
	if p.Accounts[0].Login != "renamed" {
		t.Fatalf("person = %+v", p)
	}
	// A reader sees the same rule on a stored log.
	l, err := oplog.Load(s, "person", p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(l); err != nil {
		t.Fatal(err)
	}
}

func TestMe(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	if me, err := Me(t.Context(), s); err != nil || me != "" {
		t.Fatalf("me = %q, %v", me, err)
	}
	if err := SetMe(t.Context(), s, "nobody"); err != nil {
		t.Fatal(err)
	}
	if me, err := Me(t.Context(), s); err != nil || me != "nobody" {
		t.Fatalf("me = %q, %v", me, err)
	}
}

func TestSecondCreateAndBrokenPerson(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	p := mk(t, s, NewCreate("", "ann", ""))
	if _, err := Do(t.Context(), s, p.ID, NewCreate(p.ID, "x", "")); err == nil {
		t.Fatal("second create accepted")
	}
	if _, err := Do(t.Context(), s, p.ID, SetName(p.ID, "ok")); err != nil {
		t.Fatal(err)
	}
	l, err := oplog.Create(t.Context(), s, "person", "", []oplog.Op{{Kind: "frobnicate", Time: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Load(s, l.ID); !errors.Is(err, ErrUnknownOp) {
		t.Fatalf("load err = %v", err)
	}
	if _, err := LoadDirectory(s); !errors.Is(err, ErrUnknownOp) {
		t.Fatalf("directory err = %v", err)
	}
}
