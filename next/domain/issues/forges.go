package issues

import (
	"context"
	"net/url"
	"path"
	"slices"
	"strings"

	"github.com/guilbep/git-board/next/kernel"
	bsync "github.com/guilbep/git-board/next/sync"
)

// reading is what turning rows into issues reads once: the counter, and
// the sync entities' records (docs/spec/sync.md, Records). now marks, in
// a thread, what waits for a forge: not on a board as it was then.
type reading struct {
	cnt  *Counter
	recs map[string][]bsync.Record
	sum  string // the sync entities' versions, for a cache's key
	now  bool
}

// reading is the board's counter and records. Records that don't read
// are told to notice: the issues show without their forges.
func (s *Service) reading(ctx context.Context, notice Notice, now bool) (rd reading) {
	rd = reading{cnt: s.counter(ctx, notice), now: now}
	defer func() { // the counter's GitHub repository, from the records too
		if rd.cnt != nil {
			c := s.withRepo(*rd.cnt, rd.recs)
			rd.cnt = &c
		}
	}()
	heads, err := s.Board.Store().Heads(ctx, bsync.Kind.Name())
	if err == nil && len(heads) == 0 {
		return rd
	}
	if err == nil {
		var b strings.Builder
		for _, h := range heads {
			b.WriteString(h.ID + " " + h.Version + "\n")
		}
		rd.sum = b.String()
		rd.recs, err = bsync.Records(ctx, s.Board, func(id kernel.ID, err error) {
			if notice != nil {
				notice("sync %s: %v", id, err)
			}
		})
	}
	if err != nil && notice != nil {
		notice("the sync records: %v; the issues show without their forges", err)
	}
	for _, recs := range rd.recs {
		for i, r := range recs {
			if r.Number != "" && !isNumber(r.Number) {
				if notice != nil {
					notice("sync %s: issue %s's forge number %q isn't one, left aside", r.Forge, r.Issue, r.Number)
				}
				recs[i].Number = ""
			}
		}
	}
	return rd
}

// forged sets where the issue is on forges, from its records, and its
// GitHub page: the first GitHub one's.
func (is *Issue) forged(recs []bsync.Record) {
	for _, r := range recs {
		n := r.Number
		if !isNumber(n) { // board data: anyone who can push refs/board/sync/* writes it
			n = lastNumber(r.URL)
		}
		f := Forge{Name: ForgeName(r.URL), URL: webURL(r.URL), Number: n, Sync: r.Forge}
		is.Forges = append(is.Forges, f)
		if f.Name == "GitHub" && is.GithubURL == "" {
			is.GithubURL = f.URL
		}
	}
}

// webURL is u when it is an absolute http(s) URL, else "": a record's URL
// travels through git, anyone who can push the board's refs writes it,
// and a client would make it a link (a javascript: one runs in the page).
func webURL(u string) string {
	p, err := url.Parse(u)
	if err != nil || p.Host == "" || (!strings.EqualFold(p.Scheme, "https") && !strings.EqualFold(p.Scheme, "http")) {
		return ""
	}
	return u
}

// ForgeName is the forge a URL is on, as people name it: GitHub, GitLab
// (a gitlab.* host), else the host.
func ForgeName(u string) string {
	parsed, err := url.Parse(u)
	if err != nil {
		return u
	}
	switch host := parsed.Hostname(); {
	case host == "github.com":
		return "GitHub"
	case strings.HasPrefix(host, "gitlab."):
		return "GitLab"
	default:
		return host
	}
}

// lastNumber is the issue number ending a forge URL, if it ends with one.
func lastNumber(u string) string {
	if last := path.Base(u); isNumber(last) {
		return last
	}
	return ""
}

// isNumber says s is a forge number: digits only, at least one. A number
// read from board data goes in file names (NNN-slug.md), branch names
// and searches; anything else in it is refused where it is read.
func isNumber(s string) bool {
	return s != "" && strings.Trim(s, "0123456789") == ""
}

// pending marks what hasn't reached the forge of r yet, from its record:
// a comment neither posted there nor pulled from it; a description or a
// comment there whose text git-board changed since (EditWas, the
// forge's); the latest title and status changes, and each label's, when
// git-board's value isn't the one last seen. A line comment never goes
// (isLocal), nor a label the sync holds off that forge (Held).
// ponytail: compares git-board's texts with the forge's as they are; one
// that came from another forge, whose #n and @ sync rewrites, shows as an
// edit waiting.
func (s *Service) pending(th *Thread, st *State, r bsync.Record, from map[int]*kernel.Origin, changed map[int][2][]string) {
	name := ForgeName(r.URL)
	_, key, _ := strings.Cut(r.URL, "://")
	held := func(l string) bool { return s.Held != nil && s.Held(key, l) }
	mark := func(i int) {
		if e := &th.Events[i]; !slices.Contains(e.Pending, name) {
			e.Pending = append(e.Pending, name)
		}
	}
	edit := func(e *Event, was string) {
		e.EditPending = append(e.EditPending, name)
		if e.EditWas == "" {
			e.EditWas = was
		}
	}
	title, status := -1, -1
	last := map[string]int{} // a label -> its last change's event
	for i := range th.Events {
		e := &th.Events[i]
		switch e.Kind {
		case "created":
			if was, ok := r.Seen["body"]; ok && was != st.Body {
				edit(e, was)
			}
		case "comment":
			if isLocal(e.Meta) {
				continue
			}
			fc := r.Posted[e.ID]
			if o := from[i]; fc == "" && o != nil && bsync.StateID(o.Forge) == r.Forge {
				fc = strings.TrimPrefix(o.Event, "comment:")
			}
			if fc == "" {
				mark(i)
			} else if was, ok := r.Seen["comment:"+fc]; ok && was != e.Body {
				edit(e, was)
			}
		case "title":
			title = i
		case "status":
			status = i
		case "labels":
			for _, l := range slices.Concat(changed[i][0], changed[i][1]) {
				last[l] = i
			}
		}
	}
	if title >= 0 && st.Title != r.Seen["title"] {
		mark(title)
	}
	if status >= 0 && string(st.Status) != r.Seen["status"] {
		mark(status)
	}
	seen := map[string]bool{}
	for l := range strings.SplitSeq(r.Seen["labels"], "\n") {
		seen[l] = l != ""
	}
	for l, i := range last {
		if !isAssigneeLabel(l) && !held(l) && slices.Contains(st.Labels, l) != seen[l] {
			mark(i)
		}
	}
}

// isLocal says a comment stays in git-board: a line comment, its review:*
// metadata.
func isLocal(meta map[string]string) bool {
	for k := range meta {
		if strings.HasPrefix(k, "review:") {
			return true
		}
	}
	return false
}

func isAssigneeLabel(l string) bool { return strings.HasPrefix(strings.ToLower(l), "assignee:") }
