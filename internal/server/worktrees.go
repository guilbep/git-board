package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/guilbep/git-board/internal/wtx"
)

// Why wtx can't run from here, by code.
var wtxRefusals = map[string]struct {
	status int
	reason string
}{
	"exposed": {http.StatusForbidden, "The server listens beyond this machine: agents start only from a server on localhost."},
	"no-wtx":  {http.StatusConflict, "wtx is not on PATH where git board serve runs."},
	"origin":  {http.StatusForbidden, "wtx runs only on a request from this board's own page."},
}

// wtxAllowed is the one gate every wtx run passes: loopback only, wtx
// on PATH, and for a write a same-host Origin (withGuard lets a POST
// without Origin through; this doesn't). code is "" when allowed.
func (s *apiServer) wtxAllowed(r *http.Request, write bool) (code, reason string) {
	switch origin := r.Header.Get("Origin"); {
	case len(s.exposed) > 0:
		code = "exposed"
	case !wtx.Available():
		code = "no-wtx"
	case write && (origin == "" || !sameHost(origin, r.Host) && !devOrigins[origin]):
		code = "origin"
	default:
		return "", ""
	}
	return code, wtxRefusals[code].reason
}

// refuseWtx answers the refusal and tells whether it did.
func (s *apiServer) refuseWtx(w http.ResponseWriter, r *http.Request) bool {
	code, reason := s.wtxAllowed(r, true)
	if code != "" {
		writeCoded(w, wtxRefusals[code].status, code, errors.New(reason))
	}
	return code != ""
}

type worktreesResponse struct {
	Local   bool            `json:"local"`             // the server listens on loopback alone
	Wtx     bool            `json:"wtx"`               // on PATH
	Reason  string          `json:"reason,omitempty"`  // why nothing can start from here; "" when it can
	Code    string          `json:"code,omitempty"`    // exposed, no-wtx, no-config
	Doctor  *wtx.Doctor     `json:"doctor,omitempty"`  // wtx doctor --json
	Config  bool            `json:"config"`            // wtx.toml in the repository
	Answers json.RawMessage `json:"answers,omitempty"` // wtx init --edit --no-notes
}

func (s *apiServer) handleWorktrees(w http.ResponseWriter, r *http.Request) {
	out := worktreesResponse{Local: len(s.exposed) == 0, Wtx: wtx.Available()}
	if out.Code, out.Reason = s.wtxAllowed(r, false); out.Code != "" {
		writeJSON(w, http.StatusOK, out)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Minute)
	defer cancel()
	// doctor exits 1 on a hard failure, its report still on stdout
	report, _ := wtx.Run(ctx, s.root, "doctor", "--json")
	var d wtx.Doctor
	if err := json.Unmarshal(report, &d); err != nil {
		writeError(w, http.StatusBadGateway, errors.New("wtx doctor --json: "+err.Error()))
		return
	}
	out.Doctor, out.Config = &d, d.Has("wtx.toml")
	answers, err := wtx.Run(ctx, s.root, "init", "--edit", "--no-notes")
	if err == nil && !json.Valid(answers) {
		err = errors.New("wtx init --edit: not JSON")
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	out.Answers = answers
	if !out.Config {
		out.Code, out.Reason = "no-config", "No wtx.toml in this repository yet"
	}
	writeJSON(w, http.StatusOK, out)
}

type wtxFiles struct {
	Files []wtxFile `json:"files"` // wtx.toml, .wt.toml
}

type wtxFile struct {
	Path string `json:"path"`
	Text string `json:"text"`
}

// handleWtxPreview runs wtx init --from-json in a scratch repository and
// answers what it wrote: the bytes Write puts in the checkout.
func (s *apiServer) handleWtxPreview(w http.ResponseWriter, r *http.Request) {
	s.wtxInit(w, r, false)
}

// handleWtxInit writes wtx.toml and .wt.toml in the checkout, uncommitted.
func (s *apiServer) handleWtxInit(w http.ResponseWriter, r *http.Request) {
	s.wtxInit(w, r, true)
}

func (s *apiServer) wtxInit(w http.ResponseWriter, r *http.Request, write bool) {
	if s.refuseWtx(w, r) {
		return
	}
	var body struct {
		Answers json.RawMessage `json:"answers"`
	}
	if !decode(w, r, &body) {
		return
	}
	if len(body.Answers) == 0 || string(body.Answers) == "null" {
		writeError(w, http.StatusBadRequest, errors.New("no answers"))
		return
	}
	file, err := tempFile("git-board-wtx-*.json", body.Answers)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer os.Remove(file)
	ctx, cancel := context.WithTimeout(r.Context(), time.Minute)
	defer cancel()

	dir, args := s.root, []string{"init", "--from-json", file}
	if write {
		// ponytail: --force always; the preview is what replaces the
		// file, and wtx's --edit carried its values.
		args = append(args, "--force")
		s.wtxMu.Lock()
		defer s.wtxMu.Unlock()
	} else {
		if dir, err = os.MkdirTemp("", "git-board-wtx-preview-"); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		defer os.RemoveAll(dir)
		if out, err := exec.CommandContext(ctx, "git", "init", "-q", dir).CombinedOutput(); err != nil {
			writeError(w, http.StatusInternalServerError, errors.New("git init: "+string(out)))
			return
		}
	}
	if _, err := wtx.Run(ctx, dir, args...); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	// ponytail: read back from the cwd; wtx writes in the main checkout,
	// which is where git board serve runs (git-bug's lock is there)
	var out wtxFiles
	for _, name := range []string{"wtx.toml", ".wt.toml"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		out.Files = append(out.Files, wtxFile{Path: name, Text: string(data)})
	}
	writeJSON(w, http.StatusOK, out)
}

// tempFile writes data to a new temp file and returns its path.
func tempFile(pattern string, data []byte) (string, error) {
	f, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", err
	}
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}
