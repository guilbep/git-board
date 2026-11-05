// Package people is the person kind: who makes operations, their name,
// email and forge accounts (docs/spec/data-model.md, People), and how a
// person shows on a surface.
package people

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/guilbep/git-board/next/kernel"
)

// Account is one user on one forge server, named by (Server, ID).
type Account struct {
	Server string `json:"server"`
	ID     string `json:"id"`
	Login  string `json:"login"`
}

// Person is a person's state, and its row: small, every list reads all.
type Person struct {
	ID       kernel.ID `json:"id"`
	Name     string    `json:"name"`
	Email    string    `json:"email"`
	Accounts []Account `json:"accounts"` // sorted by (Server, ID), unique by both
	made     bool      // a create was applied
}

// Kind is the person kind.
var Kind = kernel.Define(kernel.Spec[Person, Person]{
	Name:  "person",
	Rev:   1,
	New:   func(id kernel.ID) Person { return Person{ID: id} },
	Apply: apply,
	Index: func(p *Person) kernel.Doc[Person] {
		d := kernel.Doc[Person]{Row: *p}
		for _, a := range p.Accounts {
			d.Terms = append(d.Terms, "account:"+a.Server+"/"+a.ID)
		}
		return d
	},
})

var (
	ErrUnknownOp = errors.New("people: unknown operation")
	ErrBadValue  = errors.New("people: bad value")
)

var ops = []string{"create", "set-name", "set-email", "add-account", "remove-account"}

// data is the union of every operation's data.
type data struct {
	Name   string `json:"name"`
	Email  string `json:"email"`
	Server string `json:"server"`
	ID     string `json:"id"`
	Login  string `json:"login"`
}

// apply is one operation, as data-model.md › Person state says. Only an
// operation of the first pack, whose hash is the person's id, may have no
// author.
func apply(p *Person, e kernel.Entry) error {
	if !slices.Contains(ops, e.Kind) {
		return fmt.Errorf("%w: %q, operation %s", ErrUnknownOp, e.Kind, e.ID)
	}
	var d data
	if len(e.Data) > 0 {
		if err := json.Unmarshal(e.Data, &d); err != nil {
			return fmt.Errorf("people: operation %s: %w", e.ID, err)
		}
	}
	if !p.made && e.Kind != "create" {
		return fmt.Errorf("people: the first operation is %q, want create", e.Kind)
	}
	if e.Author == "" && e.ID.Pack() != p.ID {
		return fmt.Errorf("people: operation %s has no author", e.ID)
	}
	bad := func(field string) error { return fmt.Errorf("%w: empty %s, operation %s", ErrBadValue, field, e.ID) }
	switch e.Kind {
	case "create", "set-name":
		if d.Name == "" {
			return bad("name")
		}
	case "add-account", "remove-account":
		if d.Server == "" {
			return bad("server")
		}
		if d.ID == "" {
			return bad("id")
		}
		if e.Kind == "add-account" && d.Login == "" {
			return bad("login")
		}
	}
	same := func(a Account) bool { return a.Server == d.Server && a.ID == d.ID }
	switch e.Kind {
	case "create":
		if p.made {
			return fmt.Errorf("people: second create, operation %s", e.ID)
		}
		p.made, p.Name, p.Email, p.Accounts = true, d.Name, d.Email, []Account{}
	case "set-name":
		p.Name = d.Name
	case "set-email":
		p.Email = d.Email
	case "add-account":
		a := Account{d.Server, d.ID, d.Login}
		if i := slices.IndexFunc(p.Accounts, same); i >= 0 {
			p.Accounts[i] = a
			break
		}
		i, _ := slices.BinarySearchFunc(p.Accounts, a, func(x, y Account) int {
			return cmp.Or(strings.Compare(x.Server, y.Server), strings.Compare(x.ID, y.ID))
		})
		p.Accounts = slices.Insert(p.Accounts, i, a)
	case "remove-account":
		p.Accounts = slices.DeleteFunc(p.Accounts, same)
	}
	return nil
}

// Logins are a person's logins on forges, each once, in their accounts'
// order.
func (p Person) Logins() []string {
	var res []string
	for _, a := range p.Accounts {
		if !slices.Contains(res, a.Login) {
			res = append(res, a.Login)
		}
	}
	return res
}

// Who is a person as a surface shows them: their first forge login, and
// their picture. The email stays out.
type Who struct {
	ID        kernel.ID `json:"id"`
	Name      string    `json:"name"`
	Login     string    `json:"login,omitempty"`
	AvatarURL string    `json:"avatar_url,omitempty"`
}

// Avatars is where pictures come from: git config board.avatars.
type Avatars string

const (
	Forge    Avatars = "forge"    // github.com accounts (the default)
	Gravatar Avatars = "gravatar" // github.com accounts, then Gravatar
	None     Avatars = "none"     // initials only
)

// ParseAvatars reads board.avatars: forge for anything else.
func ParseAvatars(s string) Avatars {
	switch m := Avatars(strings.ToLower(strings.TrimSpace(s))); m {
	case Gravatar, None:
		return m
	}
	return Forge
}

// avatarSize is the pictures' size, in pixels.
const avatarSize = 88

// Avatar is the person's picture's URL as m allows, "" for none: their
// GitHub account's (its numeric id, never a login), else with Gravatar
// their email's.
func (p Person) Avatar(m Avatars) string {
	if m == None {
		return ""
	}
	for _, a := range p.Accounts {
		if a.Server != "github.com" {
			continue
		}
		if _, err := strconv.ParseUint(a.ID, 10, 64); err != nil {
			continue
		}
		return "https://avatars.githubusercontent.com/u/" + a.ID + "?s=" + strconv.Itoa(avatarSize)
	}
	email := strings.ToLower(strings.TrimSpace(p.Email))
	if m != Gravatar || email == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(email))
	q := url.Values{"s": {strconv.Itoa(avatarSize)}, "d": {"404"}}
	return "https://gravatar.com/avatar/" + hex.EncodeToString(sum[:]) + "?" + q.Encode()
}

// Directory is every person of a board, by id, as a surface names them.
type Directory struct {
	ByID    map[kernel.ID]Person
	Avatars Avatars
	logins  map[kernel.ID][]string // each one's Logins, read once
	names   map[kernel.ID][]string // its name, then its logins
}

// Logins are a person's logins, as Person.Logins, shared: read them,
// never write them.
func (d Directory) Logins(id kernel.ID) []string { return d.logins[id] }

// Names are a person's name then logins, shared: what author: matches.
func (d Directory) Names(id kernel.ID) []string { return d.names[id] }

// Read is the board's people, fresh.
func Read(ctx context.Context, b *kernel.Board, m Avatars) (Directory, error) {
	ls, err := Kind.List(ctx, b, kernel.Query{})
	if err != nil {
		return Directory{}, err
	}
	d := Directory{ByID: make(map[kernel.ID]Person, len(ls)), Avatars: m,
		logins: make(map[kernel.ID][]string, len(ls)), names: make(map[kernel.ID][]string, len(ls))}
	for _, l := range ls {
		d.ByID[l.ID] = l.Row
		d.logins[l.ID] = l.Row.Logins()
		d.names[l.ID] = append([]string{l.Row.Name}, d.logins[l.ID]...)
	}
	return d, nil
}

// Who is id as a surface shows them; one not known (not fetched yet)
// shows by its id's first 7 digits.
func (d Directory) Who(id kernel.ID) Who {
	p, ok := d.ByID[id]
	if !ok {
		return Who{ID: id, Name: string(id[:min(7, len(id))])}
	}
	w := Who{ID: p.ID, Name: p.Name, AvatarURL: p.Avatar(d.Avatars)}
	if ls := p.Logins(); len(ls) > 0 {
		w.Login = ls[0]
	}
	return w
}
