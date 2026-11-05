package issues

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/guilbep/git-board/next/domain/people"
	"github.com/guilbep/git-board/next/kernel"
)

// Service is the issue domain on an open board.
type Service struct {
	Board   *kernel.Board
	Schema  *Schema        // never nil: an empty one without a config
	Avatars people.Avatars // board.avatars
	Me      kernel.ID      // who writes: board.me; "" for no one
	// MeName is what @me means when a request doesn't say: serve's --me,
	// GIT_BOARD_ME; "" for the person who writes.
	MeName string
	// Notice, when set, is told what a read leaves aside (an entity
	// broken, a counter that doesn't read), as v1 logs it.
	Notice Notice
	// Folder is the clone's own folder, the inbox's files' (v1's).
	Folder string
	// Views are .git-board.toml's, which an inbox follows by name.
	Views map[string]View
	// Writing, when set, brackets each write (a Markdown folder's).
	Writing Writing
	// Held, when set, says which labels the sync keeps off the forge of
	// a key (forge.Forges.Held): none of them waits for that forge.
	Held func(key, label string) bool
}

// notice tells s.Notice, if any.
func (s *Service) notice(format string, args ...any) {
	if s.Notice != nil {
		s.Notice(format, args...)
	}
}

// DefaultFilter is ls's filter when none is given.
const DefaultFilter = "is:open"

// FilterError is a filter that can't be answered as written, whatever the
// board holds: @me when it means nobody, a date that isn't one.
type FilterError struct{ Err error }

func (e *FilterError) Error() string { return e.Err.Error() }
func (e *FilterError) Unwrap() error { return e.Err }

// ErrNoMe is @me with no one to mean.
var ErrNoMe = errors.New("@me is the person you write as: choose one (git board me --new, Settings › You), or pass --me / set GIT_BOARD_ME")

// ListReq asks for the issues a filter matches.
type ListReq struct {
	Filter string    // ls's grammar; "" matches every issue
	Me     string    // what @me means: --me, GIT_BOARD_ME; "" for the person who writes
	Today  time.Time // what @today and sprints mean; zero is now
}

// List answers the issues r.Filter matches, oldest first (by creation,
// then id), as v1's ls lists them before its --sort.
func (s *Service) List(ctx context.Context, r ListReq) ([]Issue, error) {
	ctx = s.Board.Snapshot(ctx)
	if r.Today.IsZero() {
		r.Today = time.Now()
	}
	dir, err := people.Read(ctx, s.Board, s.Avatars)
	if err != nil {
		return nil, err
	}
	// the free text's hits first: a first build then indexes as it reads
	conds, me, err := s.prepare(ctx, dir, r, true)
	if err != nil {
		return nil, err
	}
	rows, err := Kind.List(ctx, s.Board, kernel.Query{Sort: "created"})
	if err != nil {
		return nil, fmt.Errorf("listing the issues: %w", err)
	}
	broken, err := s.Board.Broken(ctx)
	if err != nil {
		return nil, err
	}
	for _, b := range broken {
		s.notice("%s %s: %v", b.Kind, b.ID, b.Err)
	}
	rd := s.reading(ctx, s.notice, true)
	list := make([]Issue, 0, len(rows))
	for _, l := range rows {
		is := s.issue(l.ID, &l.Row, dir, rd)
		is.Version = l.Version
		is.Inherited = s.Schema.RolesFor(is.Labels) // a list's alone, as v1's
		list = append(list, is)
	}
	if slices.ContainsFunc(conds, func(c Condition) bool { return c.Kind == QFollows }) {
		f, err := s.following(ctx, dir, me, list, r.Today)
		if err != nil {
			return nil, err
		}
		for i := range conds {
			if conds[i].Kind == QFollows {
				conds[i].Followed = f
			}
		}
	}
	return s.keep(list, conds, me, r.Today), nil
}

// keep keeps the issues conds match, each with what it inherits.
func (s *Service) keep(list []Issue, conds []Condition, me string, today time.Time) []Issue {
	out := list[:0]
	for _, is := range list {
		is.Inherited = s.Schema.RolesFor(is.Labels) // a list's alone, as v1's
		if Matches(is, conds, s.Schema, me, today) {
			out = append(out, is)
		}
	}
	return out
}

// prepare parses a filter, refuses what can't be answered, finds who @me
// is, and asks the view for each free text's hits (queries.md, Text).
func (s *Service) prepare(ctx context.Context, dir people.Directory, r ListReq, search bool) ([]Condition, string, error) {
	conds := ParseFilter(r.Filter)
	if err := CheckFilter(conds, r.Today); err != nil {
		return nil, "", &FilterError{err}
	}
	me := cmp.Or(r.Me, s.MeName)
	if RequiresMe(conds) && me == "" {
		mine := s.myNames(dir)
		if len(mine) > 0 {
			who := dir.Who(s.Me)
			me = cmp.Or(who.Login, who.Name)
		}
		for i := range conds {
			conds[i].Mine = mine
		}
	}
	if RequiresMe(conds) && me == "" {
		return nil, "", &FilterError{ErrNoMe}
	}
	in := TextsIn(conds)
	for i, c := range conds {
		switch {
		case c.Kind != QText || !search:
		case in != nil && len(in) == 0: // -in: of every text: nowhere
			conds[i].Hits = map[string]bool{}
		default:
			hits, err := Kind.Match(ctx, s.Board, kernel.Query{Text: c.Value, In: in})
			if err != nil {
				return nil, "", err
			}
			conds[i].Hits = idSet(hits)
		}
	}
	for i, c := range conds {
		if c.Kind == QFollows || (c.Kind == QInvolves && c.Value == "@me") {
			conds[i].MeID = string(s.Me)
		}
	}
	for i, c := range conds {
		if c.Kind != QInvolves && c.Kind != QFollows {
			continue
		}
		login := c.Value
		if c.Kind == QFollows || login == "@me" {
			login = me
		}
		var err error
		if conds[i].Mentions, err = s.mentioning(ctx, login); err != nil {
			return nil, "", err
		}
	}
	return conds, me, nil
}

// myNames are every name a label may name the person who writes by: each
// forge login, then their name. None when no one is chosen.
func (s *Service) myNames(dir people.Directory) []string {
	p, ok := dir.ByID[s.Me]
	if s.Me == "" || !ok {
		return nil
	}
	names := p.Logins()
	if p.Name != "" && !slices.ContainsFunc(names, func(l string) bool { return strings.EqualFold(l, p.Name) }) {
		names = append(names, p.Name)
	}
	return names
}

// mentioning are the issues whose description or a comment mentions
// @login.
func (s *Service) mentioning(ctx context.Context, login string) (map[string]bool, error) {
	res := map[string]bool{}
	if login == "" {
		return res, nil
	}
	hits, err := Kind.Match(ctx, s.Board, kernel.Query{Text: login})
	if err != nil {
		return nil, err
	}
	for _, id := range hits {
		e, err := Kind.Get(ctx, s.Board, id)
		if err != nil {
			return nil, err
		}
		m := Mentions(e.State.Body, login)
		for _, c := range e.State.Comments {
			m = m || Mentions(c.Body, login)
		}
		if m {
			res[string(id)] = true
		}
	}
	return res, nil
}

func idSet(ids []kernel.ID) map[string]bool {
	m := make(map[string]bool, len(ids))
	for _, id := range ids {
		m[string(id)] = true
	}
	return m
}

// issue is a row as every surface lists it: its labels as git-board reads
// them, who opened it and who commented. A list adds what it inherits.
func (s *Service) issue(id kernel.ID, r *Row, dir people.Directory, rd reading) Issue {
	is := Issue{ID: string(id), Title: r.Title, Status: string(r.Status), Labels: s.Schema.Shown(r.Labels),
		UpdatedAt: time.Unix(r.Updated, 0), CreatedAt: time.Unix(r.Created, 0), AuthorID: string(r.Author),
		Comments: r.Comments}
	if is.Labels == nil {
		is.Labels = []string{}
	}
	if r.Status == Closed {
		is.ClosedAt = time.Unix(r.Closed, 0)
	}
	is.forged(rd.recs[string(id)])
	is.Shown = is.ShownTitle()
	is.numbered(rd.cnt)
	is.AuthorLogins = dir.Names(r.Author)
	if len(r.Commenters) > 0 {
		is.CommenterIDs = make([]string, 0, len(r.Commenters))
		is.CommenterLogins = make([]string, 0, len(r.Commenters))
	}
	for _, c := range r.Commenters {
		is.CommenterIDs = append(is.CommenterIDs, string(c))
		is.CommenterLogins = append(is.CommenterLogins, dir.Logins(c)...)
	}
	return is
}

// numberName is an issue named by a number: a forge's, #2518, or its
// project number, CO2-3075.
var numberName = regexp.MustCompile(`^(#[0-9]+|[A-Za-z][A-Za-z0-9]*-[0-9]+)$`)

// ErrNotFound is no such issue, or a prefix naming several.
var ErrNotFound = kernel.ErrNotFound

// ErrDeleted is an issue a delete hid (ADR 0019): a not found.
var ErrDeleted = fmt.Errorf("%w: deleted", kernel.ErrNotFound)

// Resolve is the whole id an issue's name gives: a prefix of its id, its
// project number (CO2-3075) or a forge's (#2518).
func (s *Service) Resolve(ctx context.Context, name string) (kernel.ID, error) {
	if !numberName.MatchString(name) {
		return Kind.Resolve(ctx, s.Board, name)
	}
	all, err := s.List(ctx, ListReq{})
	if err != nil {
		return "", err
	}
	var ids []kernel.ID
	for _, is := range all {
		if forge, ok := strings.CutPrefix(name, "#"); ok && is.HasForgeNumber(forge) || strings.EqualFold(is.Number, name) {
			ids = append(ids, kernel.ID(is.ID))
		}
	}
	switch len(ids) {
	case 0:
		return "", fmt.Errorf("%w: issue %s", ErrNotFound, name)
	case 1:
		return ids[0], nil
	}
	return "", fmt.Errorf("%w: issue %s, on %d issues", kernel.ErrAmbiguous, name, len(ids))
}
