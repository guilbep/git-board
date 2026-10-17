package inbox

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/filter"
	"github.com/guilbep/git-board/internal/gitbugio"
)

// Window is how far back the inbox looks: issues updated since, events
// dated since.
// ponytail: a change arriving more than 14 days late never shows; a
// per-issue "arrived" record if that ever bites.
const Window = 14 * 24 * time.Hour

// Me is who the inbox is for: the identity git-board writes as, and its
// forge login.
type Me struct {
	ID    string `json:"id"`
	Login string `json:"login"`
}

// Is reports whether p is you: your identity, or any carrying your login
// (old bridges and the sync can leave several for one person).
func (m Me) Is(p gitbugio.Person) bool {
	return p.ID == m.ID || (m.Login != "" && strings.EqualFold(p.Login, m.Login))
}

// Row is one issue with something new for you.
type Row struct {
	Issue   gitbugio.Issue    `json:"issue"`
	Reasons []string          `json:"reasons"` // why you follow it; none: addressed to you only
	News    []string          `json:"news"`    // "2 comments", "closed", "added you as reviewer"…
	Who     []gitbugio.Person `json:"who"`
	Latest  time.Time         `json:"latest"`
	First   string            `json:"first"` // the oldest of those events: the panel scrolls there
}

// Follows is whether you follow th, why, and what you clicked (nil:
// automatic). views are the followed views th is in.
func Follows(me Me, st State, th gitbugio.Thread, views []string, cfg *config.Config) (bool, []string, *bool) {
	reasons := filter.Involvement(involved(me, th), me.Login, cfg)
	set := st.Set(th.ID)
	if set != nil && *set {
		reasons = append(reasons, "followed")
	}
	for _, v := range views {
		reasons = append(reasons, "view: "+v)
	}
	following := len(reasons) > 0 && (set == nil || *set)
	return following, reasons, set
}

// involved reads th as filter.Involvement reads a listed issue; you, by
// identity or by login, count as your login.
func involved(me Me, th gitbugio.Thread) filter.Involved {
	login := func(p gitbugio.Person) string {
		if me.Is(p) {
			return me.Login
		}
		return p.Login
	}
	in := filter.Involved{AuthorLogins: []string{login(th.Author)}, Labels: th.Labels}
	for _, e := range th.Events {
		if e.Kind == "comment" && e.Author.ID != th.Author.ID {
			in.CommenterLogins = append(in.CommenterLogins, login(e.Author))
		}
		if (e.Kind == "created" || e.Kind == "comment") && gitbugio.Mentions(e.Body, me.Login) {
			in.Mentioned = true
		}
	}
	return in
}

// Rows is what happened that concerns you on threads, newest first:
// events dated since, unseen, not yours. inViews are the followed views
// each issue is in, by id.
func Rows(me Me, st State, threads []gitbugio.Thread, inViews map[string][]string, cfg *config.Config, since time.Time) []Row {
	var rows []Row
	for _, th := range threads {
		if r, ok := rowOf(me, st, th, inViews[th.ID], cfg, since); ok {
			rows = append(rows, r)
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Latest.After(rows[j].Latest) })
	return rows
}

func rowOf(me Me, st State, th gitbugio.Thread, views []string, cfg *config.Config, since time.Time) (Row, bool) {
	following, reasons, _ := Follows(me, st, th, views, cfg)
	inField := slices.ContainsFunc(th.Labels, func(l string) bool {
		_, ok := filter.PersonField(l, me.Login, cfg)
		return ok
	})
	seen := map[string]bool{}
	for _, id := range st.Seen[th.ID] {
		seen[id] = true
	}

	row := Row{Issue: th.Issue, Reasons: reasons}
	var comments, changes int
	var status string
	var notes []string // once each: new issue, added you as…, removed you as…, mentioned you
	note := func(s string) {
		if !slices.Contains(notes, s) {
			notes = append(notes, s)
		}
	}
	for _, e := range th.Events {
		if e.At.Before(since) || seen[e.ID] || me.Is(e.Author) {
			continue
		}
		addressed := false // to you: shows followed or not, unfollowed included
		for _, l := range e.Added {
			if f, ok := filter.PersonField(l, me.Login, cfg); ok {
				note("added you as " + f)
				addressed = true
			}
		}
		for _, l := range e.Removed {
			if f, ok := filter.PersonField(l, me.Login, cfg); ok {
				note("removed you as " + f)
				addressed = true
			}
		}
		if (e.Kind == "created" || e.Kind == "comment") && gitbugio.Mentions(e.Body, me.Login) {
			note("mentioned you")
			addressed = true
		}
		counted := addressed
		if following {
			switch e.Kind {
			case "comment":
				comments++
				counted = true
			case "created":
				note("new issue")
				counted = true
			case "status":
				status = e.Status
				counted = true
			case "labels":
				if st.Fields && inField && !addressed {
					changes++
					counted = true
				}
			}
		}
		if !counted {
			continue
		}
		if row.First == "" {
			row.First = e.ID
		}
		if e.At.After(row.Latest) {
			row.Latest = e.At
		}
		if !slices.ContainsFunc(row.Who, func(p gitbugio.Person) bool { return p.ID == e.Author.ID }) {
			row.Who = append(row.Who, e.Author)
		}
	}
	if row.First == "" {
		return Row{}, false
	}
	if comments > 0 {
		row.News = append(row.News, plural(comments, "comment"))
	}
	switch status {
	case "closed":
		row.News = append(row.News, "closed")
	case "open":
		row.News = append(row.News, "reopened")
	}
	row.News = append(row.News, notes...)
	if changes > 0 {
		row.News = append(row.News, plural(changes, "field change"))
	}
	return row, true
}

func plural(n int, what string) string {
	if n == 1 {
		return "1 " + what
	}
	return fmt.Sprintf("%d %ss", n, what)
}
