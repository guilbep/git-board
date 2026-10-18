package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"

	"github.com/git-bug/git-bug/entity"

	"github.com/guilbep/git-board/internal/gitbugio"
	"github.com/guilbep/git-board/internal/markdown"
)

var errSyncing = errors.New("a sync with the forges is running: try again when it ends")

// beginWrite takes the write lock, which every change to the repository
// holds — but answers 409 at once while a forge sync runs, which holds it
// for minutes. The caller defers the returned unlock.
func (s *apiServer) beginWrite(w http.ResponseWriter) (func(), bool) {
	if s.syncing.Load() {
		writeCoded(w, http.StatusConflict, "syncing", errSyncing)
		return nil, false
	}
	s.writeMu.Lock()
	return s.writeMu.Unlock, true
}

// decode reads a request's JSON body, answering 400 when it can't.
func decode(w http.ResponseWriter, r *http.Request, into any) bool {
	if err := json.NewDecoder(r.Body).Decode(into); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return false
	}
	return true
}

// writeIssueError answers 404 for an issue that doesn't exist, 400 for what
// the request got wrong.
func writeIssueError(w http.ResponseWriter, err error) {
	if entity.IsErrNotFound(err) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeError(w, http.StatusBadRequest, err)
}

// writeThread answers with an issue's history, its texts rendered.
func (s *apiServer) writeThread(w http.ResponseWriter, status int, id string) {
	th, err := s.b.Thread(id)
	if err != nil {
		writeIssueError(w, err)
		return
	}
	for i, e := range th.Events {
		if e.Body != "" {
			th.Events[i].HTML = markdown.Render(e.Body, s.mediaURL)
		}
	}
	writeJSON(w, status, th)
}

// mediaURL is where the page loads an image of issue text from: the image
// proxy for a forge's images, else the image's own address.
func (s *apiServer) mediaURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || !forgeMedia(u, s.forgeHosts()) {
		return raw
	}
	return "/api/media?url=" + url.QueryEscape(raw)
}

func (s *apiServer) forgeHosts() []string {
	s.hostsOnce.Do(func() { s.hosts = s.b.ForgeHosts() })
	return s.hosts
}

func (s *apiServer) handleMedia(w http.ResponseWriter, r *http.Request) {
	s.mediaOnce.Do(func() { s.media = newMediaProxy(s.b.Storage(), s.b.ForgeToken, s.forgeHosts()) })
	s.media.ServeHTTP(w, r)
}

func (s *apiServer) handleThread(w http.ResponseWriter, r *http.Request) {
	s.writeThread(w, http.StatusOK, r.PathValue("id"))
}

func (s *apiServer) handleNewIssue(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title  string   `json:"title"`
		Body   string   `json:"body"`
		Labels []string `json:"labels"`
	}
	if !decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Title) == "" {
		writeError(w, http.StatusBadRequest, errors.New("an issue needs a title"))
		return
	}
	done, ok := s.beginWrite(w)
	if !ok {
		return
	}
	defer done()
	id, err := s.b.NewIssue(strings.TrimSpace(req.Title), req.Body, req.Labels)
	if err != nil {
		writeIssueError(w, err)
		return
	}
	// no number yet: it comes with the push to origin the user asks for
	s.writeThread(w, http.StatusCreated, id)
}

// handleComment comments, and optionally closes or reopens: "Close with
// comment".
func (s *apiServer) handleComment(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Body   string `json:"body"`
		Status string `json:"status"`
	}
	if !decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Body) == "" && req.Status == "" {
		writeError(w, http.StatusBadRequest, errors.New("an empty comment"))
		return
	}
	done, ok := s.beginWrite(w)
	if !ok {
		return
	}
	defer done()
	id := r.PathValue("id")
	if strings.TrimSpace(req.Body) != "" {
		if _, err := s.b.AddComment(id, req.Body); err != nil {
			writeIssueError(w, err)
			return
		}
	}
	if req.Status != "" {
		if err := s.b.SetStatus(id, req.Status); err != nil {
			writeIssueError(w, err)
			return
		}
	}
	s.writeThread(w, http.StatusOK, id)
}

func (s *apiServer) handleEditComment(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Body string `json:"body"`
		Base string `json:"base"` // the text the edit started from
	}
	if !decode(w, r, &req) {
		return
	}
	done, ok := s.beginWrite(w)
	if !ok {
		return
	}
	defer done()
	if err := s.b.EditComment(r.PathValue("id"), r.PathValue("comment"), req.Body, req.Base); err != nil {
		if errors.Is(err, gitbugio.ErrChangedMeanwhile) {
			writeCoded(w, http.StatusConflict, "changed", err)
			return
		}
		writeIssueError(w, err)
		return
	}
	s.writeThread(w, http.StatusOK, r.PathValue("id"))
}

func (s *apiServer) handleTitle(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title string `json:"title"`
	}
	if !decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Title) == "" {
		writeError(w, http.StatusBadRequest, errors.New("an issue needs a title"))
		return
	}
	done, ok := s.beginWrite(w)
	if !ok {
		return
	}
	defer done()
	if err := s.b.SetTitle(r.PathValue("id"), strings.TrimSpace(req.Title)); err != nil {
		writeIssueError(w, err)
		return
	}
	s.writeThread(w, http.StatusOK, r.PathValue("id"))
}

func (s *apiServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Status string `json:"status"`
	}
	if !decode(w, r, &req) {
		return
	}
	done, ok := s.beginWrite(w)
	if !ok {
		return
	}
	defer done()
	if err := s.b.SetStatus(r.PathValue("id"), req.Status); err != nil {
		writeIssueError(w, err)
		return
	}
	s.writeThread(w, http.StatusOK, r.PathValue("id"))
}

// handleLabels adds and removes labels, plain ones included (fields go
// through PATCH /api/issues/{id}, which knows their rules).
func (s *apiServer) handleLabels(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Add    []string `json:"add"`
		Remove []string `json:"remove"`
	}
	if !decode(w, r, &req) {
		return
	}
	done, ok := s.beginWrite(w)
	if !ok {
		return
	}
	defer done()
	id := r.PathValue("id")
	current, err := s.b.GetLabels(id)
	if err != nil {
		writeIssueError(w, err)
		return
	}
	// git-bug refuses adding what's there or removing what isn't
	add := slices.DeleteFunc(slices.Clone(req.Add), func(l string) bool { return l == "" || slices.Contains(current, l) })
	remove := slices.DeleteFunc(slices.Clone(req.Remove), func(l string) bool { return !slices.Contains(current, l) })
	if _, err := s.b.ApplyLabelChange(id, add, remove); err != nil {
		writeIssueError(w, err)
		return
	}
	s.writeThread(w, http.StatusOK, id)
}

// handleMarkdown renders a text as a comment would be: the composer's
// preview. A POST for the body's size; it writes nothing.
func (s *apiServer) handleMarkdown(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Body string `json:"body"`
	}
	if !decode(w, r, &req) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"html": markdown.Render(req.Body, s.mediaURL)})
}

// meResponse is who git-board writes as, and who it could.
type meResponse struct {
	Me         *gitbugio.Person  `json:"me"`
	Identities []gitbugio.Person `json:"identities"`
}

func (s *apiServer) writeMe(w http.ResponseWriter) {
	var res meResponse
	me, ok, err := s.b.Me()
	if err == nil && ok {
		res.Me = &me
	}
	if err == nil {
		res.Identities, err = s.b.Identities()
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *apiServer) handleMe(w http.ResponseWriter, r *http.Request) { s.writeMe(w) }

// handleSetMe adopts an identity ({"id"}) or creates one ({"name",
// "email"}): who comments and edits from git-board.
func (s *apiServer) handleSetMe(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if !decode(w, r, &req) {
		return
	}
	done, ok := s.beginWrite(w)
	if !ok {
		return
	}
	defer done()
	var err error
	switch {
	case req.ID != "":
		err = s.b.Adopt(req.ID)
	case strings.TrimSpace(req.Name) != "":
		_, err = s.b.NewIdentity(strings.TrimSpace(req.Name), strings.TrimSpace(req.Email))
	default:
		err = errors.New("pass an identity's id, or a name to create one")
	}
	if err != nil {
		writeIssueError(w, err)
		return
	}
	s.writeMe(w)
}

// lazyOnce groups the server's lazily built parts.
type lazyOnce struct {
	hostsOnce sync.Once
	hosts     []string
	mediaOnce sync.Once
	media     *mediaProxy
}
