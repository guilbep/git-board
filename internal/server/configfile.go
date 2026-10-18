package server

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/guilbep/git-board/docs"
	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/markdown"
)

// The settings page edits .git-board.toml whole: read it, check a draft
// (where it breaks, what git-board won't use), save it, and the server
// takes it at once, no restart.

func (s *apiServer) config() *config.Config {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	return s.cfg
}

func (s *apiServer) configPath() string {
	if s.cfgPath != "" {
		return s.cfgPath
	}
	return config.Path()
}

// readConfigFile is the file and its hash: what a save must start from.
func (s *apiServer) readConfigFile() (text string, exists bool, hash string, err error) {
	data, err := os.ReadFile(s.configPath())
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, "none", nil
	}
	if err != nil {
		return "", false, "", err
	}
	sum := sha256.Sum256(data)
	return string(data), true, hex.EncodeToString(sum[:]), nil
}

func (s *apiServer) handleConfigFile(w http.ResponseWriter, r *http.Request) {
	text, exists, hash, err := s.readConfigFile()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": s.configPath(), "text": text, "exists": exists, "hash": hash})
}

type configCheckResponse struct {
	OK       bool                `json:"ok"`
	Error    *config.SyntaxError `json:"error,omitempty"`
	Message  string              `json:"message,omitempty"` // an error without a position
	Warnings []string            `json:"warnings"`
	Summary  struct {
		Fields  []string `json:"fields"`
		Views   []string `json:"views"`
		Sprints int      `json:"sprints"`
	} `json:"summary"`
}

func checkConfig(text string) (*config.Config, configCheckResponse) {
	var res configCheckResponse
	res.Warnings = []string{}
	cfg, warnings, err := config.Check([]byte(text))
	var se *config.SyntaxError
	switch {
	case errors.As(err, &se):
		res.Error = se
		return nil, res
	case err != nil:
		res.Message = err.Error()
		return nil, res
	}
	res.OK, res.Warnings = true, append(res.Warnings, warnings...)
	for name := range cfg.Fields {
		res.Summary.Fields = append(res.Summary.Fields, name)
	}
	for name := range cfg.Views {
		res.Summary.Views = append(res.Summary.Views, name)
	}
	sort.Strings(res.Summary.Fields)
	sort.Strings(res.Summary.Views)
	res.Summary.Sprints = len(cfg.Sprints.List)
	return cfg, res
}

func (s *apiServer) handleConfigCheck(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Text string `json:"text"`
	}
	if !decode(w, r, &req) {
		return
	}
	_, res := checkConfig(req.Text)
	writeJSON(w, http.StatusOK, res)
}

// handleConfigSave writes the file when it is still the one the page read
// (base: its hash), then serves the new config.
func (s *apiServer) handleConfigSave(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Text string `json:"text"`
		Base string `json:"base"`
	}
	if !decode(w, r, &req) {
		return
	}
	cfg, res := checkConfig(req.Text)
	if !res.OK {
		writeJSON(w, http.StatusBadRequest, res)
		return
	}
	done, ok := s.beginWrite(w)
	if !ok {
		return
	}
	defer done()
	_, _, hash, err := s.readConfigFile()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if hash != req.Base {
		writeCoded(w, http.StatusConflict, "changed", errors.New("the file changed since you opened it: reload to see the new version"))
		return
	}
	if err := config.Save(s.configPath(), []byte(req.Text)); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.cfgMu.Lock()
	s.cfg = cfg
	s.cfgMu.Unlock()
	writeJSON(w, http.StatusOK, res)
}

// docSection is a part of a doc, one ## heading each: the settings page
// shows the one for the table under the cursor (docs/configuration.md),
// or the ones beside a step (docs/forges.md).
type docSection struct {
	ID    string `json:"id"`    // the heading's first word: fields, views…
	Title string `json:"title"` // the heading
	HTML  string `json:"html"`  // what follows it, rendered
}

// docSections cuts a doc at its ## headings; what comes before the first
// one (the # title) is left out.
func docSections(doc string) []docSection {
	var res []docSection
	var title string
	var body []string
	flush := func() {
		if title == "" {
			return
		}
		id, _, _ := strings.Cut(strings.ToLower(title), " ")
		res = append(res, docSection{ID: id, Title: title, HTML: markdown.Render(strings.Join(body, "\n"), func(u string) string { return u })})
	}
	for _, line := range strings.Split(doc, "\n") {
		if t, ok := strings.CutPrefix(line, "## "); ok {
			flush()
			title, body = t, nil
			continue
		}
		body = append(body, line)
	}
	flush()
	return res
}

var (
	configDocs = sync.OnceValue(func() []docSection { return docSections(docs.Configuration) })
	forgeDocs  = sync.OnceValue(func() []docSection { return docSections(docs.Forges) })
)

func (s *apiServer) handleConfigDocs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, configDocs())
}

func (s *apiServer) handleForgeDocs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, forgeDocs())
}

// handleSaveView writes one view's table into .git-board.toml, the board's
// "Save view" (or "Save as new view"): filter, group, sort and layout, the
// rest of the file untouched, served at once.
func (s *apiServer) handleSaveView(w http.ResponseWriter, r *http.Request) {
	var v config.View
	if !decode(w, r, &v) {
		return
	}
	done, ok := s.beginWrite(w)
	if !ok {
		return
	}
	defer done()
	text, _, _, err := s.readConfigFile()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	updated, err := config.SetViewText(text, r.PathValue("name"), v)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	cfg, res := checkConfig(updated)
	if !res.OK {
		writeJSON(w, http.StatusBadRequest, res)
		return
	}
	if err := config.Save(s.configPath(), []byte(updated)); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.cfgMu.Lock()
	s.cfg = cfg
	s.cfgMu.Unlock()
	writeJSON(w, http.StatusOK, cfg.Views)
}
