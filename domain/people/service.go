package people

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/guilbep/git-board/kernel"
)

// ErrNoMe is a write with no one chosen to write as.
var ErrNoMe = errors.New("choose who you write as first")

// Service is the people domain on an open board.
type Service struct {
	Board   *kernel.Board
	Avatars Avatars
	Me      kernel.ID // board.me; "" for no one
	// SetMe writes board.me, the clone's own choice.
	SetMe func(ctx context.Context, id kernel.ID) error
}

// Whom is who writes: their id, or ErrNoMe.
func (s *Service) Whom() (kernel.ID, error) {
	if s.Me == "" {
		return "", ErrNoMe
	}
	return s.Me, nil
}

// WhoAmI is the person who writes, as a surface shows them; false when no
// one is chosen.
func (s *Service) WhoAmI(ctx context.Context) (Who, bool, error) {
	if s.Me == "" {
		return Who{}, false, nil
	}
	d, err := Read(ctx, s.Board, s.Avatars)
	if err != nil {
		return Who{}, false, err
	}
	if _, ok := d.ByID[s.Me]; !ok {
		return Who{}, false, fmt.Errorf("%w: you, person %s", kernel.ErrNotFound, s.Me)
	}
	return d.Who(s.Me), true, nil
}

// List is everyone on the board, by name then id.
func (s *Service) List(ctx context.Context) ([]Who, error) {
	d, err := Read(ctx, s.Board, s.Avatars)
	if err != nil {
		return nil, err
	}
	out := make([]Who, 0, len(d.ByID))
	for id := range d.ByID {
		out = append(out, d.Who(id))
	}
	slices.SortFunc(out, func(x, y Who) int { return cmp.Or(cmp.Compare(x.Name, y.Name), cmp.Compare(x.ID, y.ID)) })
	return out, nil
}

// ErrExists is a person made twice.
var ErrExists = errors.New("on the board already")

// New makes a person and writes as them. Who writes now makes them, or
// no one: the first person of a board. One with the same name and email
// is refused, and named.
func (s *Service) New(ctx context.Context, name, email string) (Who, error) {
	if name == "" {
		return Who{}, errors.New("no name: pass --name, or set git config user.name")
	}
	d, err := Read(ctx, s.Board, s.Avatars)
	if err != nil {
		return Who{}, err
	}
	for id, p := range d.ByID {
		if p.Name == name && strings.EqualFold(p.Email, email) {
			return Who{}, fmt.Errorf("%s <%s> is %w, as %.8s: git board me %.8s", name, email, ErrExists, id, id)
		}
	}
	data, _ := json.Marshal(struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}{name, email})
	e, err := Kind.Create(ctx, s.Board, "", kernel.Op{Kind: "create", Author: string(s.Me), Time: time.Now().Unix(), Data: data})
	if err != nil {
		return Who{}, err
	}
	if err := s.SetMe(ctx, e.ID); err != nil {
		return Who{}, err
	}
	s.Me = e.ID
	return Directory{ByID: map[kernel.ID]Person{e.ID: e.State}, Avatars: s.Avatars}.Who(e.ID), nil
}

// Adopt writes as the person a prefix names.
func (s *Service) Adopt(ctx context.Context, prefix string) error {
	id, err := Kind.Resolve(ctx, s.Board, prefix)
	if err != nil {
		return err
	}
	if err := s.SetMe(ctx, id); err != nil {
		return err
	}
	s.Me = id
	return nil
}
