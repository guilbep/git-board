package server

import (
	"fmt"
	"net/http"
	"sort"

	"github.com/guilbep/git-board/internal/config"
)

// The settings' forms read and write one section of .git-board.toml at a
// time. GET /api/config/model answers every section typed, from the config
// the server holds; PUT /api/config/section/{name}?base=<hash> applies one
// section's value (config.Set*Text: only its tables change, comments
// kept), checks the whole file, saves it when the file is still the one
// the form read (the base hash; else 409 changed, as the raw editor's
// save), and serves the check and the fresh model.

// configModel is every section the forms edit, as the file holds it now.
type configModel struct {
	Hash   string                  `json:"hash"`
	Exists bool                    `json:"exists"`
	Error  string                  `json:"error,omitempty"` // the file doesn't parse: the forms show it, Advanced fixes it
	Fields map[string]config.Field `json:"fields"`
	Labels config.Labels           `json:"labels"`
	Views  map[string]config.View  `json:"views"`
	// ViewOrder is the file's order: the tabs', the first the landing one.
	// The map above is sorted, this is the file's.
	ViewOrder []string `json:"view_order"`
	// FieldOrder is the file's order likewise: the fields page's list, the
	// order an issue's fields line takes.
	FieldOrder []string             `json:"field_order"`
	Sprints    config.SprintsConfig `json:"sprints"`
	Roles      map[string]string    `json:"roles"`
	// Branches, Cycle (its On says the file has the table), Repos, Uploads
	// and Release, as the file holds them
	Branches config.Branches `json:"branches"`
	Cycle    config.Cycle    `json:"cycle"`
	Repos    struct {
		List      []string `json:"list"`
		Assignees string   `json:"assignees"` // how the forges hold assignees: native, labels; preserved
	} `json:"repos"`
	Uploads config.Uploads `json:"uploads"`
	Release config.Release `json:"release"`
}

func (s *apiServer) configModel() (configModel, error) {
	text, exists, hash, err := s.readConfigFile()
	if err != nil {
		return configModel{}, err
	}
	m := configModel{Hash: hash, Exists: exists}
	m.Fields = map[string]config.Field{}
	m.Views = map[string]config.View{}
	m.Roles = map[string]string{}
	if !exists {
		return m, nil
	}
	cfg, _, err := config.Check([]byte(text))
	if err != nil {
		m.Error = err.Error()
		return m, nil
	}
	for name, f := range cfg.Fields {
		m.Fields[name] = f
	}
	m.Labels = cfg.Labels
	m.Labels.List = orEmpty(cfg.Labels.List)
	m.Labels.Colors = orEmptyMap(cfg.Labels.Colors)
	m.Labels.Descriptions = orEmptyMap(cfg.Labels.Descriptions)
	for name, v := range cfg.Views {
		m.Views[name] = v
	}
	m.ViewOrder = cfg.ViewNames()
	m.FieldOrder = cfg.FieldOrder()
	m.Sprints = cfg.Sprints
	m.Sprints.List = orSprints(cfg.Sprints.List)
	for role, who := range cfg.Roles {
		m.Roles[role] = who
	}
	m.Branches = cfg.Branches
	if m.Branches.Kinds == nil {
		m.Branches.Kinds = map[string]string{}
	}
	m.Cycle = cfg.Cycle
	m.Cycle.Kinds = orEmptyStringMap(cfg.Cycle.Kinds)
	m.Cycle.Signoffs = orEmptyStringMap(cfg.Cycle.Signoffs)
	if m.Cycle.Tests == nil {
		m.Cycle.Tests = map[string]config.Suite{}
	}
	if m.Cycle.Checks == nil {
		m.Cycle.Checks = map[string]config.Suite{}
	}
	m.Repos.List = orEmpty(cfg.Repos)
	m.Repos.Assignees = cfg.Assignees
	m.Uploads = cfg.Uploads
	m.Release = cfg.Release
	if m.Release.Images == nil {
		m.Release.Images = []config.Image{}
	}
	if m.Release.Deploy == nil {
		m.Release.Deploy = []config.Target{}
	}
	return m, nil
}

func orEmptyStringMap(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

func orSprints(list []config.Sprint) []config.Sprint {
	if list == nil {
		return []config.Sprint{}
	}
	return list
}

func (s *apiServer) handleConfigModel(w http.ResponseWriter, r *http.Request) {
	m, err := s.configModel()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

// handleConfigSection applies one section's value to the file and serves
// the check and the fresh model. The body's shape is the section's own:
// the same JSON the model answers, so a form sends back what it holds.
func (s *apiServer) handleConfigSection(w http.ResponseWriter, r *http.Request) {
	apply, err := sectionWriter(r.PathValue("section"), r, w)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if apply == nil {
		return // a bad body: decode already answered
	}
	done, ok := s.beginWrite(w)
	if !ok {
		return
	}
	defer done()
	text, _, hash, err := s.readConfigFile()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	updated, err := apply(text)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	cfg, res := checkConfig(updated)
	if !res.OK {
		writeJSON(w, http.StatusBadRequest, res)
		return
	}
	if hash != r.FormValue("base") {
		writeCoded(w, http.StatusConflict, "changed", fmt.Errorf("the file changed since you opened it: reload to see the new version"))
		return
	}
	if err := config.Save(s.configPath(), []byte(updated)); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.setConfig(cfg)
	m, err := s.configModel()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Check configCheckResponse `json:"check"`
		Model configModel         `json:"model"`
	}{Check: res, Model: m})
}

// sectionWriter decodes the section's body and returns what applies it to
// the file's text. It answers the request itself (and returns nil, nil)
// when the body is wrong; a section it doesn't know is an error.
func sectionWriter(section string, r *http.Request, w http.ResponseWriter) (func(string) (string, error), error) {
	switch section {
	case "fields":
		var req struct {
			Set    map[string]config.Field `json:"set"`
			Remove []string                `json:"remove"`
			Order  []string                `json:"order"` // the list's: each field moved above the one before it
		}
		if !decode(w, r, &req) {
			return nil, nil
		}
		return func(text string) (string, error) {
			text, err := config.SetFieldsText(text, req.Set, req.Remove)
			if err != nil {
				return "", err
			}
			// the file's order is the list's: realize it pairwise, from the
			// bottom up — each field moved above the one after it, which the
			// later moves leave where they put it
			for i := len(req.Order) - 2; i >= 0; i-- {
				if text, err = config.MoveFieldText(text, req.Order[i], req.Order[i+1]); err != nil {
					return "", err
				}
			}
			return text, nil
		}, nil
	case "labels":
		var req config.Labels
		if !decode(w, r, &req) {
			return nil, nil
		}
		return func(text string) (string, error) { return config.SetLabelsText(text, req) }, nil
	case "views":
		var req struct {
			Set    map[string]config.View `json:"set"`
			Remove []string               `json:"remove"`
			Order  []string               `json:"order"` // the tabs': each view moved above the one before it
		}
		if !decode(w, r, &req) {
			return nil, nil
		}
		return func(text string) (string, error) {
			// the sets first, then the removals: a rename is set the new
			// name, remove the old, and the old must not win
			text, err := setViewsText(text, req.Set)
			if err != nil {
				return "", err
			}
			for _, name := range req.Remove {
				if _, saved := req.Set[name]; saved {
					continue
				}
				if text, err = config.DeleteViewText(text, name); err != nil {
					return "", err
				}
			}
			// the file's order is the tabs': realize it pairwise, from the
			// bottom up — each view moved above the one after it, which the
			// later moves leave where they put it
			for i := len(req.Order) - 2; i >= 0; i-- {
				if text, err = config.MoveViewText(text, req.Order[i], req.Order[i+1]); err != nil {
					return "", err
				}
			}
			return text, nil
		}, nil
	case "sprints":
		var req config.SprintsConfig
		if !decode(w, r, &req) {
			return nil, nil
		}
		return func(text string) (string, error) { return config.SetSprintsText(text, req) }, nil
	case "roles":
		var req map[string]string
		if !decode(w, r, &req) {
			return nil, nil
		}
		return func(text string) (string, error) { return config.SetRolesText(text, req) }, nil
	case "branches":
		var req config.Branches
		if !decode(w, r, &req) {
			return nil, nil
		}
		return func(text string) (string, error) { return config.SetBranchesText(text, req) }, nil
	case "cycle":
		var req struct {
			On    bool         `json:"on"`
			Cycle config.Cycle `json:"cycle"`
		}
		if !decode(w, r, &req) {
			return nil, nil
		}
		return func(text string) (string, error) { return config.SetCycleText(text, req.On, req.Cycle) }, nil
	case "repos":
		var req struct {
			List      []string `json:"list"`
			Assignees string   `json:"assignees"`
		}
		if !decode(w, r, &req) {
			return nil, nil
		}
		return func(text string) (string, error) { return config.SetReposText(text, req.List, req.Assignees) }, nil
	case "uploads":
		var req config.Uploads
		if !decode(w, r, &req) {
			return nil, nil
		}
		return func(text string) (string, error) { return config.SetUploadsText(text, req) }, nil
	case "release":
		var req config.Release
		if !decode(w, r, &req) {
			return nil, nil
		}
		return func(text string) (string, error) { return config.SetReleaseText(text, req) }, nil
	}
	return nil, fmt.Errorf("no section %q: fields, labels, views, sprints, roles, branches, cycle, repos, uploads, release", section)
}

// setViewsText applies each view's table, alphabetically for a stable
// file: one view's save touches one table, several touch several, none
// the other's lines.
func setViewsText(text string, set map[string]config.View) (string, error) {
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	var err error
	for _, name := range names {
		if text, err = config.SetViewText(text, name, set[name]); err != nil {
			return "", err
		}
	}
	return text, nil
}
