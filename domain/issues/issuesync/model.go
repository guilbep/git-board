// Package issuesync syncs issues with forge projects (docs/spec/sync.md):
// the issue mapper and the run, v1's syncer (core/syncer) ported onto
// v2's kernel, the issue domain and the people domain. The records, the
// texts' forms, the lock and the journal are the kind-free engine's
// (sync).
//
// The run reads issues and persons as v1's did, as plain values keyed by
// string ids (Issue, Person): this file turns v2's states into them, and
// writes through the kernel.
// ponytail: the issue mapper is the run itself, as in v1; the Mapper
// interface of design.md comes with the second kind (releases).
package issuesync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/guilbep/git-board/domain/issues"
	"github.com/guilbep/git-board/domain/people"
	"github.com/guilbep/git-board/kernel"
	bsync "github.com/guilbep/git-board/sync"
)

// Status is an issue's status.
type Status = issues.Status

// The statuses.
const (
	Open   = issues.Open
	Closed = issues.Closed
)

// Comment is one comment, as the run reads it.
type Comment struct {
	ID, Author, Body string
	Time, Edited     int64
	Origin           *kernel.Origin
	Meta             map[string]string
}

// Change is who last changed a field, when, and from where.
type Change struct {
	Author string
	Time   int64
	Origin *kernel.Origin
}

// Issue is an issue's state, as the run reads it.
type Issue struct {
	ID, Author, Title, Body, BodyOp string
	Created, Updated                int64
	Status                          Status
	Labels                          []string
	Comments                        []Comment
	Origin                          *kernel.Origin
	Meta                            map[string]string
	Changes                         map[string]Change
	Deleted                         bool
	Removed                         []Comment
}

// IsRemoved says a delete hid the comment with this id.
func (is *Issue) IsRemoved(id string) bool {
	return slices.ContainsFunc(is.Removed, func(c Comment) bool { return c.ID == id })
}

func commentOf(c issues.Comment) Comment {
	return Comment{ID: string(c.ID), Author: string(c.Author), Body: c.Body, Time: c.Time, Edited: c.Edited, Origin: c.Origin, Meta: c.Meta}
}

func issueOf(st *issues.State) *Issue {
	is := &Issue{ID: string(st.ID), Author: string(st.Author), Title: st.Title, Body: st.Body, BodyOp: string(st.BodyOp),
		Created: st.Created, Updated: st.Updated, Status: st.Status, Labels: st.Labels, Origin: st.Origin, Meta: st.Meta,
		Changes: make(map[string]Change, len(st.Changes)), Deleted: st.Deleted}
	for _, c := range st.Comments {
		is.Comments = append(is.Comments, commentOf(c))
	}
	for _, c := range st.Removed {
		is.Removed = append(is.Removed, commentOf(c))
	}
	for k, c := range st.Changes {
		is.Changes[k] = Change{Author: string(c.Author), Time: c.Time, Origin: c.Origin}
	}
	return is
}

// newOp makes an operation made now.
func newOp(kind, author string, d any) kernel.Op {
	raw, _ := json.Marshal(d)
	return kernel.Op{Kind: kind, Author: author, Time: time.Now().Unix(), Data: raw}
}

type m = map[string]any

func NewCreate(author, title, body string) kernel.Op {
	return newOp("create", author, m{"title": title, "body": body})
}

func NewComment(author, body string) kernel.Op {
	return newOp("comment", author, m{"body": body})
}

func NewCommentMeta(author, body string, meta map[string]string) kernel.Op {
	return newOp("comment", author, m{"body": body, "meta": meta})
}

func SetMeta(author, target string, meta map[string]string) kernel.Op {
	return newOp("set-meta", author, m{"target": target, "meta": meta})
}

func EditText(author, target, body string) kernel.Op {
	return newOp("edit-text", author, m{"target": target, "body": body})
}

func SetTitle(author, title string) kernel.Op {
	return newOp("set-title", author, m{"title": title})
}

func SetStatus(author string, s Status) kernel.Op {
	return newOp("set-status", author, m{"status": s})
}

func ChangeLabels(author string, add, remove []string) kernel.Op {
	return newOp("change-labels", author, m{"add": add, "remove": remove})
}

// ListIssues is every issue's id, sorted.
func ListIssues(b *kernel.Board) ([]string, error) {
	heads, err := b.Store().Heads(context.Background(), issues.Kind.Name())
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(heads))
	for i, h := range heads {
		ids[i] = h.ID
	}
	slices.Sort(ids)
	return ids, nil
}

func LoadIssue(b *kernel.Board, id string) (*Issue, error) {
	e, err := issues.Kind.Get(context.Background(), b, kernel.ID(id))
	if err != nil {
		return nil, err
	}
	return issueOf(&e.State), nil
}

// issueFold is the issue a part of its log adds up to.
func issueFold(id string, log []kernel.Entry) (*Issue, error) {
	st, err := issues.Kind.Fold(kernel.ID(id), log)
	if err != nil {
		return nil, err
	}
	return issueOf(&st), nil
}

func CreateIssue(ctx context.Context, b *kernel.Board, ops ...kernel.Op) (*Issue, error) {
	if len(ops) == 0 || ops[0].Kind != "create" {
		return nil, errors.New("issue: the first operation must be a create")
	}
	e, err := issues.Kind.Create(ctx, b, "", ops...)
	if err != nil {
		return nil, err
	}
	return issueOf(&e.State), nil
}

func DoIssue(ctx context.Context, b *kernel.Board, id string, ops ...kernel.Op) (*Issue, error) {
	return DecideIssue(ctx, b, id, func(*Issue) ([]kernel.Op, error) { return ops, nil })
}

// DecideIssue writes the operations decide gives for the issue as it is
// at the head they are written on, asked again when it moved meanwhile
// (kernel's Do), and answers the issue after them.
func DecideIssue(ctx context.Context, b *kernel.Board, id string, decide func(cur *Issue) ([]kernel.Op, error)) (*Issue, error) {
	e, err := issues.Kind.Do(ctx, b, kernel.ID(id), func(cur kernel.Entity[issues.State]) ([]kernel.Op, error) {
		return decide(issueOf(&cur.State))
	})
	if err != nil {
		return nil, err
	}
	return issueOf(&e.State), nil
}

// Account is one user on one forge server.
type Account = people.Account

// Person is a person, as the run reads it.
type Person struct {
	ID, Name, Email string
	Accounts        []Account
}

func personOf(p people.Person) *Person {
	return &Person{ID: string(p.ID), Name: p.Name, Email: p.Email, Accounts: p.Accounts}
}

func NewPerson(author, name, email string) kernel.Op {
	return newOp("create", author, m{"name": name, "email": email})
}

func AddAccount(author string, a Account) kernel.Op {
	return newOp("add-account", author, m{"server": a.Server, "id": a.ID, "login": a.Login})
}

func ListPersons(b *kernel.Board) ([]string, error) {
	heads, err := b.Store().Heads(context.Background(), people.Kind.Name())
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(heads))
	for i, h := range heads {
		ids[i] = h.ID
	}
	slices.Sort(ids)
	return ids, nil
}

func LoadPerson(b *kernel.Board, id string) (*Person, error) {
	e, err := people.Kind.Get(context.Background(), b, kernel.ID(id))
	if err != nil {
		return nil, err
	}
	return personOf(e.State), nil
}

func CreatePerson(ctx context.Context, b *kernel.Board, ops ...kernel.Op) (*Person, error) {
	if len(ops) == 0 || ops[0].Kind != "create" {
		return nil, errors.New("people: the first operation must be a create")
	}
	e, err := people.Kind.Create(ctx, b, "", ops...)
	if err != nil {
		return nil, err
	}
	return personOf(e.State), nil
}

func DoPerson(ctx context.Context, b *kernel.Board, id string, ops ...kernel.Op) (*Person, error) {
	e, err := people.Kind.Do(ctx, b, kernel.ID(id), func(kernel.Entity[people.Person]) ([]kernel.Op, error) { return ops, nil })
	if err != nil {
		return nil, err
	}
	return personOf(e.State), nil
}

// Directory finds the person owning an account: of two, the smaller id
// (data-model.md, People).
type Directory struct{ owner map[[2]string]*Person }

func personNewDirectory() *Directory { return &Directory{owner: map[[2]string]*Person{}} }

// Put takes in a person made or changed since the directory loaded. It
// owns each of its accounts that no lower id holds.
func (d *Directory) Put(p *Person) {
	for _, a := range p.Accounts {
		k := [2]string{a.Server, a.ID}
		if o, ok := d.owner[k]; !ok || p.ID <= o.ID {
			d.owner[k] = p
		}
	}
}

// ForAccount is the person owning an account.
func (d *Directory) ForAccount(server, id string) (*Person, bool) {
	p, ok := d.owner[[2]string{server, id}]
	return p, ok
}

// state is a sync entity as a run reads and writes it: its fold, and
// whether it exists and the run wrote to it.
type state struct {
	bsync.State
	exists bool
	wrote  bool
	// the fold's fields, as the run names them
	links    map[string]string
	link     map[string]bsync.LinkData
	numbers  map[string]string
	anyLink  map[string]bool
	seen     map[string]map[string]string
	posted   map[[2]string]string
	postedBy map[string]string
	refused  map[[3]string]bool
	lastRun  int64
}

func (st *state) set(s bsync.State) {
	st.State = s
	st.links, st.link, st.numbers, st.anyLink = s.Links, s.Link, s.Numbers, s.AnyLink
	st.seen, st.posted, st.postedBy, st.refused, st.lastRun = s.Seen, s.Posted, s.PostedBy, s.Refused, s.LastRun
}

func emptyState() *state {
	st := &state{}
	st.set(bsync.New(""))
	return st
}

func loadState(b *kernel.Board, key string) (*state, error) {
	st := emptyState()
	e, err := bsync.Kind.Get(context.Background(), b, bsync.StateID(key))
	if errors.Is(err, kernel.ErrNotFound) {
		return st, nil
	}
	if err != nil {
		return nil, err
	}
	st.set(e.State)
	st.exists = true
	return st, nil
}

// write appends ops to the entity, creating it if needed, and folds
// them. It refuses an op with no author, which would break the entity.
func (st *state) write(ctx context.Context, b *kernel.Board, key string, ops []kernel.Op) error {
	if len(ops) == 0 {
		return nil
	}
	for i, op := range ops {
		if op.Author == "" {
			return fmt.Errorf("syncer: operation %d has no author", i)
		}
	}
	id := bsync.StateID(key)
	e, err := writeOps(ctx, b, id, st.exists, ops)
	if err != nil {
		return err
	}
	st.set(e.State)
	st.exists, st.wrote = true, true
	return nil
}

// writeOps appends ops to the sync entity id, or creates it with them
// when it doesn't exist. A create that finds the entity made by another
// write since appends to it instead; not there yet, the other write still
// holds the ref's lock, so it creates again. It tries 5 times, as the
// kernel's writes do.
func writeOps(ctx context.Context, b *kernel.Board, id kernel.ID, exists bool, ops []kernel.Op) (kernel.Entity[bsync.State], error) {
	for try := 1; ; try++ {
		if exists {
			return bsync.Kind.Do(ctx, b, id, func(kernel.Entity[bsync.State]) ([]kernel.Op, error) { return ops, nil })
		}
		e, err := bsync.Kind.Create(ctx, b, id, ops...)
		if err == nil || !errors.Is(err, kernel.ErrMoved) || try == 5 {
			return e, err
		}
		_, err = bsync.Kind.Get(ctx, b, id)
		if err != nil && !errors.Is(err, kernel.ErrNotFound) {
			return e, err
		}
		exists = err == nil
	}
}

func syncOp(kind, author string, now int64, d any) kernel.Op {
	raw, _ := json.Marshal(d)
	return kernel.Op{Kind: kind, Author: author, Time: now, Data: raw}
}

func linkOp(author string, now int64, issue, forgeID, number, url string) kernel.Op {
	return syncOp("link", author, now, bsync.LinkData{Issue: issue, ForgeID: forgeID, Number: number, URL: url})
}

func seenOp(author string, now int64, issue, field, value string) kernel.Op {
	return syncOp("seen", author, now, bsync.SeenData{Issue: issue, Field: field, Value: value})
}

func postedOp(author string, now int64, issue, comment, forgeID string) kernel.Op {
	return syncOp("posted", author, now, bsync.PostedData{Issue: issue, Comment: comment, ForgeID: forgeID})
}

func refusedOp(author string, now int64, issue, field, value, reason string) kernel.Op {
	return syncOp("refused", author, now, bsync.RefusedData{Issue: issue, Field: field, Value: value, Reason: reason})
}

func runOp(author string, now, started int64) kernel.Op {
	return syncOp("run", author, now, bsync.RunData{Started: started})
}
