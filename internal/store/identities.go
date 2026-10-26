package store

import (
	"cmp"
	"slices"

	"github.com/guilbep/git-board/core/people"
)

// Identities lists the persons, by name.
func (b *Backend) Identities() ([]Person, error) {
	persons, err := b.allPersons()
	if err != nil {
		return nil, err
	}
	res := []Person{}
	for id, p := range persons {
		res = append(res, personOf(id, p))
	}
	slices.SortFunc(res, func(x, y Person) int { return cmp.Or(cmp.Compare(x.Name, y.Name), cmp.Compare(x.ID, y.ID)) })
	return res, nil
}

// Adopt makes a person, by id prefix, the one git-board writes as.
func (b *Backend) Adopt(idPrefix string) error {
	id, err := b.resolve("person", idPrefix)
	if err != nil {
		return err
	}
	return people.SetMe(ctx, b.s, id)
}

// NewIdentity creates a person and adopts them. You create them, or no one
// when no one is chosen yet.
func (b *Backend) NewIdentity(name, email string) (Person, error) {
	author, _ := people.Me(ctx, b.s)
	p, err := people.Create(ctx, b.s, people.NewCreate(author, name, email))
	if err != nil {
		return Person{}, err
	}
	if err := people.SetMe(ctx, b.s, p.ID); err != nil {
		return Person{}, err
	}
	return personOf(p.ID, p), nil
}

// MyLogin is the forge login of the person git-board writes as: what @me
// means (assignee:@me), since assignee labels hold forge logins. "" when
// they have no account.
// ponytail: their first account's login; the posting forge's when
// sync is wired (M5's second phase)
func (b *Backend) MyLogin() string {
	me, ok, err := b.Me()
	if err != nil || !ok {
		return ""
	}
	return me.Login
}

// Logins are what each person is assigned as: their forge login, else
// their name.
func (b *Backend) Logins() ([]string, error) {
	all, err := b.Identities()
	if err != nil {
		return nil, err
	}
	var res []string
	for _, p := range all {
		res = append(res, cmp.Or(p.Login, p.Name))
	}
	return res, nil
}
