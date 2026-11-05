package issuesync

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/guilbep/git-board/next/forge"
	"github.com/guilbep/git-board/next/kernel"
)

// authors finds the person of each forge account, writing it first when
// there is none or its login changed.
type authors struct {
	s       *kernel.Board
	dir     *Directory
	persons map[string]*Person
}

// loadAuthors reads every person. One that fails to load stops the run:
// ownership can't be decided without every person.
func loadAuthors(s *kernel.Board) (*authors, error) {
	ids, err := ListPersons(s)
	if err != nil {
		return nil, err
	}
	au := &authors{s: s, dir: personNewDirectory(), persons: map[string]*Person{}}
	for _, id := range ids {
		p, err := LoadPerson(s, id)
		if err != nil {
			return nil, fmt.Errorf("syncer: people: %w", err)
		}
		au.persons[id] = p
		au.dir.Put(p)
	}
	return au, nil
}

func (au *authors) byID(id string) *Person { return au.persons[id] }

// of gives the person id of each author of fi, the issue's and its
// comments', by forge account id. With assignees, it also gives a person
// to each of fi's assignees, so the assignee:<login> label pulled for one
// names someone (sync.md, "Assignees").
func (au *authors) of(ctx context.Context, server string, fi forge.Issue, assignees bool) (map[string]string, error) {
	accounts := []forge.Account{fi.Author}
	for _, c := range fi.Comments {
		accounts = append(accounts, c.Author)
	}
	if assignees {
		accounts = append(accounts, fi.Assignees...)
	}
	who := map[string]string{}
	for _, a := range accounts {
		p, err := au.person(ctx, server, a)
		if err != nil {
			return nil, fmt.Errorf("syncer: forge user %s (%s): %w", a.ID, a.Login, err)
		}
		who[a.ID] = p.ID
	}
	return who, nil
}

func (au *authors) person(ctx context.Context, server string, a forge.Account) (*Person, error) {
	acc := Account{Server: server, ID: a.ID, Login: a.Login}
	p, ok := au.dir.ForAccount(server, a.ID)
	var err error
	switch {
	case !ok:
		p, err = CreatePerson(ctx, au.s, NewPerson("", a.Login, ""), AddAccount("", acc))
	case !slices.Contains(p.Accounts, acc): // its login changed
		p, err = DoPerson(ctx, au.s, p.ID, AddAccount(p.ID, acc))
	default:
		return p, nil
	}
	if err != nil {
		return nil, err
	}
	au.dir.Put(p)
	au.persons[p.ID] = p
	return p, nil
}

// isAssignee says whether a label is an assignee label: the prefix
// assignee:, in any case, and a name.
func isAssignee(l string) bool {
	return len(l) > len("assignee:") && strings.EqualFold(l[:len("assignee:")], "assignee:")
}

// labelSet is the unique, sorted labels without assignee labels, and
// without the labels p holds.
func (r *run) labelSet(p *project, labels []string) []string {
	var out []string
	for _, l := range labels {
		if !isAssignee(l) && !r.held(p, l) {
			out = append(out, l)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// pullsAssignees says whether p's forge assignees are the assignees here
// (Options.Assignees).
func (r *run) pullsAssignees(p *project) bool {
	return r.opt.Assignees == nil || r.opt.Assignees(p.key)
}

// held says whether p holds the label (Options.Held).
func (r *run) held(p *project, l string) bool {
	return r.opt.Held != nil && r.opt.Held(p.key, l)
}

func split(v string) []string {
	if v == "" {
		return nil
	}
	return strings.Split(v, "\n")
}

// from stamps an operation with the forge's origin and the forge's time of
// the change, so the later change wins by when it was made, not pulled. A
// forge that gives no time gets the run's.
func from(op kernel.Op, key, event string, at int64) kernel.Op {
	op.Origin = &kernel.Origin{Forge: key, Event: event}
	op.Time = at
	if at == 0 {
		op.Time = time.Now().Unix()
	}
	return op
}

// ids is the sorted, unique ids of accounts, one per line.
func ids(accounts []forge.Account) string {
	var out []string
	for _, a := range accounts {
		out = append(out, a.ID)
	}
	slices.Sort(out)
	return strings.Join(slices.Compact(out), "\n")
}

// pullNew makes an issue from a forge issue, and links it.
func (r *run) pullNew(p *project, fi forge.Issue) (string, error) {
	key, now := p.key, r.now
	who, err := r.au.of(r.ctx, p.f.Server(), fi, r.pullsAssignees(p))
	if err != nil {
		return "", err
	}
	a := who[fi.Author.ID]
	ev := "issue:" + fi.ID
	labels := strings.Join(r.labelSet(p, fi.Labels), "\n")
	add := split(labels)
	// the assignees live where their field's mapping says: with another
	// home on this forge, the forge's own assignees are no one's to pull
	if r.pullsAssignees(p) {
		for _, as := range fi.Assignees {
			add = append(add, "assignee:"+as.Login)
		}
	}
	ops := []kernel.Op{from(NewCreate(a, fi.Title, plain(fi.Body)), key, ev, fi.Created)}
	if fi.Status == "closed" {
		ops = append(ops, from(SetStatus(a, Closed), key, ev, changedAt(fi, "status")))
	}
	if len(add) > 0 {
		ops = append(ops, from(ChangeLabels(a, add, nil), key, ev, changedAt(fi, "labels")))
	}
	for _, c := range fi.Comments {
		ops = append(ops, from(NewComment(who[c.Author.ID], plain(c.Body)), key, "comment:"+c.ID, c.Time))
	}
	is, err := CreateIssue(r.ctx, r.s, ops...)
	if err != nil {
		return "", err
	}
	r.issues[is.ID] = is
	sops := []kernel.Op{
		linkOp(a, now, is.ID, fi.ID, fi.Number, fi.URL),
		seenOp(a, now, is.ID, "title", fi.Title),
		seenOp(a, now, is.ID, "body", fi.Body),
		seenOp(a, now, is.ID, "status", fi.Status),
		seenOp(a, now, is.ID, "labels", labels),
	}
	if len(fi.Assignees) > 0 && r.pullsAssignees(p) {
		sops = append(sops, seenOp(a, now, is.ID, "assignees", ids(fi.Assignees)))
	}
	for _, c := range fi.Comments {
		sops = append(sops, seenOp(a, now, is.ID, "comment:"+c.ID, c.Body))
	}
	if err := p.st.write(r.ctx, r.s, key, sops); err != nil {
		return "", err
	}
	r.did(key, "created", is.ID, len(ops))
	return is.ID, nil
}

// reconcile compares an issue with its forge issue, field by field, and
// writes what sync.md's four cases say: pulled operations in one pack,
// then the sync entity's, then each push, recorded as it lands. pull and
// push say which of the two it may do. It says whether it wrote anything,
// whether it wrote to the forge, and how many operations it pulled.
func (r *run) reconcile(p *project, id string, fi forge.Issue, pull, push bool) (wrote, pushed bool, pulled int, err error) {
	is, err := LoadIssue(r.s, id)
	if err != nil {
		if isMissing(err) {
			return false, false, 0, nil // the issue may arrive with a fetch, or isn't one
		}
		return false, false, 0, err
	}
	r.issues[id] = is
	who, err := r.au.of(r.ctx, p.f.Server(), fi, r.pullsAssignees(p))
	if err != nil {
		return false, false, 0, err
	}
	pl, err := r.plan(p, is, fi, who, pull, push)
	if err != nil {
		return false, false, 0, err
	}
	if len(pl.ops) > 0 {
		// planned again on the head the pull is written on: a local change
		// that landed since the read above is kept (sync.md, The four cases)
		if is, err = DecideIssue(r.ctx, r.s, id, func(cur *Issue) ([]kernel.Op, error) {
			if pl, err = r.plan(p, cur, fi, who, pull, push); err != nil {
				return nil, err
			}
			return pl.ops, nil
		}); err != nil {
			return false, false, 0, err
		}
		r.issues[id] = is
	}
	if err := p.st.write(r.ctx, r.s, p.key, pl.sops); err != nil {
		return false, false, len(pl.ops), err
	}
	x := &pushing{p: p, is: is, fi: fi}
	for _, send := range pl.pushes() {
		if x.gone {
			break
		}
		if err := send(x); err != nil {
			return false, x.wrote, len(pl.ops), err
		}
	}
	return len(pl.ops)+len(pl.sops) > 0 || x.wrote, x.wrote, len(pl.ops), nil
}

// isMissing says a link names no issue here: none yet, or an id that names none.
func isMissing(err error) bool {
	return errors.Is(err, kernel.ErrNotFound)
}
