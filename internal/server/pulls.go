package server

import (
	"cmp"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/guilbep/git-board/internal/branches"
	"github.com/guilbep/git-board/internal/ghpr"
	"github.com/guilbep/git-board/internal/gitbugio"
)

// The PR gate (docs/prd/work-unit.md, M3): for each branch on origin
// naming an issue, the GitHub pull request whose head it is, found by the
// branch's name. Open PR and Merge act on GitHub as whoever uses
// git-board, with their own token (Backend.GitHubToken).

// branchPull is a branch on origin and its pull request, nil for none.
type branchPull struct {
	Branch string   `json:"branch"`
	PR     *ghpr.PR `json:"pr"`
}

// pullsResponse is an issue's gate. Repo is "" when origin isn't on GitHub.
type pullsResponse struct {
	Repo  string       `json:"repo"`
	Base  string       `json:"base"`
	Login string       `json:"login"`
	Pulls []branchPull `json:"pulls"`
}

// github is GitHub's API as the user: their login and token.
func (s *apiServer) github() (*ghpr.Client, string, error) {
	login, token, err := s.b.GitHubToken()
	if err != nil {
		return nil, login, err
	}
	return &ghpr.Client{API: s.githubAPI, Token: token}, login, nil
}

// onOrigin are the issue's branches origin has, by name.
func onOrigin(b *gitbugio.Backend, is gitbugio.Issue) ([]string, error) {
	all, err := b.Branches()
	if err != nil {
		return nil, err
	}
	var names []string
	for _, br := range all {
		if br.Remote == "origin" && branches.Names(br.Name, is) {
			names = append(names, br.Name)
		}
	}
	return names, nil
}

// handlePulls answers the pull request of each of the issue's branches on
// origin. It asks GitHub nothing when origin isn't GitHub or has none of
// them, so Development works offline.
// ponytail: one GraphQL request per branch, in turn; an issue has one to
// three. Aliases in one query if that grows.
func (s *apiServer) handlePulls(w http.ResponseWriter, r *http.Request) {
	is, err := s.b.GetIssue(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	sug, err := branches.Suggest(s.b, s.config().Branches, is)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	res := pullsResponse{Repo: s.b.GitHubRepo("origin"), Base: sug.Base, Pulls: []branchPull{}}
	names, err := onOrigin(s.b, is)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if res.Repo == "" || len(names) == 0 {
		writeJSON(w, http.StatusOK, res)
		return
	}
	c, login, err := s.github()
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	res.Login = login
	required := map[string][]string{} // by base, read for an open PR only
	for _, name := range names {
		pr, err := c.Find(r.Context(), res.Repo, name)
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		if pr != nil {
			if _, ok := required[pr.Base]; !ok && pr.State == "OPEN" {
				if required[pr.Base], err = c.Required(r.Context(), res.Repo, pr.Base); err != nil {
					writeError(w, http.StatusBadGateway, err)
					return
				}
			}
			ghpr.Gate(pr, required[pr.Base])
		}
		res.Pulls = append(res.Pulls, branchPull{name, pr})
	}
	writeJSON(w, http.StatusOK, res)
}

// handleOpenPull opens a pull request from one of the issue's branches on
// origin into the base, titled and described from the issue.
func (s *apiServer) handleOpenPull(w http.ResponseWriter, r *http.Request) {
	is, err := s.b.GetIssue(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	var in struct {
		Branch string `json:"branch"`
	}
	if !decode(w, r, &in) {
		return
	}
	repo := s.b.GitHubRepo("origin")
	if repo == "" {
		writeError(w, http.StatusBadRequest, errors.New("origin isn't on GitHub"))
		return
	}
	names, err := onOrigin(s.b, is)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if !slices.Contains(names, in.Branch) {
		writeError(w, http.StatusBadRequest, fmt.Errorf("%q isn't a branch of this issue on origin: push it first", in.Branch))
		return
	}
	sug, err := branches.Suggest(s.b, s.config().Branches, is)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	c, _, err := s.github()
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	pr, err := c.Open(r.Context(), repo, in.Branch, sug.Base, prTitle(is), prBody(is, repo))
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusCreated, pr)
}

// prTitle is the issue as lists show it: CO2-3075 and its shown title.
func prTitle(is gitbugio.Issue) string {
	return strings.TrimSpace(is.Number + " " + is.ShownTitle())
}

// prBody names the issue (its project number, else its git-bug id's first
// 7 characters) and, when it is an issue of the same GitHub repository,
// closes it there on merge (into the default branch, as GitHub does it).
func prBody(is gitbugio.Issue, repo string) string {
	body := cmp.Or(is.Number, is.ID[:min(7, len(is.ID))])
	for _, f := range is.Forges {
		if f.Gone == "" && f.Number != "" && strings.EqualFold(f.URL, "https://github.com/"+repo+"/issues/"+f.Number) {
			return body + "\n\nCloses #" + f.Number
		}
	}
	return body
}

// handleMergePull merges a pull request with a merge commit, if its head
// is still the commit the page showed.
func (s *apiServer) handleMergePull(w http.ResponseWriter, r *http.Request) {
	if _, err := s.b.GetIssue(r.PathValue("id")); err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	number, err := strconv.Atoi(r.PathValue("number"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var in struct {
		SHA string `json:"sha"`
	}
	if !decode(w, r, &in) {
		return
	}
	repo := s.b.GitHubRepo("origin")
	if in.SHA == "" || repo == "" {
		writeError(w, http.StatusBadRequest, errors.New("a merge needs the head commit shown, and origin on GitHub"))
		return
	}
	c, _, err := s.github()
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if err := c.Merge(r.Context(), repo, number, in.SHA); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, struct{}{})
}
