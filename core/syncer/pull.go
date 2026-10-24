package syncer

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/guilbep/git-board/core/forge"
	"github.com/guilbep/git-board/core/gitstore"
	"github.com/guilbep/git-board/core/issue"
	"github.com/guilbep/git-board/core/oplog"
	"github.com/guilbep/git-board/core/people"
)

// authors finds the person of each forge account, writing it first when
// there is none or its login changed.
type authors struct {
	s       *gitstore.Store
	dir     *people.Directory
	persons map[string]*people.Person
}

// loadAuthors reads every person. One that fails to load stops the run:
// ownership can't be decided without every person.
func loadAuthors(s *gitstore.Store) (*authors, error) {
	dir, err := people.LoadDirectory(s)
	if err != nil {
		return nil, fmt.Errorf("syncer: people: %w", err)
	}
	ids, err := people.List(s)
	if err != nil {
		return nil, err
	}
	au := &authors{s: s, dir: dir, persons: map[string]*people.Person{}}
	for _, id := range ids {
		if au.persons[id], err = people.Load(s, id); err != nil {
			return nil, fmt.Errorf("syncer: people: %w", err)
		}
	}
	return au, nil
}

func (au *authors) byID(id string) *people.Person { return au.persons[id] }

// of gives the person id of each author of fi, the issue's and its
// comments', by forge account id.
func (au *authors) of(ctx context.Context, server string, fi forge.Issue) (map[string]string, error) {
	accounts := []forge.Account{fi.Author}
	for _, c := range fi.Comments {
		accounts = append(accounts, c.Author)
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

func (au *authors) person(ctx context.Context, server string, a forge.Account) (*people.Person, error) {
	acc := people.Account{Server: server, ID: a.ID, Login: a.Login}
	p, ok := au.dir.ForAccount(server, a.ID)
	var err error
	switch {
	case !ok:
		p, err = people.Create(ctx, au.s, people.NewCreate("", a.Login, ""), people.AddAccount("", acc))
	case !slices.Contains(p.Accounts, acc): // its login changed
		p, err = people.Do(ctx, au.s, p.ID, people.AddAccount(p.ID, acc))
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

// labelSet is the unique, sorted labels without assignee labels.
func labelSet(labels []string) []string {
	var out []string
	for _, l := range labels {
		if !isAssignee(l) {
			out = append(out, l)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
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
func from(op oplog.Op, key, event string, at int64) oplog.Op {
	op.Origin = &oplog.Origin{Forge: key, Event: event}
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
func (r *run) pullNew(p *project, fi forge.Issue) error {
	key, now := p.key, r.now
	who, err := r.au.of(r.ctx, p.f.Server(), fi)
	if err != nil {
		return err
	}
	a := who[fi.Author.ID]
	ev := "issue:" + fi.ID
	labels := strings.Join(labelSet(fi.Labels), "\n")
	add := split(labels)
	for _, as := range fi.Assignees {
		add = append(add, "assignee:"+as.Login)
	}
	ops := []oplog.Op{from(issue.NewCreate(a, fi.Title, fi.Body), key, ev, fi.Created)}
	if fi.Status == "closed" {
		ops = append(ops, from(issue.SetStatus(a, issue.Closed), key, ev, changedAt(fi, "status")))
	}
	if len(add) > 0 {
		ops = append(ops, from(issue.ChangeLabels(a, add, nil), key, ev, changedAt(fi, "labels")))
	}
	for _, c := range fi.Comments {
		ops = append(ops, from(issue.NewComment(who[c.Author.ID], c.Body), key, "comment:"+c.ID, c.Time))
	}
	is, err := issue.Create(r.ctx, r.s, ops...)
	if err != nil {
		return err
	}
	r.issues[is.ID] = is
	sops := []oplog.Op{
		linkOp(a, now, is.ID, fi.ID, fi.Number, fi.URL),
		seenOp(a, now, is.ID, "title", fi.Title),
		seenOp(a, now, is.ID, "body", fi.Body),
		seenOp(a, now, is.ID, "status", fi.Status),
		seenOp(a, now, is.ID, "labels", labels),
	}
	if len(fi.Assignees) > 0 {
		sops = append(sops, seenOp(a, now, is.ID, "assignees", ids(fi.Assignees)))
	}
	for _, c := range fi.Comments {
		sops = append(sops, seenOp(a, now, is.ID, "comment:"+c.ID, c.Body))
	}
	return p.st.write(r.ctx, r.s, key, sops)
}

// reconcile compares an issue with its forge issue, field by field, and
// writes what sync.md's four cases say: pulled operations in one pack,
// then the sync entity's, then each push, recorded as it lands. pull and
// push say which of the two it may do. It says whether it wrote anything,
// and whether it wrote to the forge.
func (r *run) reconcile(p *project, id string, fi forge.Issue, pull, push bool) (wrote, pushed bool, err error) {
	is, err := issue.Load(r.s, id)
	if err != nil {
		if isMissing(err) {
			return false, false, nil // the issue may arrive with a fetch, or isn't one
		}
		return false, false, err
	}
	r.issues[id] = is
	who, err := r.au.of(r.ctx, p.f.Server(), fi)
	if err != nil {
		return false, false, err
	}
	pl, err := r.plan(p, is, fi, who, pull, push)
	if err != nil {
		return false, false, err
	}
	if len(pl.ops) > 0 {
		if is, err = issue.Do(r.ctx, r.s, id, pl.ops...); err != nil {
			return false, false, err
		}
		r.issues[id] = is
	}
	if err := p.st.write(r.ctx, r.s, p.key, pl.sops); err != nil {
		return false, false, err
	}
	x := &pushing{p: p, is: is, fi: fi}
	for _, send := range pl.pushes() {
		if x.gone {
			break
		}
		if err := send(x); err != nil {
			return false, x.wrote, err
		}
	}
	return len(pl.ops)+len(pl.sops) > 0 || x.wrote, x.wrote, nil
}

// isMissing says a link names no issue here: none yet, or an id that names none.
func isMissing(err error) bool {
	return errors.Is(err, oplog.ErrNotFound) || errors.Is(err, oplog.ErrBadName)
}
