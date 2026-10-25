// Package gitbugiotest builds real, throwaway repositories on the core for
// tests, instead of a hand-rolled fake: a git repository in t.TempDir(),
// opened as a gitbugio.Backend. Split into its own package so other
// packages' tests can use it too.
package gitbugiotest

import (
	"encoding/json"
	"errors"
	"net/url"
	"os/exec"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/guilbep/git-board/core/issue"
	"github.com/guilbep/git-board/core/oplog"
	"github.com/guilbep/git-board/core/people"
	"github.com/guilbep/git-board/core/syncer"
	"github.com/guilbep/git-board/internal/gitbugio"
)

// Git runs git in dir, failing the test on an error, and answers its
// output. Commits are never signed, whatever the user's config says.
func Git(t testing.TB, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false"}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// Repo makes a git repository in a new temporary directory, bare or not,
// with a user, and answers its directory. No one is chosen to write as.
func Repo(t testing.TB, bare bool) string {
	t.Helper()
	dir := t.TempDir()
	if bare {
		Git(t, dir, "init", "-q", "--bare")
		return dir
	}
	Git(t, dir, "init", "-q")
	Git(t, dir, "config", "user.name", "dev")
	Git(t, dir, "config", "user.email", "dev@example.com")
	return dir
}

// Init makes the repository at dir one git-board writes in: a person,
// named as git's user (dev when unset), who is me.
func Init(t testing.TB, dir string) {
	t.Helper()
	b := Open(t, dir)
	name := strings.TrimSpace(gitOut(dir, "config", "user.name"))
	if name == "" {
		name = "dev"
	}
	if _, err := b.NewIdentity(name, strings.TrimSpace(gitOut(dir, "config", "user.email"))); err != nil {
		t.Fatalf("NewIdentity: %v", err)
	}
}

func gitOut(dir string, args ...string) string {
	out, _ := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	return string(out)
}

// Open opens the repository at dir, closed when the test ends.
func Open(t testing.TB, dir string) *gitbugio.Backend {
	t.Helper()
	b, err := gitbugio.Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = b.Close() })
	return b
}

// New is a repository in a t.TempDir() with someone chosen to write as,
// ready to create and query issues.
func New(t testing.TB) *gitbugio.Backend {
	t.Helper()
	dir := Repo(t, false)
	Init(t, dir)
	return Open(t, dir)
}

// NewEmpty is New with no one chosen to write as.
func NewEmpty(t testing.TB) *gitbugio.Backend {
	t.Helper()
	return Open(t, Repo(t, false))
}

// NewPair is New twice, two clones of one bare remote, origin: you and a
// colleague sharing a server.
func NewPair(t testing.TB) (mine, theirs *gitbugio.Backend) {
	t.Helper()
	remote := Repo(t, true)
	a, b := Repo(t, false), Repo(t, false)
	Git(t, a, "remote", "add", "origin", remote)
	Git(t, b, "remote", "add", "origin", remote)
	Init(t, a)
	Init(t, b)
	return Open(t, a), Open(t, b)
}

// me is who b writes as.
func me(t testing.TB, b *gitbugio.Backend) string {
	t.Helper()
	p, ok, err := b.Me()
	if err != nil || !ok {
		t.Fatalf("Me: %v, %v", ok, err)
	}
	return p.ID
}

// NewIssue creates an issue with the given title and returns its id.
func NewIssue(t testing.TB, b *gitbugio.Backend, title string) string {
	t.Helper()
	id, err := b.NewIssue(title, "", nil)
	if err != nil {
		t.Fatalf("NewIssue: %v", err)
	}
	return id
}

// NewImportedIssue is NewIssue for an issue linked to the GitHub issue at
// githubURL, as a pull from GitHub leaves it: what internal/ghbridge
// matches a Project item with.
func NewImportedIssue(t testing.TB, b *gitbugio.Backend, title, githubURL string) string {
	t.Helper()
	id := NewIssue(t, b, title)
	Link(t, b, id, githubURL)
	return id
}

// Link links an issue to the forge issue at u, in the sync entity of u's
// project (host/owner/project), as a pull or a push records it.
func Link(t testing.TB, b *gitbugio.Backend, id, u string) {
	t.Helper()
	parsed, err := url.Parse(u)
	if err != nil {
		t.Fatal(err)
	}
	project, _, _ := strings.Cut(strings.TrimPrefix(parsed.Path, "/"), "/-/")
	project, _, _ = strings.Cut(project, "/issues/")
	key := parsed.Host + "/" + project
	data, _ := json.Marshal(map[string]string{"issue": id, "forge_id": u, "number": path.Base(u), "url": u})
	op := oplog.Op{Kind: "link", Author: me(t, b), Time: time.Now().Unix(), Data: data}
	sid := syncer.StateID(key)
	l, err := oplog.Load(b.Store(), "sync", sid)
	switch {
	case errors.Is(err, oplog.ErrNotFound):
		_, err = oplog.Create(t.Context(), b.Store(), "sync", sid, []oplog.Op{op})
	case err == nil:
		err = l.Append(t.Context(), b.Store(), []oplog.Op{op})
	}
	if err != nil {
		t.Fatalf("link %s: %v", u, err)
	}
}

// CloseIssue closes the given issue (by id prefix) so tests can exercise
// status:closed filtering.
func CloseIssue(t testing.TB, b *gitbugio.Backend, idPrefix string) {
	t.Helper()
	if err := b.CloseBug(idPrefix); err != nil {
		t.Fatalf("CloseBug: %v", err)
	}
}

// NewPerson creates another person with a GitHub account of that login:
// a colleague, or a second person for you. An empty login is no account.
func NewPerson(t testing.TB, b *gitbugio.Backend, name, login string) gitbugio.Person {
	t.Helper()
	ops := []oplog.Op{people.NewCreate(me(t, b), name, "")}
	if login != "" {
		ops = append(ops, people.AddAccount(me(t, b), people.Account{Server: "github.com", ID: login, Login: login}))
	}
	p, err := people.Create(t.Context(), b.Store(), ops...)
	if err != nil {
		t.Fatalf("people.Create: %v", err)
	}
	return gitbugio.Person{ID: p.ID, Name: p.Name, Login: login}
}

// CommentAs comments on the issue as who, at unix time, and returns the
// comment's id as the thread gives it (Event.ID).
func CommentAs(t testing.TB, b *gitbugio.Backend, idPrefix string, who gitbugio.Person, unix int64, body string) string {
	t.Helper()
	is, err := b.GetIssue(idPrefix)
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	op := issue.NewComment(who.ID, body)
	op.Time = unix
	got, err := issue.Do(t.Context(), b.Store(), is.ID, op)
	if err != nil {
		t.Fatalf("comment: %v", err)
	}
	return got.Comments[len(got.Comments)-1].ID
}

// SetMyLogin gives the person b writes as an account on github.com with
// that login: what @me then means.
func SetMyLogin(t testing.TB, b *gitbugio.Backend, login string) {
	t.Helper()
	id := me(t, b)
	if _, err := people.Do(t.Context(), b.Store(), id, people.AddAccount(id, people.Account{Server: "github.com", ID: login, Login: login})); err != nil {
		t.Fatalf("add account: %v", err)
	}
}

// RawLabels adds labels as stored, past the flat kinds' translation: as a
// forge has them.
func RawLabels(t testing.TB, b *gitbugio.Backend, id string, labels ...string) {
	t.Helper()
	if _, err := issue.Do(t.Context(), b.Store(), id, issue.ChangeLabels(me(t, b), labels, nil)); err != nil {
		t.Fatalf("labels: %v", err)
	}
}

// IssueAs creates an issue as who, at unix time, with labels, and returns
// its id.
func IssueAs(t testing.TB, b *gitbugio.Backend, who gitbugio.Person, unix int64, title, body string, labels ...string) string {
	t.Helper()
	ops := []oplog.Op{issue.NewCreate(who.ID, title, body)}
	if len(labels) > 0 {
		ops = append(ops, issue.ChangeLabels(who.ID, labels, nil))
	}
	for i := range ops {
		ops[i].Time = unix
	}
	is, err := issue.Create(t.Context(), b.Store(), ops...)
	if err != nil {
		t.Fatalf("issue.Create: %v", err)
	}
	return is.ID
}

// Do writes core operations to an issue, past the backend: another
// person's, or at another time.
func Do(t testing.TB, b *gitbugio.Backend, id string, ops ...oplog.Op) {
	t.Helper()
	if _, err := issue.Do(t.Context(), b.Store(), id, ops...); err != nil {
		t.Fatalf("issue.Do: %v", err)
	}
}
