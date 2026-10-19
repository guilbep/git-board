package server

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/guilbep/git-board/internal/branches"
	"github.com/guilbep/git-board/internal/gitbugio"
)

// The Changes tab (docs/prd/work-unit.md, M4): an issue's branch diffed
// against where it left the base, and the review written on it as comments
// with review:* metadata, which the fork's sync mirrors to the pull request.

var errNoBranch = errors.New("no branch names this issue yet")

// changeRange is what the Changes tab shows: a branch of the issue, the
// base it starts from, and the commits compared (from…to, base…head by
// default).
type changeRange struct {
	Branch   string `json:"branch"`
	BaseName string `json:"base_name"`
	Base     string `json:"base"` // where the branch left the base
	Head     string `json:"head"`
	From     string `json:"from"`
	To       string `json:"to"`
	start    string // Suggestion.Start: origin/<base>, or <base>
}

// reviewHead picks the issue's branch to review, want or else the first
// origin has, and its head: origin's copy, what the pull request shows and
// what others can review, else the local one. want must name the issue.
func reviewHead(b *gitbugio.Backend, is gitbugio.Issue, want string) (name, ref string, err error) {
	all, err := b.Branches()
	if err != nil {
		return "", "", err
	}
	var names []string
	refs := map[string]string{} // branch → origin's copy, else the first other
	for _, br := range all {
		if !branches.Names(br.Name, is) || want != "" && br.Name != want {
			continue
		}
		if _, seen := refs[br.Name]; !seen {
			names = append(names, br.Name)
		}
		if br.Remote == "origin" || refs[br.Name] == "" {
			refs[br.Name] = strings.TrimPrefix(br.Remote+"/"+br.Name, "/")
		}
	}
	if len(names) == 0 {
		if want != "" {
			return "", "", fmt.Errorf("%q isn't a branch of this issue", want)
		}
		return "", "", errNoBranch
	}
	for _, n := range names {
		if strings.HasPrefix(refs[n], "origin/") {
			return n, refs[n], nil
		}
	}
	return names[0], refs[names[0]], nil
}

// changeRangeOf reads the range a request asks for (?branch=&from=&to=),
// answering the error itself when it can't.
func (s *apiServer) changeRangeOf(w http.ResponseWriter, r *http.Request) (changeRange, bool) {
	is, err := s.b.GetIssue(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return changeRange{}, false
	}
	rg, err := s.rangeOf(is, r.URL.Query().Get("branch"))
	if errors.Is(err, errNoBranch) {
		writeCoded(w, http.StatusNotFound, "no-branch", err)
		return rg, false
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return rg, false
	}
	for param, into := range map[string]*string{"from": &rg.From, "to": &rg.To} {
		if q := r.URL.Query().Get(param); q != "" {
			if *into, err = s.b.RevParse(q); err != nil {
				writeError(w, http.StatusBadRequest, err)
				return rg, false
			}
		}
	}
	return rg, true
}

// rangeOf is a branch of the issue from its merge base to its head.
func (s *apiServer) rangeOf(is gitbugio.Issue, want string) (changeRange, error) {
	name, ref, err := reviewHead(s.b, is, want)
	if err != nil {
		return changeRange{}, err
	}
	sug, err := branches.Suggest(s.b, s.config().Branches, is)
	if err != nil {
		return changeRange{}, err
	}
	head, err := s.b.RevParse(ref)
	if err != nil {
		return changeRange{}, err
	}
	base, err := s.b.MergeBase(sug.Start, head)
	if err != nil {
		return changeRange{}, fmt.Errorf("%s and %s: %w", sug.Start, name, err)
	}
	return changeRange{Branch: name, BaseName: sug.Base, Base: base, Head: head, From: base, To: head, start: sug.Start}, nil
}

func (s *apiServer) handleChanges(w http.ResponseWriter, r *http.Request) {
	rg, ok := s.changeRangeOf(w, r)
	if !ok {
		return
	}
	commits, err := s.b.Log(rg.Base, rg.Head)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	files, err := s.b.DiffStat(rg.From, rg.To)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		changeRange
		Commits []gitbugio.Commit   `json:"commits"`
		Files   []gitbugio.FileStat `json:"files"`
	}{rg, commits, files})
}

// handleChangesFile answers one file's diff (?path=, &from_path= when
// renamed), cut after ?limit= lines.
func (s *apiServer) handleChangesFile(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	path := q.Get("path")
	if path == "" {
		writeError(w, http.StatusBadRequest, errors.New("which file? (path)"))
		return
	}
	limit := 0
	if l := q.Get("limit"); l != "" {
		var err error
		if limit, err = strconv.Atoi(l); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("limit %q", l))
			return
		}
	}
	rg, ok := s.changeRangeOf(w, r)
	if !ok {
		return
	}
	paths := []string{path}
	if from := q.Get("from_path"); from != "" {
		paths = append(paths, from)
	}
	diff, truncated, err := s.b.DiffFile(rg.From, rg.To, limit, paths...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": path, "diff": diff, "truncated": truncated})
}

// pullKey is a pull request as review:pr writes it.
var pullKey = regexp.MustCompile(`^github\.com/[^/#\s]+/[^/#\s]+#[0-9]+$`)

var reviewStates = map[string]bool{"APPROVE": true, "REQUEST_CHANGES": true, "COMMENT": true}

// handleReview writes a line comment (path, line, side, commit), a reply
// (reply_to: it takes its thread's anchor), or a review (state, commit).
// The commit is resolved to its full SHA; pr is left out until there is one.
func (s *apiServer) handleReview(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Body    string `json:"body"`
		Path    string `json:"path"`
		Line    int    `json:"line"`
		Side    string `json:"side"`
		Commit  string `json:"commit"`
		PR      string `json:"pr"`
		ReplyTo string `json:"reply_to"`
		State   string `json:"state"`
	}
	if !decode(w, r, &req) {
		return
	}
	bad := func(msg string) { writeError(w, http.StatusBadRequest, errors.New(msg)) }
	if req.PR != "" && !pullKey.MatchString(req.PR) {
		bad("pr: want github.com/<owner>/<repo>#<number>")
		return
	}
	done, ok := s.beginWrite(w)
	if !ok {
		return
	}
	defer done()
	id := r.PathValue("id")
	empty := strings.TrimSpace(req.Body) == ""
	meta := map[string]string{}
	switch {
	case req.ReplyTo != "":
		parent, err := s.b.CommentReview(id, req.ReplyTo)
		if err != nil {
			writeIssueError(w, err)
			return
		}
		if parent == nil || parent.Path == "" {
			bad("a reply answers a line comment")
			return
		}
		if empty {
			bad("an empty reply")
			return
		}
		root := req.ReplyTo
		if parent.ReplyTo != "" { // GitHub's replies all answer the thread's first comment
			root = parent.ReplyTo
		}
		req.Path, req.Line, req.Side, req.Commit = parent.Path, parent.Line, parent.Side, parent.Commit
		if parent.PR != "" {
			req.PR = parent.PR
		}
		meta["review:reply-to"] = root
	case req.State != "":
		if !reviewStates[req.State] {
			bad("state: want APPROVE, REQUEST_CHANGES or COMMENT")
			return
		}
		if empty && req.State != "APPROVE" {
			bad("say what to change, or comment something")
			return
		}
		meta["review:state"] = req.State
	case req.Path != "":
		if req.Line < 1 || req.Side != "old" && req.Side != "new" {
			bad("a line comment needs a line and a side, old or new")
			return
		}
		if empty {
			bad("an empty comment")
			return
		}
	default:
		bad("a line comment needs a path, a reply reply_to, a review a state")
		return
	}
	if req.Commit == "" {
		bad("which commit? (commit)")
		return
	}
	sha, err := s.b.RevParse(req.Commit)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	meta["review:commit"] = sha
	if req.Path != "" {
		meta["review:path"], meta["review:line"], meta["review:side"] = req.Path, strconv.Itoa(req.Line), req.Side
	}
	if req.PR != "" { // the fork keeps the first value: never write an empty one
		meta["review:pr"] = req.PR
	}
	if _, err := s.b.CommentWith(id, req.Body, meta); err != nil {
		writeIssueError(w, err)
		return
	}
	s.writeThread(w, http.StatusCreated, id)
}

// handleResolve resolves a thread, or reopens it, on its first comment.
func (s *apiServer) handleResolve(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Resolved bool `json:"resolved"`
	}
	if !decode(w, r, &req) {
		return
	}
	done, ok := s.beginWrite(w)
	if !ok {
		return
	}
	defer done()
	id, comment := r.PathValue("id"), r.PathValue("comment")
	rv, err := s.b.CommentReview(id, comment)
	if err != nil {
		writeIssueError(w, err)
		return
	}
	if rv == nil || rv.Path == "" || rv.ReplyTo != "" {
		writeError(w, http.StatusBadRequest, errors.New("resolve a thread on its first line comment"))
		return
	}
	if err := s.b.SetCommentMetadata(id, comment, map[string]string{"review:resolved": strconv.FormatBool(req.Resolved)}); err != nil {
		writeIssueError(w, err)
		return
	}
	s.writeThread(w, http.StatusOK, id)
}

// annotate fills the line comments of a thread from the issue's branch
// head: the line's text at the comment's commit, where the line is at
// head, and whether a change since covers it. A comment on the old side
// quotes the base, where the branch left it, and is never outdated: the
// merge base moves only on a rebase. Nothing without a repository, a
// branch, or the comment's commit (an imported one never fetched).
// ponytail: one git diff per comment and one git show per (commit, path);
// group LineAt per (commit, path) if threads with many comments are slow.
func (s *apiServer) annotate(th *gitbugio.Thread) {
	var rg *changeRange
	lines := map[string][]string{}
	for _, e := range th.Events {
		rv := e.Review
		if rv == nil || rv.Path == "" || rv.Line < 1 || !s.b.HasCommit(rv.Commit) {
			continue
		}
		if rg == nil {
			is, err := s.b.GetIssue(th.Issue.ID)
			if err != nil {
				return
			}
			got, err := s.rangeOf(is, "")
			if err != nil {
				return
			}
			rg = &got
		}
		at := rv.Commit
		if rv.Side == "old" {
			base, err := s.b.MergeBase(rg.start, rv.Commit)
			if err != nil {
				continue
			}
			at = base
			rv.HeadLine = rv.Line
		} else if line, ok, err := s.b.LineAt(rv.Commit, rg.Head, rv.Path, rv.Line); err == nil {
			rv.HeadLine, rv.Outdated = line, !ok
		}
		key := at + ":" + rv.Path
		if _, ok := lines[key]; !ok {
			lines[key], _ = s.b.FileLines(at, rv.Path)
		}
		if l := lines[key]; rv.Line <= len(l) {
			rv.Text = l[rv.Line-1]
		}
	}
}
