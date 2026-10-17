package main

import (
	"cmp"
	"context"
	"errors"
	"net/http"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/guilbep/git-board/internal/ghbridge"
	"github.com/guilbep/git-board/internal/gitbugio"
)

// The project's own numbers from Settings › Numbers (docs/configuration.md,
// Numbers): where they stand, starting them, numbering what origin has.
// Only these writes, when asked, push: refs/board/seq, to origin.

const numbersRemote = gitbugio.NumbersRemote

var (
	errNoOrigin    = errors.New("no origin remote: the numbers are kept on origin. Add one first (git remote add origin <url>)")
	errNotNumbered = errors.New("the project doesn't number its issues yet: start its numbers first")
)

// numbersResponse is GET /api/numbers, and what a write answers.
type numbersResponse struct {
	On     bool   `json:"on"`
	Prefix string `json:"prefix,omitempty"`
	Start  int    `json:"start,omitempty"`
	Next   int    `json:"next,omitempty"`
	gitbugio.Numbering
	Origin *gitbugio.Remote `json:"origin"` // where the counter goes; null without an origin remote
	Name   string           `json:"name"`   // the repository's, for a prefix: its GitHub forge's, else origin's
	GitHub *githubNumbers   `json:"github,omitempty"`
	Done   int              `json:"done"`            // a write: how many issues it numbered
	Error  string           `json:"error,omitempty"` // started, but the issues not numbered yet
}

// githubNumbers is what GitHub says, asked with ?github=1 before the
// switch, when a GitHub forge is set up.
type githubNumbers struct {
	Issues  string `json:"issues"`  // the repository's issues page
	Highest int    `json:"highest"` // its highest issue or pull request number
	Error   string `json:"error,omitempty"`
}

func (s *apiServer) numbers() (numbersResponse, error) {
	var res numbersResponse
	c, on, err := s.b.Counter()
	if err != nil {
		return res, err
	}
	if res.Numbering, err = s.b.Numbering(numbersRemote); err != nil {
		return res, err
	}
	if on {
		res.On, res.Prefix, res.Start, res.Next = true, c.Prefix, c.Start, c.Next()
		res.Numbering.GitHub = nil // the preview's, before the switch
	}
	remotes, err := s.b.Remotes()
	if err != nil {
		return res, err
	}
	for i := range remotes {
		if remotes[i].Name == numbersRemote {
			res.Origin = &remotes[i]
			res.Name = strings.TrimSuffix(path.Base(remotes[i].URL), ".git")
		}
	}
	if repo := s.githubRepo(); repo != "" {
		res.Name = path.Base(repo)
	}
	return res, nil
}

// githubRepo is the GitHub forge's repository, owner/name; "" for none.
// ponytail: the first GitHub forge; a project with two is unheard of
func (s *apiServer) githubRepo() string {
	bridges, _ := s.b.Bridges()
	for _, br := range bridges {
		if repo, ok := strings.CutPrefix(br.Where, "https://github.com/"); ok && br.Target == "github" {
			return repo
		}
	}
	return ""
}

func (s *apiServer) handleNumbers(w http.ResponseWriter, r *http.Request) {
	res, err := s.numbers()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if repo := s.githubRepo(); r.URL.Query().Has("github") && !res.On && repo != "" {
		ask := s.githubHighest
		if ask == nil {
			ask = ghbridge.HighestNumber
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		res.GitHub = &githubNumbers{Issues: "https://github.com/" + repo + "/issues"}
		if res.GitHub.Highest, err = ask(ctx, repo); err != nil {
			res.GitHub.Error = err.Error()
		}
	}
	writeJSON(w, http.StatusOK, res)
}

// handleNumbersInit starts the numbers (InitNumbers), then numbers what
// origin has. Started, a numbering that fails answers the numbers with
// its error: they are on, Number them now is the retry.
func (s *apiServer) handleNumbersInit(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Prefix string `json:"prefix"`
		Start  int    `json:"start"`
	}
	if !decode(w, r, &req) {
		return
	}
	if err := gitbugio.CheckNumbers(req.Prefix, req.Start); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	done, ok := s.beginNumbers(w)
	if !ok {
		return
	}
	defer done()
	if err := s.b.InitNumbers(numbersRemote, req.Prefix, req.Start); err != nil {
		writeNumbersError(w, err)
		return
	}
	n, err := s.b.NumberIssues(numbersRemote)
	failed := ""
	if err != nil {
		failed = gitbugio.NoCredentials("the numbers are on, but the issues aren't numbered yet: " + err.Error())
	}
	s.answerNumbers(w, n, failed)
}

// handleNumbersRun numbers what origin has, as git board numbers does.
func (s *apiServer) handleNumbersRun(w http.ResponseWriter, r *http.Request) {
	done, ok := s.beginNumbers(w)
	if !ok {
		return
	}
	defer done()
	if err := s.b.FetchNumbers(numbersRemote); err != nil {
		writeNumbersError(w, err)
		return
	}
	if _, on, err := s.b.Counter(); err != nil || !on {
		writeError(w, http.StatusConflict, cmp.Or(err, errNotNumbered))
		return
	}
	n, err := s.b.NumberIssues(numbersRemote)
	if err != nil {
		writeNumbersError(w, err)
		return
	}
	s.answerNumbers(w, n, "")
}

// beginNumbers starts a write to origin's counter: origin there, the
// write lock held, origin's issues fetched (what "on origin" means).
func (s *apiServer) beginNumbers(w http.ResponseWriter) (func(), bool) {
	remotes, err := s.b.Remotes()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return nil, false
	}
	if !slices.ContainsFunc(remotes, func(r gitbugio.Remote) bool { return r.Name == numbersRemote }) {
		writeError(w, http.StatusConflict, errNoOrigin)
		return nil, false
	}
	done, ok := s.beginWrite(w)
	if !ok {
		return nil, false
	}
	if err := s.b.Fetch(numbersRemote); err != nil {
		done()
		writeNumbersError(w, err)
		return nil, false
	}
	return done, true
}

func (s *apiServer) answerNumbers(w http.ResponseWriter, done int, failed string) {
	res, err := s.numbers()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	res.Done, res.Error = done, failed
	writeJSON(w, http.StatusOK, res)
}

// writeNumbersError answers 409 when someone else numbers, or started the
// numbers, first; else 502, origin's answer without credentials.
func writeNumbersError(w http.ResponseWriter, err error) {
	code := http.StatusBadGateway
	if errors.Is(err, gitbugio.ErrNumbered) || errors.Is(err, gitbugio.ErrNumbersMoved) {
		code = http.StatusConflict
	}
	writeError(w, code, errors.New(gitbugio.NoCredentials(err.Error())))
}
