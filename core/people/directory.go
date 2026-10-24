package people

import "github.com/guilbep/git-board/core/gitstore"

// Directory finds a person by account.
type Directory struct{ owner map[[2]string]*Person }

// LoadDirectory reads every person. An account belongs to the person whose
// id sorts first in byte order.
func LoadDirectory(s *gitstore.Store) (*Directory, error) {
	ids, err := List(s)
	if err != nil {
		return nil, err
	}
	d := &Directory{owner: map[[2]string]*Person{}}
	for _, id := range ids {
		p, err := Load(s, id)
		if err != nil {
			return nil, err
		}
		d.Put(p)
	}
	return d, nil
}

// Put takes in a person made or changed since the directory loaded. It owns
// each of its accounts that no lower id holds.
// ponytail: an account p no longer holds keeps it as owner; LoadDirectory
// again when accounts get removed.
func (d *Directory) Put(p *Person) {
	for _, a := range p.Accounts {
		k := [2]string{a.Server, a.ID}
		if o, ok := d.owner[k]; !ok || p.ID <= o.ID {
			d.owner[k] = p
		}
	}
}

// ForAccount returns the person that owns the account.
func (d *Directory) ForAccount(server, id string) (*Person, bool) {
	p, ok := d.owner[[2]string{server, id}]
	return p, ok
}
