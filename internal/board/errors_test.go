package board

import (
	"errors"
	"testing"

	"github.com/guilbep/git-board/core/model"
	"github.com/guilbep/git-board/internal/config"
)

type failing struct{ err error }

func (f failing) ListIssues() ([]model.Issue, error) { return nil, f.err }

type listed []model.Issue

func (l listed) ListIssues() ([]model.Issue, error) { return l, nil }

// A filter that can't be answered is a FilterError; reading the board
// failing is not one.
func TestFilterErrorIsTheFilters(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{}
	_, err := LoadMatching(listed{{Title: "x"}}, cfg, "assignee:@me", "")
	if !IsFilterError(err) || !errors.Is(err, ErrNoMe) {
		t.Errorf("@me meaning nobody: %v, want a FilterError of ErrNoMe", err)
	}
	if _, err := Facets(listed{{Title: "x"}}, cfg, "assignee:@me", ""); !IsFilterError(err) {
		t.Errorf("facets, @me meaning nobody: %v, want a FilterError", err)
	}
	_, err = LoadMatching(listed{{Title: "x"}}, cfg, "follows:@me", "ann")
	if !IsFilterError(err) {
		t.Errorf("follows:@me without a repository: %v, want a FilterError", err)
	}
	broken := errors.New("datastore: gone")
	_, err = LoadMatching(failing{broken}, cfg, "is:open", "")
	if IsFilterError(err) || !errors.Is(err, broken) {
		t.Errorf("the board failing: %v, want it as it is, no FilterError", err)
	}
}
