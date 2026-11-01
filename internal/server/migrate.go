package server

import (
	"errors"
	"net/http"

	"github.com/guilbep/git-board/core/forge"
	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/migrate"
	"github.com/guilbep/git-board/internal/store"
)

// The fields page's migration: a mapping changed, the sync writes the new
// home by itself, and this moves the values the sync won't revisit — the
// old labels off the forge, the assignees set. The preview counts; the
// run writes through the forge adapters, with their token rule.

func (s *apiServer) migrateField(w http.ResponseWriter, r *http.Request, run bool) {
	cfg := s.config()
	field := r.URL.Query().Get("field")
	if _, ok := cfg.Fields[field]; !ok {
		writeError(w, http.StatusBadRequest, errors.New("no field named "+field))
		return
	}
	issues, err := s.b.ListIssues()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if !run {
		writeJSON(w, http.StatusOK, map[string]any{"plans": migrate.Preview(cfg, field, issues)})
		return
	}
	say := func(string) {} // ponytail: the run's lines, streamed when there is a channel for them
	open := func(f config.Forge) (forge.Forge, error) {
		forges, err := store.OpenForges(r.Context(), []config.Forge{f})
		if err != nil {
			return nil, err
		}
		return forges[0].Forge, nil
	}
	plans, err := migrate.Migrate(r.Context(), cfg, field, issues, open, say)
	body := map[string]any{"plans": plans}
	if err != nil {
		body["error"] = err.Error()
	}
	writeJSON(w, http.StatusOK, body)
}
