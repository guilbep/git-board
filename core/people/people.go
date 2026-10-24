// Package people folds a person's operations into its state.
package people

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/guilbep/git-board/core/oplog"
)

// Account is a person's account on a forge server.
type Account struct{ Server, ID, Login string }

// Person is the state its operations add up to.
type Person struct {
	ID, Name, Email string
	Accounts        []Account // sorted by (Server, ID)
}

var (
	ErrUnknownOp = errors.New("people: unknown operation")
	ErrBadValue  = errors.New("people: bad value")
)

var kinds = []string{"create", "set-name", "set-email", "add-account", "remove-account"}

// data is the union of the fields of every operation's data.
type data struct {
	Name   string `json:"name"`
	Email  string `json:"email"`
	Server string `json:"server"`
	ID     string `json:"id"`
	Login  string `json:"login"`
}

// Apply folds the log's operations, in order, into a person. An operation
// is in the first pack when the pack hash in its id equals the log's id;
// only there may its author be empty.
func Apply(l *oplog.Log) (*Person, error) {
	var p *Person
	for _, e := range l.Entries {
		if !slices.Contains(kinds, e.Kind) {
			return nil, fmt.Errorf("%w: %q, operation %s", ErrUnknownOp, e.Kind, e.ID)
		}
		var d data
		if len(e.Data) > 0 {
			if err := json.Unmarshal(e.Data, &d); err != nil {
				return nil, fmt.Errorf("people: operation %s: %w", e.ID, err)
			}
		}
		if p == nil && e.Kind != "create" {
			return nil, fmt.Errorf("people: the first operation is %q, want create", e.Kind)
		}
		if e.Author == "" && !strings.HasPrefix(e.ID, l.ID+":") {
			return nil, fmt.Errorf("people: operation %s has no author", e.ID)
		}
		bad := func(field string) error {
			return fmt.Errorf("%w: empty %s, operation %s", ErrBadValue, field, e.ID)
		}
		switch e.Kind {
		case "create", "set-name":
			if d.Name == "" {
				return nil, bad("name")
			}
		case "add-account", "remove-account":
			if d.Server == "" {
				return nil, bad("server")
			}
			if d.ID == "" {
				return nil, bad("id")
			}
			if e.Kind == "add-account" && d.Login == "" {
				return nil, bad("login")
			}
		}
		switch e.Kind {
		case "create":
			if p != nil {
				return nil, fmt.Errorf("people: second create, operation %s", e.ID)
			}
			p = &Person{ID: l.ID, Name: d.Name, Email: d.Email, Accounts: []Account{}}
		case "set-name":
			p.Name = d.Name
		case "set-email":
			p.Email = d.Email
		case "add-account":
			a := Account{d.Server, d.ID, d.Login}
			if i := slices.IndexFunc(p.Accounts, func(x Account) bool { return x.Server == a.Server && x.ID == a.ID }); i >= 0 {
				p.Accounts[i] = a
			} else {
				p.Accounts = append(p.Accounts, a)
			}
		case "remove-account":
			p.Accounts = slices.DeleteFunc(p.Accounts, func(x Account) bool { return x.Server == d.Server && x.ID == d.ID })
		}
	}
	if p == nil {
		return nil, errors.New("people: no operation")
	}
	slices.SortFunc(p.Accounts, func(a, b Account) int {
		return strings.Compare(a.Server+"\x00"+a.ID, b.Server+"\x00"+b.ID)
	})
	return p, nil
}
