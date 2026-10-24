package people

import "github.com/guilbep/git-board/core/gitstore"

// Directory finds a person by account.
type Directory struct{ owner map[[2]string]*Person }

// LoadDirectory reads every person. An account belongs to the person whose
// id sorts first in byte order.
func LoadDirectory(s *gitstore.Store) (*Directory, error) {
	ids, err := List(s) // sorted, so the first to hold an account owns it
	if err != nil {
		return nil, err
	}
	d := &Directory{owner: map[[2]string]*Person{}}
	for _, id := range ids {
		p, err := Load(s, id)
		if err != nil {
			return nil, err
		}
		for _, a := range p.Accounts {
			if _, ok := d.owner[[2]string{a.Server, a.ID}]; !ok {
				d.owner[[2]string{a.Server, a.ID}] = p
			}
		}
	}
	return d, nil
}

// ForAccount returns the person that owns the account.
func (d *Directory) ForAccount(server, id string) (*Person, bool) {
	p, ok := d.owner[[2]string{server, id}]
	return p, ok
}
