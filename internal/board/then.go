package board

import (
	"time"

	"github.com/guilbep/git-board/core/model"
	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/store"
)

// LoadMatchingAt is LoadMatching on the board as it stood at a time
// (store.Backend.ListIssuesAt), with today then: @current is that day's
// sprint, @today that day. Free text looks in the issues' texts as they
// were, not through the index, which holds now's. Who you are, what you
// follow and who is mentioned are as now.
func LoadMatchingAt(b *store.Backend, cfg *config.Config, filterStr, me string, at time.Time) ([]model.Issue, error) {
	issues, err := b.ListIssuesAt(at)
	if err != nil {
		return nil, err
	}
	return Match(then{b}, cfg, filterStr, me, issues, at)
}

// then is b without its full-text search, for LoadMatchingAt.
type then struct{ b *store.Backend }

func (t then) MyName() string                                   { return t.b.MyName() }
func (t then) MyNames() []string                                { return t.b.MyNames() }
func (t then) Me() (model.Person, bool, error)                  { return t.b.Me() }
func (t then) Storage() store.Storage                           { return t.b.Storage() }
func (t then) Mentioning(login string) (map[string]bool, error) { return t.b.Mentioning(login) }
