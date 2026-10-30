package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/guilbep/git-board/internal/branches"
	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/store"
)

// issueRef is an issue an issue field names: its value as written (#12,
// a git-bug id), and the issue when one in the repository matches.
type issueRef struct {
	Value  string `json:"value"`
	ID     string `json:"id,omitempty"`
	Title  string `json:"title,omitempty"`
	Status string `json:"status,omitempty"`
}

// refersTo: #12 names an issue with that number on one of its forges; a
// hex prefix of 7 or more, its git-bug id.
func refersTo(value string, issue store.Issue) bool {
	if n, ok := strings.CutPrefix(value, "#"); ok {
		for _, f := range issue.Forges {
			if f.Number == n {
				return true
			}
		}
		return false
	}
	return len(value) >= 7 && strings.HasPrefix(issue.ID, value)
}

// handleRefs answers an issue's links through its issue fields (parent,
// blocked-by…): out, the issues it names; in, the issues naming it (its
// sub-issues, what it blocks).
func (s *apiServer) handleRefs(w http.ResponseWriter, r *http.Request) {
	me, err := s.b.GetIssue(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	issues, err := s.b.ListIssues()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	cfg := s.config()
	isRef := map[string]bool{}
	for name, vs := range s.labelValues() {
		isRef[name] = cfg.TypeOf(name, vs) == config.TypeIssue
	}
	out, in := map[string][]issueRef{}, map[string][]issueRef{}
	for _, l := range me.Labels {
		field, v, _ := strings.Cut(l, ":")
		if !isRef[field] {
			continue
		}
		ref := issueRef{Value: v}
		for _, o := range issues {
			if o.ID != me.ID && refersTo(v, o) {
				ref.ID, ref.Title, ref.Status = o.ID, o.Title, o.Status
				break
			}
		}
		out[field] = append(out[field], ref)
	}
	for _, o := range issues {
		if o.ID == me.ID {
			continue
		}
		for _, l := range o.Labels {
			if field, v, _ := strings.Cut(l, ":"); isRef[field] && refersTo(v, me) {
				in[field] = append(in[field], issueRef{Value: v, ID: o.ID, Title: o.Title, Status: o.Status})
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"out": out, "in": in})
}

// handleBranches answers the branch to create for an issue and the
// branches naming it (internal/branches). It creates nothing.
func (s *apiServer) handleBranches(w http.ResponseWriter, r *http.Request) {
	is, err := s.b.GetIssue(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	sug, linked, err := s.branchesOf(is)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		branches.Suggestion
		Branches []branches.Branch `json:"branches"`
	}{sug, linked})
}

// branchesOf is the branch to create for an issue and the branches naming
// it, against the base.
func (s *apiServer) branchesOf(is store.Issue) (branches.Suggestion, []branches.Branch, error) {
	sug, err := branches.Suggest(s.b, s.config().Branches, is)
	if err != nil {
		return sug, nil, err
	}
	linked, err := branches.Linked(s.b, is, sug.Start)
	return sug, linked, err
}

// localWrite is the gate a write to the repository's branches passes, as
// wtx runs do (wtxAllowed): loopback only, and a same-host Origin, the
// board's own page. code is "" when allowed.
func (s *apiServer) localWrite(r *http.Request) (code, reason string) {
	switch origin := r.Header.Get("Origin"); {
	case len(s.exposed) > 0:
		return "exposed", "The server listens beyond this machine: branches are made only from a server on localhost."
	case origin == "" || !sameHost(origin, r.Host) && !devOrigins[origin]:
		return "origin", "A branch is made only on a request from this board's own page."
	}
	return "", ""
}

// pushTimeout bounds the push to origin: a forge that doesn't answer
// leaves the branch local, said so.
const pushTimeout = time.Minute

// handleCreateBranch makes the issue's branch (docs/design/issue-head,
// decision 3, "both: Create and Copy"): `git branch --no-track <name>
// <start>`, then `git push -u origin <name>`. It never switches: the
// clone the server runs in keeps its checkout. The name, the suggested
// one when none is given, must be a branch name git takes as it is, name
// the issue, and be new. A refused push leaves the branch local, its push
// command on the chip; the answer is the branches as GET gives them.
func (s *apiServer) handleCreateBranch(w http.ResponseWriter, r *http.Request) {
	if code, reason := s.localWrite(r); code != "" {
		writeCoded(w, http.StatusForbidden, code, errors.New(reason))
		return
	}
	is, err := s.b.GetIssue(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	sug, linked, err := s.branchesOf(is)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = sug.Suggested
	}
	switch {
	case !branches.Valid(name):
		writeCoded(w, http.StatusBadRequest, "bad-name", errors.New("Not a branch name git takes as it is: letters, digits, . _ - and / between words."))
		return
	case !branches.Names(name, is):
		writeCoded(w, http.StatusBadRequest, "not-this-issue", errors.New("The name doesn't name this issue: its number must be the name's first number."))
		return
	case slices.ContainsFunc(linked, func(br branches.Branch) bool { return br.Name == name }) || s.b.HasBranch(name):
		writeCoded(w, http.StatusConflict, "exists", errors.New("A branch of that name exists already."))
		return
	}
	if err := s.b.CreateBranch(name, sug.Start); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), pushTimeout)
	defer cancel()
	pushErr := s.b.PushBranch(ctx, "origin", name)
	sug, linked, err = s.branchesOf(is)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	res := struct {
		branches.Suggestion
		Branches  []branches.Branch `json:"branches"`
		Created   string            `json:"created"`
		Pushed    bool              `json:"pushed"`
		PushError string            `json:"push_error,omitempty"`
	}{Suggestion: sug, Branches: linked, Created: name, Pushed: pushErr == nil}
	if pushErr != nil {
		res.PushError = pushErr.Error()
	}
	writeJSON(w, http.StatusOK, res)
}
