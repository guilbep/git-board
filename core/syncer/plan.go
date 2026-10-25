package syncer

import (
	"errors"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/guilbep/git-board/core/forge"
	"github.com/guilbep/git-board/core/issue"
	"github.com/guilbep/git-board/core/oplog"
)

// plan is what reconciling an issue with its forge issue writes: the
// issue's pulled operations, in one pack; then the sync entity's, for what
// agrees, was pulled or was found; then the pushes, in sync.md's order.
type plan struct {
	ops, sops            []oplog.Op
	text, comments, rest []func(*pushing) error
}

func (pl *plan) pushes() []func(*pushing) error { return slices.Concat(pl.text, pl.comments, pl.rest) }

// pushing is one issue's push under way. gone stops it: the forge issue is
// deleted, or confidential.
type pushing struct {
	p           *project
	is          *issue.Issue
	fi          forge.Issue
	gone, wrote bool
}

var errNoAccount = errors.New("no account there")

// verdict is what the four cases say for one field.
type verdict int

const (
	agreed verdict = iota
	pullIt
	pushIt
)

// decide compares git-board's value g and the forge's f with last seen s.
// took says git-board took its value from this forge: it counts as last
// seen. Both changed, the later wins, the forge's at the same second.
func decide(g, f, s string, took bool, gt, ft int64) verdict {
	if took {
		s = g
	}
	switch {
	case g == f:
		return agreed
	case f == s:
		return pushIt
	case g == s:
		return pullIt
	case gt > ft:
		return pushIt
	default:
		return pullIt
	}
}

// changedAt is the forge's time of a field's last change.
func changedAt(fi forge.Issue, field string) int64 {
	if t := fi.Changed[field]; t != 0 {
		return t
	}
	return fi.Updated
}

// wrapLike gives a text the header and marker last seen had, if sync
// posted it, or the fork's start, if the fork did: the fork's copies end
// without spaces or blank lines, so a text in its form does too.
func wrapLike(seen, body string, posted bool) string {
	w := unwrap(seen)
	switch {
	case w.forked():
		body = strings.TrimRight(body, " \t\r\n")
	case !posted || w.marker == "":
		return body
	}
	w.body = body
	return w.String()
}

// plain is a text as git-board keeps it: without the fork's start.
func plain(text string) string {
	if w := unwrap(text); w.forked() {
		return w.body
	}
	return text
}

// plan compares is with fi, as sync.md's four cases say. who gives the
// persons of fi's authors; it is nil for a virtual fi, which finds nothing.
func (r *run) plan(p *project, is *issue.Issue, fi forge.Issue, who map[string]string, pull, push bool) (*plan, error) {
	pl := &plan{}
	key, now := p.key, r.now
	seen := p.st.seen[is.ID]
	a := who[fi.Author.ID]
	ev := "issue:" + fi.ID
	rec := func(field, value string) oplog.Op { return seenOp(is.Author, time.Now().Unix(), is.ID, field, value) }
	refused := func(field, value string) bool { return p.st.refused[[3]string{is.ID, field, value}] }

	// text compares one text field. pullOp is the issue's operation taking
	// v, write the forge's.
	text := func(bucket *[]func(*pushing) error, field, g, f string, posted bool, ch issue.Change, ft int64,
		guard []*oplog.Origin, pullOp func(v string) oplog.Op, write func(fw forge.Forge, v string) error) {
		switch decide(g, f, seen[field], origin(ch) == key, ch.Time, ft) {
		case agreed:
			if seen[field] != f {
				pl.sops = append(pl.sops, seenOp(is.Author, now, is.ID, field, f))
			}
		case pullIt:
			if !pull {
				return
			}
			v := f
			if w := unwrap(f); posted && w.marker != "" || w.forked() {
				v = w.body
			}
			pl.ops = append(pl.ops, pullOp(v))
			pl.sops = append(pl.sops, seenOp(is.Author, now, is.ID, field, f))
		case pushIt:
			if !push || r.blocked(p, append(guard, ch.Origin)...) || refused(field, g) {
				return
			}
			*bucket = append(*bucket, func(x *pushing) error {
				return r.send(x, ch.Author, field, []string{g}, func(fw forge.Forge) ([]oplog.Op, error) {
					return []oplog.Op{rec(field, g)}, write(fw, g)
				})
			})
		}
	}

	// An issue linked here whose create isn't from here was created here
	// by sync, with its marker.
	posted := is.Origin == nil || is.Origin.Forge != key
	ch := is.Changes
	text(&pl.text, "title", r.form(p, is.Title, origin(ch["title"]), true), fi.Title, false, ch["title"], changedAt(fi, "title"), nil,
		func(v string) oplog.Op { return from(issue.SetTitle(a, v), key, ev, changedAt(fi, "title")) },
		func(fw forge.Forge, v string) error { return fw.SetTitle(r.ctx, fi.ID, v) })
	text(&pl.text, "body", wrapLike(seen["body"], r.form(p, is.Body, origin(ch["body"]), false), posted), fi.Body, posted,
		ch["body"], changedAt(fi, "body"), nil,
		func(v string) oplog.Op { return from(issue.EditText(a, is.BodyOp, v), key, ev, changedAt(fi, "body")) },
		func(fw forge.Forge, v string) error { return fw.SetBody(r.ctx, fi.ID, v) })
	text(&pl.rest, "status", string(is.Status), fi.Status, false, ch["status"], changedAt(fi, "status"), nil,
		func(v string) oplog.Op {
			return from(issue.SetStatus(a, issue.Status(v)), key, ev, changedAt(fi, "status"))
		},
		func(fw forge.Forge, v string) error { return fw.SetStatus(r.ctx, fi.ID, v) })

	pullAdd, pullRemove := r.planLabels(pl, p, is, fi, pull, push)
	aAdd, aRemove := r.planAssignees(pl, p, is, fi, pull, push)
	pullAdd, pullRemove = append(pullAdd, aAdd...), append(pullRemove, aRemove...)
	if len(pullAdd) > 0 || len(pullRemove) > 0 {
		pl.ops = append(pl.ops, from(issue.ChangeLabels(a, pullAdd, pullRemove), key, ev, changedAt(fi, "labels")))
	}

	// Comments: each forge comment is a git-board comment it was pulled
	// into, or one sync posted, found by its record or its marker; else it
	// is new.
	matched := map[string]bool{}
	comment := func(id string) *issue.Comment {
		i := slices.IndexFunc(is.Comments, func(c issue.Comment) bool { return c.ID == id })
		if i < 0 {
			return nil
		}
		return &is.Comments[i]
	}
	for _, fc := range fi.Comments {
		field := "comment:" + fc.ID
		var c *issue.Comment
		sent := false
		if cid, ok := p.st.postedBy[fc.ID]; ok {
			if c, sent = comment(cid), true; c == nil {
				continue
			}
		} else if i := slices.IndexFunc(is.Comments, func(c issue.Comment) bool {
			return c.Origin != nil && *c.Origin == oplog.Origin{Forge: key, Event: field}
		}); i >= 0 {
			c = &is.Comments[i]
		} else if w := unwrap(fc.Body); who != nil && w.kind == "comment" && r.isSync(p, fc.Author) {
			// posted, then stopped before its record
			if c = comment(w.id); c == nil || matched[c.ID] || p.st.posted[[2]string{is.ID, c.ID}] != "" {
				continue
			}
			sent = true
			pl.sops = append(pl.sops, postedOp(is.Author, now, is.ID, c.ID, fc.ID), seenOp(is.Author, now, is.ID, field, fc.Body))
			seen = withSeen(seen, field, fc.Body)
		} else if who != nil {
			// a mirror the fork posted: git-board's copy of the original is
			// the forge's, recorded as posted. The original's text wins: last
			// seen is the forge's when it is the fork's copy, else git-board's.
			var copied bool
			if c, copied = r.mirrored(p, is, fc, matched); c != nil {
				v := fc.Body
				if !copied {
					v = r.form(p, c.Body, origin(ch["comment:"+c.ID]), false)
				}
				pl.sops = append(pl.sops, postedOp(is.Author, now, is.ID, c.ID, fc.ID), seenOp(is.Author, now, is.ID, field, v))
				seen = withSeen(seen, field, v)
			}
		}
		if c == nil {
			if pull {
				pl.ops = append(pl.ops, from(issue.NewComment(who[fc.Author.ID], plain(fc.Body)), key, field, fc.Time))
				pl.sops = append(pl.sops, seenOp(is.Author, now, is.ID, field, fc.Body))
			}
			continue
		}
		matched[c.ID] = true
		cch := ch["comment:"+c.ID]
		ft := fc.Time
		if fc.Edited != 0 {
			ft = fc.Edited
		}
		cid, fcID, ca := c.ID, fc.ID, who[fc.Author.ID]
		text(&pl.comments, field, wrapLike(seen[field], r.form(p, c.Body, origin(cch), false), sent), fc.Body, sent, cch, ft,
			[]*oplog.Origin{c.Origin},
			func(v string) oplog.Op { return from(issue.EditText(ca, cid, v), key, field, ft) },
			func(fw forge.Forge, v string) error { return fw.EditComment(r.ctx, fi.ID, fcID, v) })
	}
	if push {
		for _, c := range is.Comments {
			if matched[c.ID] || p.st.posted[[2]string{is.ID, c.ID}] != "" || (c.Origin != nil && c.Origin.Forge == key) ||
				(r.opt.Local != nil && r.opt.Local(c.Meta)) {
				continue
			}
			cch := ch["comment:"+c.ID]
			if r.blocked(p, c.Origin, cch.Origin) {
				continue
			}
			_, direct := r.writer(p, c.Author)
			w := wrapped{body: r.form(p, c.Body, origin(cch), false), marker: marker("comment", c.ID)}
			if !direct {
				w.header = r.header(c.Author, c.Time, c.Origin, is.ID)
			}
			body, field, cid := w.String(), "comment:"+c.ID, c.ID
			if refused(field, body) {
				continue
			}
			pl.comments = append(pl.comments, func(x *pushing) error {
				return r.send(x, c.Author, field, []string{body}, func(fw forge.Forge) ([]oplog.Op, error) {
					fc, err := fw.CreateComment(r.ctx, fi.ID, body)
					return []oplog.Op{postedOp(is.Author, time.Now().Unix(), is.ID, cid, fc.ID), rec("comment:"+fc.ID, body)}, err
				})
			})
		}
	}
	return pl, nil
}

// mirrored is the comment of is that fc mirrors, or that mirrors fc, as the
// fork posted them on two projects: neither from p nor sent there. fc is a
// copy when in the fork's form from sync's accounts (copied); the comment
// is one when its text was, on the forge it came from. A copy's header
// names its original exactly; failing that, the two have the same text.
func (r *run) mirrored(p *project, is *issue.Issue, fc forge.Comment, matched map[string]bool) (c *issue.Comment, copied bool) {
	w := unwrap(fc.Body)
	copied = w.forked() && r.isSync(p, fc.Author)
	host, orig := commentLinked(w.link)
	var byText *issue.Comment
	for i := range is.Comments {
		c := &is.Comments[i]
		if matched[c.ID] || p.st.posted[[2]string{is.ID, c.ID}] != "" || c.Origin != nil && c.Origin.Forge == p.key ||
			r.opt.Local != nil && r.opt.Local(c.Meta) {
			continue
		}
		cw, from := r.pulledFrom(is.ID, c)
		cHost, cOrig := commentLinked(cw.link)
		switch {
		case copied && orig != "" && from != nil && from.f.Server() == host && c.Origin.Event == "comment:"+orig:
			return c, true // fc is c's copy
		case !copied && cOrig != "" && cHost == p.f.Server() && cOrig == fc.ID:
			return c, false // c is fc's copy
		case byText == nil && (copied || cw.forked()) && same(fc.Body, c.Body):
			byText = c
		}
	}
	return byText, copied && byText != nil
}

// pulledFrom is a comment's text as last seen on the run's project it was
// pulled from, and that project; nil for a comment from none.
func (r *run) pulledFrom(issueID string, c *issue.Comment) (wrapped, *project) {
	if c.Origin == nil {
		return wrapped{}, nil
	}
	fid, ok := strings.CutPrefix(c.Origin.Event, "comment:")
	if !ok {
		return wrapped{}, nil
	}
	w := unwrap(r.stateOf(c.Origin.Forge).seen[issueID]["comment:"+fid])
	for _, q := range r.projects {
		if q.key == c.Origin.Forge {
			return w, q
		}
	}
	return w, nil
}

func withSeen(seen map[string]string, field, v string) map[string]string {
	out := map[string]string{field: v}
	for k, x := range seen {
		if k != field {
			out[k] = x
		}
	}
	return out
}

// set compares a set per item, as labels and assignees are: each item's g,
// f and s. took says git-board took the item from this forge.
type item struct {
	g, f, s, took bool
	ch            issue.Change
}

// sets runs the four cases on each item, in order. It returns the items to
// pull and to push, and last seen once the pulls and the agreed are in.
func sets(items map[string]*item, pull, push bool) (pulled, pushed []string, base []string) {
	for _, k := range slices.Sorted(maps.Keys(items)) {
		it := items[k]
		s := it.s
		if it.took {
			s = it.g
		}
		keep := it.s
		switch {
		case it.g == it.f:
			if pull {
				keep = it.f
			}
		case it.f == s:
			if push {
				pushed = append(pushed, k)
			}
		default:
			if pull {
				pulled = append(pulled, k)
				keep = it.f
			}
		}
		if keep {
			base = append(base, k)
		}
	}
	return pulled, pushed, base
}

func sign(on bool) string {
	if on {
		return "+"
	}
	return "-"
}

// planLabels compares the labels, each on its own. It returns the labels
// the pull adds and removes.
func (r *run) planLabels(pl *plan, p *project, is *issue.Issue, fi forge.Issue, pull, push bool) (add, remove []string) {
	seen := p.st.seen[is.ID]
	items := map[string]*item{}
	at := func(l string) *item {
		if items[l] == nil {
			ch := is.Changes["label:"+l]
			items[l] = &item{ch: ch, took: origin(ch) == p.key}
		}
		return items[l]
	}
	for _, l := range labelSet(is.Labels) {
		at(l).g = true
	}
	for _, l := range labelSet(fi.Labels) {
		at(l).f = true
	}
	for _, l := range split(seen["labels"]) {
		at(l).s = true
	}
	pulled, pushed, base := sets(items, pull, push)
	for _, l := range pulled {
		if items[l].f {
			add = append(add, l)
		} else {
			remove = append(remove, l)
		}
	}
	baseV := strings.Join(base, "\n")
	if baseV != seen["labels"] {
		pl.sops = append(pl.sops, seenOp(is.Author, r.now, is.ID, "labels", baseV))
	}
	var pAdd, pRemove, values []string
	var last issue.Change
	for _, l := range pushed {
		it := items[l]
		if r.blocked(p, it.ch.Origin) || p.st.refused[[3]string{is.ID, "labels", sign(it.g) + l}] {
			continue
		}
		if it.g {
			pAdd = append(pAdd, l)
		} else {
			pRemove = append(pRemove, l)
		}
		values = append(values, sign(it.g)+l)
		if it.ch.Time >= last.Time {
			last = it.ch
		}
	}
	if len(values) == 0 {
		return add, remove
	}
	pl.rest = append(pl.rest, func(x *pushing) error {
		next := slices.DeleteFunc(slices.Concat(base, pAdd), func(l string) bool { return slices.Contains(pRemove, l) })
		slices.Sort(next)
		return r.send(x, last.Author, "labels", values, func(fw forge.Forge) ([]oplog.Op, error) {
			return []oplog.Op{seenOp(is.Author, time.Now().Unix(), is.ID, "labels", strings.Join(slices.Compact(next), "\n"))},
				fw.ChangeLabels(r.ctx, fi.ID, pAdd, pRemove)
		})
	})
	return add, remove
}

// assignee is the account a name in an assignee label names on p's server:
// by login, without case, else by a person's name.
func (r *run) assignee(p *project, name string) (forge.Account, bool) {
	server := p.f.Server()
	for _, byName := range []bool{false, true} {
		for _, id := range slices.Sorted(maps.Keys(r.au.persons)) {
			pe := r.au.persons[id]
			for _, a := range pe.Accounts {
				if a.Server == server && (!byName && strings.EqualFold(a.Login, name) || byName && pe.Name == name) {
					return forge.Account{ID: a.ID, Login: a.Login}, true
				}
			}
		}
	}
	return forge.Account{}, false
}

// planAssignees compares the assignees, each on its own, by forge user id.
// It returns the assignee labels the pull adds and removes.
func (r *run) planAssignees(pl *plan, p *project, is *issue.Issue, fi forge.Issue, pull, push bool) (add, remove []string) {
	seen := p.st.seen[is.ID]
	items := map[string]*item{}
	accounts := map[string]forge.Account{}
	names := map[string][]string{} // id -> the labels naming it
	var unknown []string
	at := func(id string) *item {
		if items[id] == nil {
			items[id] = &item{}
		}
		return items[id]
	}
	for _, l := range is.Labels {
		if !isAssignee(l) {
			continue
		}
		acc, ok := r.assignee(p, l[len("assignee:"):])
		if !ok {
			unknown = append(unknown, l[len("assignee:"):])
			continue
		}
		at(acc.ID).g = true
		accounts[acc.ID], names[acc.ID] = acc, append(names[acc.ID], l)
	}
	for k, ch := range is.Changes { // the label that last named each id, present or not
		if l, ok := strings.CutPrefix(k, "label:"); ok && isAssignee(l) {
			if acc, ok := r.assignee(p, l[len("assignee:"):]); ok && ch.Time >= at(acc.ID).ch.Time {
				at(acc.ID).ch = ch
			}
		}
	}
	for _, as := range fi.Assignees {
		at(as.ID).f = true
		accounts[as.ID] = as
	}
	for _, id := range split(seen["assignees"]) {
		at(id).s = true
	}
	for id, it := range items {
		if !it.g && !it.f && !it.s {
			delete(items, id)
		}
		it.took = origin(it.ch) == p.key
	}
	pulled, pushed, base := sets(items, pull, push)
	for _, id := range pulled {
		if items[id].f {
			add = append(add, "assignee:"+accounts[id].Login)
		} else {
			remove = append(remove, names[id]...)
		}
	}
	if baseV := strings.Join(base, "\n"); baseV != seen["assignees"] {
		pl.sops = append(pl.sops, seenOp(is.Author, r.now, is.ID, "assignees", baseV))
	}
	if !push {
		return add, remove
	}
	for _, name := range unknown {
		if !p.st.refused[[3]string{is.ID, "assignees", "+" + name}] {
			pl.rest = append(pl.rest, func(x *pushing) error {
				return r.refuse(p, is, "assignees", "+"+name, errNoAccount)
			})
		}
	}
	var want, sent, values []string // want: the forge's set, plus git-board's adds, less its removes
	var last issue.Change
	for _, as := range fi.Assignees {
		want = append(want, as.ID)
	}
	value := func(id string) string {
		if items[id].g {
			return "+" + names[id][0][len("assignee:"):]
		}
		return "-" + accounts[id].Login
	}
	for _, id := range pushed {
		it := items[id]
		v := value(id)
		if r.blocked(p, it.ch.Origin) || p.st.refused[[3]string{is.ID, "assignees", v}] {
			continue
		}
		want = slices.DeleteFunc(want, func(x string) bool { return x == id })
		if it.g {
			want = append(want, id)
		}
		sent, values = append(sent, id), append(values, v)
		if it.ch.Time >= last.Time {
			last = it.ch
		}
	}
	if len(sent) == 0 {
		return add, remove
	}
	slices.Sort(want)
	pl.rest = append(pl.rest, func(x *pushing) error {
		var users []forge.Account
		for _, id := range want {
			users = append(users, accounts[id])
		}
		var kept []string
		landed := false
		err := r.send(x, last.Author, "assignees", values, func(fw forge.Forge) ([]oplog.Op, error) {
			got, err := fw.SetAssignees(r.ctx, fi.ID, users)
			if err != nil {
				return nil, err
			}
			landed, kept = true, split(ids(got))
			next := slices.DeleteFunc(slices.Clone(base), func(id string) bool { return slices.Contains(sent, id) })
			for _, id := range sent {
				if slices.Contains(kept, id) {
					next = append(next, id)
				}
			}
			slices.Sort(next)
			return []oplog.Op{seenOp(is.Author, time.Now().Unix(), is.ID, "assignees", strings.Join(next, "\n"))}, nil
		})
		if err != nil || !landed {
			return err
		}
		for _, id := range sent {
			if items[id].g != slices.Contains(kept, id) {
				if err := r.refuse(p, is, "assignees", value(id), errors.New("the forge didn't keep it")); err != nil {
					return err
				}
			}
		}
		return nil
	})
	return add, remove
}

// send makes one write as person, then records it. A refusal is recorded
// and warned once; an issue gone or confidential stops the issue's push.
// "Not found" counts as gone only when the posting account gets it too,
// and as refused only when that account reads the issue.
func (r *run) send(x *pushing, person, field string, values []string, write func(forge.Forge) ([]oplog.Op, error)) error {
	if x.gone {
		return nil
	}
	fw, _ := r.writer(x.p, person)
	record, err := write(fw)
	switch {
	case err == nil:
		x.wrote = true
		return x.p.st.write(r.ctx, r.s, x.p.key, record)
	case errors.Is(err, forge.ErrConfidential):
		x.gone = true
		return nil
	case errors.Is(err, forge.ErrNotFound):
		_, e := x.p.f.Issue(r.ctx, x.fi.ID)
		if errors.Is(e, forge.ErrNotFound) || errors.Is(e, forge.ErrConfidential) {
			x.gone = true
			return nil
		}
		if e != nil {
			return e // the posting account's read failed: nothing is known yet
		}
		fallthrough
	case errors.Is(err, forge.ErrRefused):
		for _, v := range values {
			if err := r.refuse(x.p, x.is, field, v, err); err != nil {
				return err
			}
		}
		return nil
	default:
		return err
	}
}
