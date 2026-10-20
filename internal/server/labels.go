package server

import (
	"errors"
	"maps"
	"net/http"
	"slices"
	"strings"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/ghlabels"
	"github.com/guilbep/git-board/internal/lint"
)

// Settings › Labels: the labels the project defines, their issues, and how
// each GitHub repository has them (internal/ghlabels, through gh, as you);
// GitHub's color and description adopted into a draft of the file, or the
// file's sent to GitHub; lint, and the GitHub labels outside the project's.

// labelRepos is [repos], else origin's GitHub repository, else none.
func (s *apiServer) labelRepos(cfg *config.Config) []string {
	if len(cfg.Repos) > 0 {
		return cfg.Repos
	}
	if r := s.b.GitHubRepo("origin"); r != "" {
		return []string{r}
	}
	return nil
}

type repoError struct {
	Repo    string `json:"repo"`
	Message string `json:"message"`
}

// githubLabels is each repository's labels; one gh can't read is in errs.
// ponytail: one gh label list per repository on every load; cache them if
// it drags.
func githubLabels(repos []string) (map[string][]ghlabels.Label, []repoError) {
	res, errs := map[string][]ghlabels.Label{}, []repoError{}
	for _, repo := range repos {
		labels, err := ghlabels.Existing(repo)
		if err != nil {
			errs = append(errs, repoError{repo, err.Error()})
			continue
		}
		res[repo] = labels
	}
	return res, errs
}

// labelCounts is how many issues carry each label, as the board reads it
// (a flat kind as kind:bug); open ones only when open.
func (s *apiServer) labelCounts(open bool) (map[string]int, error) {
	issues, err := s.b.ListIssues()
	if err != nil {
		return nil, err
	}
	res := map[string]int{}
	for _, issue := range issues {
		if open && issue.Status != "open" {
			continue
		}
		for _, l := range issue.Labels {
			res[strings.ToLower(l)]++
		}
	}
	return res, nil
}

type labelOnGitHub struct {
	Repo        string `json:"repo"`
	State       string `json:"state"` // ghlabels.State
	Color       string `json:"color"`
	Description string `json:"description"`
}

type labelRow struct {
	ghlabels.Label
	Field          string          `json:"field"`
	Value          string          `json:"value"`
	OwnColor       bool            `json:"own_color"`
	OwnDescription bool            `json:"own_description"`
	Count          int             `json:"count"` // open issues
	GitHub         []labelOnGitHub `json:"github"`
}

func (s *apiServer) handleLabelList(w http.ResponseWriter, r *http.Request) {
	cfg := s.config()
	counts, err := s.labelCounts(true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	repos := s.labelRepos(cfg)
	existing, errs := githubLabels(repos)
	rows := []labelRow{}
	for _, want := range ghlabels.Desired(cfg) {
		row := labelRow{Label: want, Field: want.Field, Value: want.Value, OwnColor: want.OwnColor, OwnDescription: want.OwnDescription,
			Count: counts[strings.ToLower(want.Field+":"+want.Value)], GitHub: []labelOnGitHub{}}
		for _, repo := range repos {
			if labels, ok := existing[repo]; ok {
				got := labelOnGitHub{Repo: repo, State: ghlabels.State(want, labels)}
				if i := slices.IndexFunc(labels, func(l ghlabels.Label) bool { return strings.EqualFold(l.Name, want.Name) }); i >= 0 {
					got.Color, got.Description = labels[i].Color, labels[i].Description
				}
				row.GitHub = append(row.GitHub, got)
			}
		}
		rows = append(rows, row)
	}
	writeJSON(w, http.StatusOK, map[string]any{"repos": orEmpty(repos), "rows": rows, "errors": errs})
}

type labelChange struct {
	ghlabels.Change
	Error string `json:"error,omitempty"`
}

// handleLabelSend creates the missing labels on each repository and
// updates those the file colors or describes otherwise (ghlabels.Plan);
// dry_run lists them.
func (s *apiServer) handleLabelSend(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DryRun bool `json:"dry_run"`
	}
	if !decode(w, r, &req) {
		return
	}
	cfg := s.config()
	repos := s.labelRepos(cfg)
	existing, errs := githubLabels(repos)
	desired := ghlabels.Desired(cfg)
	changes := []labelChange{}
	for _, repo := range repos {
		labels, ok := existing[repo]
		if !ok {
			continue
		}
		for _, c := range ghlabels.Plan(repo, desired, labels) {
			done := labelChange{Change: c}
			if !req.DryRun {
				if err := ghlabels.Apply(c); err != nil {
					done.Error = err.Error()
				}
			}
			changes = append(changes, done)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"changes": changes, "errors": errs})
}

// handleLabelAdopt writes GitHub's color and description of a label into
// text, the page's draft of the file: [fields.<field>.colors] and
// .descriptions, the others in them kept. It writes no file: the Project
// file tab reviews and saves the draft.
func (s *apiServer) handleLabelAdopt(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Text        string `json:"text"`
		Field       string `json:"field"`
		Value       string `json:"value"`
		Color       string `json:"color"`
		Description string `json:"description"`
	}
	if !decode(w, r, &req) {
		return
	}
	cfg, res := checkConfig(req.Text)
	if !res.OK {
		writeJSON(w, http.StatusBadRequest, res)
		return
	}
	if !slices.ContainsFunc(ghlabels.Desired(cfg), func(l ghlabels.Label) bool { return l.Field == req.Field && l.Value == req.Value }) {
		writeError(w, http.StatusBadRequest, errors.New("not one of the project's labels: "+req.Field+":"+req.Value))
		return
	}
	set := func(m map[string]string, v string) map[string]string {
		m = maps.Clone(m)
		if m == nil {
			m = map[string]string{}
		}
		maps.DeleteFunc(m, func(k, _ string) bool { return strings.EqualFold(k, req.Value) })
		if v != "" {
			m[req.Value] = v
		}
		return m
	}
	f, text := cfg.Fields[req.Field], req.Text
	if req.Color != "" {
		text = config.SetFieldTable(text, req.Field, "colors", set(f.Colors, "#"+strings.ToLower(req.Color)))
	}
	text = config.SetFieldTable(text, req.Field, "descriptions", set(f.Descriptions, strings.TrimSpace(req.Description)))
	if _, res := checkConfig(text); !res.OK {
		writeJSON(w, http.StatusBadRequest, res)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"text": text})
}

type strayLabel struct {
	ghlabels.Label
	Repo  string `json:"repo"`
	Count int    `json:"count"` // local issues, open or closed
}

// handleLint is `git board lint`, and each repository's labels outside the
// project's (ghlabels.Stray) with the issues here that carry them.
func (s *apiServer) handleLint(w http.ResponseWriter, r *http.Request) {
	cfg := s.config()
	issues, err := s.b.ListIssues()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	findings := lint.Run(issues, cfg)
	slices.SortStableFunc(findings, func(a, b lint.Finding) int {
		return slices.Index(lint.CheckOrder, a.Check) - slices.Index(lint.CheckOrder, b.Check)
	})
	// ponytail: counted as the board reads labels, so in the flat form a
	// stray kind:bug counts the bug issues too; the delete's confirm asks
	// GitHub for its own count.
	counts, err := s.labelCounts(false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	repos := s.labelRepos(cfg)
	existing, errs := githubLabels(repos)
	desired := ghlabels.Desired(cfg)
	stray := []strayLabel{}
	for _, repo := range repos {
		for _, l := range ghlabels.Stray(cfg, desired, existing[repo]) {
			stray = append(stray, strayLabel{Label: l, Repo: repo, Count: counts[strings.ToLower(l.Name)]})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"findings": orEmpty(findings), "stray": stray, "errors": errs})
}

// handleLabelDelete deletes a GitHub label outside the project's from one
// of its repositories; dry_run answers how many issues and pull requests
// carry it there. A label the project defines is refused.
func (s *apiServer) handleLabelDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Repo   string `json:"repo"`
		Name   string `json:"name"`
		DryRun bool   `json:"dry_run"`
	}
	if !decode(w, r, &req) {
		return
	}
	cfg := s.config()
	if !slices.Contains(s.labelRepos(cfg), req.Repo) {
		writeError(w, http.StatusBadRequest, errors.New("not one of the project's repositories: "+req.Repo))
		return
	}
	if slices.ContainsFunc(ghlabels.Desired(cfg), func(l ghlabels.Label) bool { return strings.EqualFold(l.Name, req.Name) }) {
		writeError(w, http.StatusBadRequest, errors.New(req.Name+" is one of the project's labels"))
		return
	}
	if req.DryRun {
		n, err := ghlabels.Count(req.Repo, req.Name)
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]int{"count": n})
		return
	}
	if err := ghlabels.Delete(req.Repo, req.Name); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}
