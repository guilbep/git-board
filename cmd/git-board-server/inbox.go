package main

import (
	"errors"
	"net/http"
	"time"

	"github.com/guilbep/git-board/internal/board"
	"github.com/guilbep/git-board/internal/gitbugio"
	"github.com/guilbep/git-board/internal/inbox"
)

// The inbox (internal/inbox): what happened that concerns you on the
// issues of the last 14 days. Your file is yours, not the repository's:
// it takes inboxMu, never writeMu, and answers during a forge sync.

type inboxResponse struct {
	Me           inbox.Me    `json:"me"`
	Rows         []inbox.Row `json:"rows"`
	Fields       bool        `json:"fields"`
	Views        []string    `json:"views"`
	ViewsMissing []string    `json:"views_missing"`
	Following    int         `json:"following"` // open issues follows:@me matches: the empty state's
}

type followResponse struct {
	Following bool     `json:"following"`
	Reasons   []string `json:"reasons"`
	Set       *bool    `json:"set"` // what you clicked; null: automatic
}

// inboxMe is who the inbox is for, or the answer saying why there's none.
func (s *apiServer) inboxMe(w http.ResponseWriter) (inbox.Me, bool) {
	p, ok, err := s.b.Me()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return inbox.Me{}, false
	}
	if !ok {
		writeCoded(w, http.StatusConflict, "no-identity", errors.New("the inbox is the identity you write as: choose one in Settings"))
		return inbox.Me{}, false
	}
	login := s.me
	if login == "" {
		login = s.b.MyLogin()
	}
	if login == "" {
		writeCoded(w, http.StatusConflict, "no-login", errors.New("your identity has no forge login, so no field and no mention can name you: git bug bridge new tags it, or start the server with --me"))
		return inbox.Me{}, false
	}
	return inbox.Me{ID: p.ID, Login: login}, true
}

func (s *apiServer) loadInbox(id string) (inbox.State, error) {
	s.inboxMu.Lock()
	defer s.inboxMu.Unlock()
	return inbox.Load(s.b.Storage(), id)
}

// changeInbox loads your file, changes it and saves it, in one hold of
// inboxMu: two tabs writing at once both stick.
func (s *apiServer) changeInbox(id string, change func(*inbox.State)) error {
	s.inboxMu.Lock()
	defer s.inboxMu.Unlock()
	st, err := inbox.Load(s.b.Storage(), id)
	if err != nil {
		return err
	}
	change(&st)
	return inbox.Save(s.b.Storage(), id, st)
}

// recent is the issues updated within the window.
func recent(all []gitbugio.Issue, since time.Time) []gitbugio.Issue {
	var res []gitbugio.Issue
	for _, i := range all {
		if !i.UpdatedAt.Before(since) {
			res = append(res, i)
		}
	}
	return res
}

func (s *apiServer) handleInbox(w http.ResponseWriter, r *http.Request) {
	me, ok := s.inboxMe(w)
	if !ok {
		return
	}
	res, err := s.inbox(me, time.Now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// inbox gathers the window's threads and the followed views they're in,
// and asks inbox.Rows.
// ponytail: recomputed on every request, the header's count included;
// keep the last result per identity until the refs (RefsFingerprint), the
// file or the config change, if a big repository makes it slow.
func (s *apiServer) inbox(me inbox.Me, now time.Time) (inboxResponse, error) {
	st, err := s.loadInbox(me.ID)
	if err != nil {
		return inboxResponse{}, err
	}
	cfg := s.config()
	since := now.Add(-inbox.Window)
	all, err := s.b.ListIssues()
	if err != nil {
		return inboxResponse{}, err
	}
	window := recent(all, since)
	threads := make([]gitbugio.Thread, 0, len(window))
	for _, i := range window {
		th, err := s.b.Thread(i.ID)
		if err != nil {
			return inboxResponse{}, err
		}
		threads = append(threads, th)
	}
	inViews, missing, err := board.FollowedViews(s.b, cfg, st.Views, me.Login, window, now)
	if err != nil {
		return inboxResponse{}, err
	}
	following, err := board.Match(s.b, cfg, "follows:@me is:open", me.Login, all, now)
	if err != nil {
		return inboxResponse{}, err
	}
	res := inboxResponse{Me: me, Rows: inbox.Rows(me, st, threads, inViews, cfg, since), Fields: st.Fields,
		Views: st.Views, ViewsMissing: missing, Following: len(following)}
	if res.Rows == nil {
		res.Rows = []inbox.Row{}
	}
	if res.Views == nil {
		res.Views = []string{}
	}
	if res.ViewsMissing == nil {
		res.ViewsMissing = []string{}
	}
	return res, nil
}

// handleInboxSeen marks every event the issues have now as seen: an issue
// opened, or Mark all read.
func (s *apiServer) handleInboxSeen(w http.ResponseWriter, r *http.Request) {
	me, ok := s.inboxMe(w)
	if !ok {
		return
	}
	var req struct {
		Issues []string `json:"issues"`
	}
	if !decode(w, r, &req) {
		return
	}
	threads := make([]gitbugio.Thread, 0, len(req.Issues))
	for _, id := range req.Issues {
		th, err := s.b.Thread(id)
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		threads = append(threads, th)
	}
	all, err := s.b.ListIssues()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	keep := map[string]bool{}
	for _, i := range recent(all, time.Now().Add(-inbox.Window)) {
		keep[i.ID] = true
	}
	err = s.changeInbox(me.ID, func(st *inbox.State) {
		for _, th := range threads {
			st.MarkSeen(th)
		}
		st.Prune(keep)
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, struct{}{})
}

// follow is whether you follow the issue (an id or a prefix), and why.
func (s *apiServer) follow(me inbox.Me, th gitbugio.Thread) (followResponse, error) {
	st, err := s.loadInbox(me.ID)
	if err != nil {
		return followResponse{}, err
	}
	cfg := s.config()
	inViews, _, err := board.FollowedViews(s.b, cfg, st.Views, me.Login, []gitbugio.Issue{th.Issue}, time.Now())
	if err != nil {
		return followResponse{}, err
	}
	following, reasons, set := inbox.Follows(me, st, th, inViews[th.ID], cfg)
	if reasons == nil {
		reasons = []string{}
	}
	return followResponse{Following: following, Reasons: reasons, Set: set}, nil
}

func (s *apiServer) handleFollow(w http.ResponseWriter, r *http.Request) {
	me, ok := s.inboxMe(w)
	if !ok {
		return
	}
	th, err := s.b.Thread(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	res, err := s.follow(me, th)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleSetFollow is Follow (true), Unfollow (false), back to automatic
// (null), stored under the issue's full id.
func (s *apiServer) handleSetFollow(w http.ResponseWriter, r *http.Request) {
	me, ok := s.inboxMe(w)
	if !ok {
		return
	}
	var req struct {
		Follow *bool `json:"follow"`
	}
	if !decode(w, r, &req) {
		return
	}
	th, err := s.b.Thread(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err := s.changeInbox(me.ID, func(st *inbox.State) { st.SetFollow(th.ID, req.Follow) }); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	res, err := s.follow(me, th)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *apiServer) handleInboxSettings(w http.ResponseWriter, r *http.Request) {
	me, ok := s.inboxMe(w)
	if !ok {
		return
	}
	var req struct {
		Views  []string `json:"views"`
		Fields bool     `json:"fields"`
	}
	if !decode(w, r, &req) {
		return
	}
	if err := s.changeInbox(me.ID, func(st *inbox.State) { st.Views, st.Fields = req.Views, req.Fields }); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, struct{}{})
}
