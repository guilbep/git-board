package store

import (
	"cmp"
	"slices"
	"strings"

	"github.com/guilbep/git-board/core/people"
)

// Persons lists every person, as the sync reads them: a name, an email,
// and the accounts (server, id, login) a forge write resolves through.
func (b *Backend) Persons() ([]people.Person, error) {
	persons, err := b.allPersons()
	if err != nil {
		return nil, err
	}
	res := make([]people.Person, 0, len(persons))
	for _, p := range persons {
		res = append(res, *p)
	}
	slices.SortFunc(res, func(x, y people.Person) int { return cmp.Compare(x.Name, y.Name) })
	return res, nil
}

// Identities lists the persons, by name.
func (b *Backend) Identities() ([]Person, error) {
	persons, err := b.allPersons()
	if err != nil {
		return nil, err
	}
	m, err := b.avatarMode()
	if err != nil {
		return nil, err
	}
	res := []Person{}
	for id, p := range persons {
		res = append(res, personOf(id, p, m))
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
	return people.SetMe(ctx, b.whoStore(), id)
}

// NewIdentity creates a person and adopts them. You create them, or no one
// when no one is chosen yet.
func (b *Backend) NewIdentity(name, email string) (Person, error) {
	author, _ := people.Me(ctx, b.whoStore())
	p, err := people.Create(ctx, b.ds, people.NewCreate(author, name, email))
	if err != nil {
		return Person{}, err
	}
	if err := people.SetMe(ctx, b.whoStore(), p.ID); err != nil {
		return Person{}, err
	}
	m, err := b.avatarMode()
	if err != nil {
		return Person{}, err
	}
	return personOf(p.ID, p, m), nil
}

// MyName is what @me means (assignee:@me, assignee=+@me): the forge
// login of the person git-board writes as, else their name, as an
// assignee label names a person with no account (Logins). "" when no one
// is chosen.
// ponytail: their first account's login; the posting forge's when
// sync is wired (M5's second phase)
func (b *Backend) MyName() string {
	me, ok, err := b.Me()
	if err != nil || !ok {
		return ""
	}
	return cmp.Or(me.Login, me.Name)
}

// MyNames are every name a label may name you by: each of your forge
// logins, then your name. A label written as your name before you had an
// account still names you once you have one: sync sends it as that
// account (docs/spec/sync.md, Assignees). None when no one is chosen.
func (b *Backend) MyNames() []string {
	id, err := b.me()
	if err != nil {
		return nil
	}
	persons, err := b.allPersons()
	if err != nil || persons[id] == nil {
		return nil
	}
	p := persons[id]
	names := logins(p)
	if p.Name != "" && !slices.ContainsFunc(names, func(l string) bool { return strings.EqualFold(l, p.Name) }) {
		names = append(names, p.Name)
	}
	return names
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
