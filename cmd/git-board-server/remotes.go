package main

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/go-git/go-git/v5/plumbing/transport"

	"github.com/guilbep/git-board/internal/gitbugio"
)

// The git remotes: issues and people go with git, as git bug pull and git
// bug push move them. Fetching, pulling and pushing reach the remote, so
// they happen only when asked; the list says what the last one left.
//
// ponytail: each runs within its request, as /api/sync/check does, not as
// a job: a fetch or push of the issues' refs takes seconds. A job with
// progress, like the forge sync's, when a big remote makes it hurt.

type remotesStatus struct {
	Remotes []remoteStatus `json:"remotes"`
}

type remoteStatus struct {
	gitbugio.Remote
	Checked *time.Time `json:"checked,omitempty"` // this server's last fetch, pull or push of it
}

func (s *apiServer) remotes() (remotesStatus, error) {
	rs, err := s.b.Remotes()
	if err != nil {
		return remotesStatus{}, err
	}
	st := remotesStatus{Remotes: []remoteStatus{}}
	s.forgeMu.Lock()
	defer s.forgeMu.Unlock()
	for _, r := range rs {
		row := remoteStatus{Remote: r}
		if t, ok := s.checked[r.Name]; ok {
			row.Checked = &t
		}
		st.Remotes = append(st.Remotes, row)
	}
	return st, nil
}

func (s *apiServer) handleRemotes(w http.ResponseWriter, r *http.Request) {
	st, err := s.remotes()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// handleRemote fetches from a remote (a check: nothing local changes),
// pulls from it (fetch, then merge) or pushes to it, and answers the
// remotes after.
func (s *apiServer) handleRemote(w http.ResponseWriter, r *http.Request) {
	name, action := r.PathValue("name"), r.PathValue("action")
	do := map[string]func(string) error{
		"fetch": s.b.Fetch,
		"pull": func(remote string) error {
			err := s.b.Pull(remote)
			if err == nil && remote == gitbugio.NumbersRemote {
				err = s.b.FetchNumbers(remote)
			}
			return err
		},
		// numbers go out with a push to origin the user asks for, never on
		// their own (docs/configuration.md, Numbers)
		"push": func(remote string) error {
			_, err := s.b.Push(remote)
			if err == nil && remote == gitbugio.NumbersRemote {
				_, err = s.b.NumberIssues(remote)
			}
			return err
		},
	}[action]
	if do == nil {
		writeError(w, http.StatusNotFound, fmt.Errorf("a remote takes fetch, pull or push, not %q", action))
		return
	}
	st, err := s.remotes()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if !slices.ContainsFunc(st.Remotes, func(r remoteStatus) bool { return r.Name == name }) {
		writeError(w, http.StatusNotFound, fmt.Errorf("no git remote is named %s", name))
		return
	}

	done, ok := s.beginWrite(w)
	if !ok {
		return
	}
	defer done()
	err = do(name)
	if errors.Is(err, transport.ErrEmptyRemoteRepository) {
		err = nil // nothing pushed there yet: nothing to pull
	}
	switch {
	case err != nil && strings.Contains(err.Error(), "non-fast-forward"):
		writeError(w, http.StatusConflict, fmt.Errorf("%s has changes this repository doesn't have: pull first", name))
		return
	case err != nil:
		writeError(w, http.StatusBadGateway, errors.New(gitbugio.NoCredentials(fmt.Sprintf("%s %s: %v", action, name, err))))
		return
	}
	s.forgeMu.Lock()
	if s.checked == nil {
		s.checked = map[string]time.Time{}
	}
	s.checked[name] = time.Now()
	s.forgeMu.Unlock()
	s.handleRemotes(w, r)
}
