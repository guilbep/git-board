package server

import (
	"net/http"

	"github.com/guilbep/git-board/internal/config"
)

// experimental serves h while feature is on ([experimental] features, or
// this clone's board.experimental); off, the route is not there: 404, its
// error naming the switch, code "experimental".
func (s *apiServer) experimental(feature string, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.config().Experimental(feature) {
			writeCoded(w, http.StatusNotFound, "experimental", config.ExperimentalOff(feature))
			return
		}
		h(w, r)
	}
}
