# Inbox Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A per-identity inbox in git-board's web UI: what happened that concerns you on the issues you follow, since you last opened them, with Follow/Unfollow and two new filter terms (`involves:`, `follows:@me`).

**Architecture:** "How someone is in an issue" lives once, in `internal/filter` (`Involvement`), used by the new `involves:`/`follows:@me` terms and by `internal/inbox`. `internal/inbox` holds one identity's file (git-bug local storage) and one pure function, `Rows`, with every rule. `cmd/git-board-server/inbox.go` gathers the last 14 days of threads and serves five endpoints. The web gets an Inbox page, a header count, and a Follow block in the issue's side column. Opening the inbox fetches the git remotes and offers Pull: it never pulls by itself.

**Tech Stack:** Go 1.22+ (`net/http` method routing), git-bug fork (`github.com/guilbep/git-bug`, cache + bleve full-text index), Vue 3 + Quasar + TypeScript, `node --test` for web unit tests.

**Spec:** `docs/prd/inbox.md` (read it first; this plan argues from it). Mockup: https://claude.ai/artifact/Xh5BK2s82ysTfaMw9a72TX

## Global Constraints

- No new dependency, Go or npm.
- The 14-day window: `inbox.Window = 14 * 24 * time.Hour`, on `Issue.UpdatedAt` to load and on each event's own time to count.
- `seen` holds event IDs (`Event.ID`, git-bug's `CombinedId`), never a time.
- One file per identity: `git-board/inbox/<identity-id>.json` in `Backend.Storage()` (git-bug's local storage, the common git dir: shared by worktrees, never pushed).
- You are the identity git-board writes as (`Backend.Me()`) **and any identity whose login equals yours**, ignoring case. Your login: the server's `--me`, else `Backend.MyLogin()`.
- No identity: `409` code `no-identity`. No login: `409` code `no-login`. An unreadable file: an error naming it, never reset.
- Opening the inbox fetches (`POST /api/remotes/{name}/fetch`) each remote not checked in the last 5 minutes; Pull is a click; nothing pulls on its own.
- Web: all HTTP goes through `web/src/api/board.ts` ("never fetch() from a component directly"). Unit-tested modules import types relatively (`import type { X } from './api/board'`), as `issuesync.ts` does, because `node --test` doesn't resolve `@/`.
- Deliberate shortcuts carry a `ponytail:` comment naming the limit and the upgrade, as the codebase already does.
- Commit messages: `feat(inbox): …` / `docs(inbox): …`, lower case, ending with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- Before touching `web/src/components/IssueSide.vue` (Task 10): merge `main` after the `issuelink` branch has landed there.

## Review Focus

1. **Switching who you write as** (Settings → You) must show that identity's inbox: its own file, never the previous identity's follows or seen. Pinned in Task 6 (`TestInboxIsPerIdentity`).
2. **Two tabs marking issues read at the same moment** must both stick (one read-change-write lock). Pinned in Task 6 (`TestInboxSeenFromTwoTabs`).
3. **An issue named by a prefix** (links and the CLI carry 7 characters) in `seen` or `follow` must be stored under its full id, or the inbox never finds it. Pinned in Task 6 (`TestInboxTakesPrefixes`).
4. **An unknown issue** in `seen` or `follow` answers `404` and leaves the file as it was. Pinned in Task 6 (`TestInboxTakesPrefixes`).
5. **A forge sync running when the inbox opens**: the remotes' check answers `409 syncing`; the page says "a forge sync is running" and still shows the rows. Pinned in Task 7 (`checkFailure` test).

---

## File Structure

| File | Responsibility |
|---|---|
| `internal/gitbugio/mentions.go` (new) | the mention pattern, `Mentions`, `Backend.Mentioning` |
| `internal/gitbugio/gitbugio.go` | `Issue.CommenterLogins`, filled from the excerpt |
| `internal/gitbugio/gitbugiotest/gitbugiotest.go` | `NewPerson`, `CommentAs` test helpers |
| `internal/filter/filter.go` | `Involved`, `Involvement`, `PersonField`, `involves:`, `follows:@me` |
| `internal/board/board.go` | fills `involves:` mentions and `follows:@me` sets; `FollowedViews` |
| `internal/inbox/state.go` (new) | the file: `State`, `Load`, `Save`, `MarkSeen`, `SetFollow`, `Set`, `Prune` |
| `internal/inbox/rows.go` (new) | `Me`, `Window`, `Row`, `Follows`, `Rows` |
| `cmd/git-board-server/inbox.go` (new) | the five endpoints |
| `cmd/git-board-server/server.go`, `events.go` | routes, `inboxMu`, the `inbox` event kind |
| `web/src/api/board.ts` | inbox types and calls |
| `web/src/inboxcheck.ts` (new) | pure: which remotes to check, which offer Pull, what a failure says |
| `web/src/inbox.ts` (new) | the shared inbox ref the header and the page read |
| `web/src/pages/InboxPage.vue` (new) | the page |
| `web/src/components/IssuePanel.vue` | `at`: scroll to an event once loaded |
| `web/src/components/AppHeader.vue`, `CommandPalette.vue`, `GbIcon.vue`, `router/routes.ts`, `live.ts` | the link, its count, the tab title, the palette entry, the icon, the route, the event kind |
| `web/src/components/IssueSide.vue` | the Follow block |
| `docs/configuration.md`, `docs/implementation-plan.md` | the two filter terms; a status update |

---

### Task 1: Mentions and commenters in `gitbugio`

**Files:**
- Create: `internal/gitbugio/mentions.go`
- Create: `internal/gitbugio/mentions_test.go`
- Modify: `internal/gitbugio/gitbugio.go` (`Issue` struct at line 38; `issueFrom` at line 179)
- Modify: `internal/gitbugio/gitbugiotest/gitbugiotest.go` (append helpers)

**Interfaces:**
- Produces: `gitbugio.Mentions(text, login string) bool`; `(*Backend).Mentioning(login string) (map[string]bool, error)`; `Issue.CommenterLogins []string` (logins of the identities that commented, the author excepted; `json:"-"`); `gitbugiotest.NewPerson(t, b, name, login string) *cache.IdentityCache`; `gitbugiotest.CommentAs(t, b, idPrefix string, who *cache.IdentityCache, unix int64, body string) string` (the comment's timeline id).

- [ ] **Step 1: Add the test helpers** to `internal/gitbugio/gitbugiotest/gitbugiotest.go` (add `"github.com/git-bug/git-bug/cache"` to its imports):

```go
// NewPerson creates another identity with a forge login: a colleague, or
// a second identity of yours (old bridges left some).
func NewPerson(t testing.TB, b *gitbugio.Backend, name, login string) *cache.IdentityCache {
	t.Helper()
	i, err := b.TestCache().Identities().NewRaw(name, "", login, "", nil, nil)
	if err != nil {
		t.Fatalf("Identities().NewRaw: %v", err)
	}
	return i
}

// CommentAs comments on the issue as who, at unix time, and returns the
// comment's id as the timeline gives it (Event.ID).
func CommentAs(t testing.TB, b *gitbugio.Backend, idPrefix string, who *cache.IdentityCache, unix int64, body string) string {
	t.Helper()
	bug, err := b.TestCache().Bugs().ResolvePrefix(idPrefix)
	if err != nil {
		t.Fatalf("ResolvePrefix: %v", err)
	}
	id, _, err := bug.AddCommentRaw(who, unix, body, nil, nil)
	if err != nil {
		t.Fatalf("AddCommentRaw: %v", err)
	}
	if err := bug.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	return id.String()
}
```

- [ ] **Step 2: Write the failing tests** in `internal/gitbugio/mentions_test.go`:

```go
package gitbugio_test

import (
	"slices"
	"testing"
	"time"

	"github.com/guilbep/git-board/internal/gitbugio"
	"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"
)

func TestMentions(t *testing.T) {
	for text, want := range map[string]bool{
		"thanks @guilbep":             true,
		"thanks @GuilBep.":            true,
		"(@guilbep)":                  true,
		"mail guilbep.pierre@epfl.ch": false, // an email
		"@guilbep-bot did it":         false, // another login
		"see `@guilbep`":              false, // code
		"a/b/@guilbep":                false, // a path
		"no one":                      false,
	} {
		if got := gitbugio.Mentions(text, "guilbep"); got != want {
			t.Errorf("Mentions(%q) = %v, want %v", text, got, want)
		}
	}
	if gitbugio.Mentions("@x", "") {
		t.Error("an empty login is mentioned by nothing")
	}
}

// The index finds the login anywhere (an email, @guilbep-bot); the
// pattern keeps the real mentions.
func TestMentioningConfirmsWhatTheIndexFinds(t *testing.T) {
	b := gitbugiotest.New(t)
	inBody, _ := b.NewIssue("Train legs", "ping @guilbep", nil)
	inComment, _ := b.NewIssue("Buildings", "nothing", nil)
	if _, err := b.AddComment(inComment, "what do you think @guilbep?"); err != nil {
		t.Fatal(err)
	}
	email, _ := b.NewIssue("Mail", "write to guilbep.pierre@epfl.ch or @guilbep-bot", nil)

	got, err := b.Mentioning("guilbep")
	if err != nil {
		t.Fatal(err)
	}
	if !got[inBody] || !got[inComment] || got[email] || len(got) != 2 {
		t.Errorf("Mentioning = %v, want %s and %s only", got, inBody, inComment)
	}
}

func TestCommenterLoginsLeaveOutTheAuthor(t *testing.T) {
	b := gitbugiotest.New(t)
	id, _ := b.NewIssue("Train legs", "", nil)
	if _, err := b.AddComment(id, "mine"); err != nil {
		t.Fatal(err)
	}
	ann := gitbugiotest.NewPerson(t, b, "Ann", "ann")
	gitbugiotest.CommentAs(t, b, id, ann, time.Now().Unix(), "hers")

	issue, err := b.GetIssue(id)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(issue.CommenterLogins, []string{"ann"}) {
		t.Errorf("CommenterLogins = %v, want [ann]", issue.CommenterLogins)
	}
}
```

- [ ] **Step 3: Run them to see them fail**

Run: `go test ./internal/gitbugio/ -run 'TestMentions|TestMentioning|TestCommenterLogins' -v`
Expected: build failure, `undefined: gitbugio.Mentions`, `b.Mentioning undefined`, `issue.CommenterLogins undefined`.

- [ ] **Step 4: Write `internal/gitbugio/mentions.go`**

```go
package gitbugio

import (
	"regexp"
	"strings"
)

// mentionRe is the fork's bridge/core/render.go mentionRe, which is
// unexported (web/src/richtext.ts copies it too): an @login not inside an
// email, a path or code, ending on a letter, a digit or _. Its second
// group is the mention, @ included.
var mentionRe = regexp.MustCompile(`(^|[^\w.\-/@` + "`" + `])(@[A-Za-z0-9_](?:[A-Za-z0-9_.\-]*[A-Za-z0-9_])?)`)

// Mentions reports whether text mentions login (@login), ignoring case.
func Mentions(text, login string) bool {
	if login == "" {
		return false
	}
	for _, m := range mentionRe.FindAllStringSubmatch(text, -1) {
		if strings.EqualFold(m[2][1:], login) {
			return true
		}
	}
	return false
}

// Mentioning is the issues whose description or a comment mentions login,
// by id: the full-text index narrows (it finds the login anywhere, in an
// email too), the mention pattern confirms on their text.
func (b *Backend) Mentioning(login string) (map[string]bool, error) {
	res := map[string]bool{}
	if login == "" {
		return res, nil
	}
	hits, err := b.SearchText(login)
	if err != nil {
		return nil, err
	}
	for id := range hits {
		bug, err := b.c.Bugs().ResolvePrefix(id)
		if err != nil {
			return nil, err
		}
		for _, c := range bug.Snapshot().Comments { // the description is the first
			if Mentions(c.Message, login) {
				res[id] = true
				break
			}
		}
	}
	return res, nil
}
```

- [ ] **Step 5: Add `CommenterLogins`** to `Issue` in `internal/gitbugio/gitbugio.go`, after `AuthorLogins`:

```go
	// CommenterLogins are the logins of who commented, the author
	// excepted, for involves:.
	CommenterLogins []string `json:"-"`
```

and fill it in `issueFrom`, after the author block:

```go
	for _, p := range e.Participants {
		if p == e.AuthorId {
			continue
		}
		if c, err := b.c.Identities().ResolveExcerpt(p); err == nil {
			for _, l := range []string{c.Login, c.ImmutableMetadata["github-login"], c.ImmutableMetadata["gitlab-login"], c.ImmutableMetadata["gitea-login"]} {
				if l != "" {
					issue.CommenterLogins = append(issue.CommenterLogins, l)
				}
			}
		}
	}
```

- [ ] **Step 6: Run the tests to see them pass**

Run: `go test ./internal/gitbugio/... -v -run 'TestMentions|TestMentioning|TestCommenterLogins'` then `go test ./internal/...`
Expected: PASS, and nothing else broken.

- [ ] **Step 7: Commit**

```bash
git add internal/gitbugio
git commit -m "feat(inbox): who an issue mentions, and who commented on it

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 2: `Involvement` and the `involves:` term

**Files:**
- Modify: `internal/filter/filter.go`
- Modify: `internal/board/board.go` (`prepare`, line ~57)
- Test: `internal/filter/involves_test.go` (new), `internal/board/search_test.go` (append)
- Modify: `docs/configuration.md` (the Filters table, line ~158)

**Interfaces:**
- Consumes: `Issue.AuthorLogins`, `Issue.CommenterLogins`, `(*Backend).Mentioning` (Task 1).
- Produces: `filter.KindInvolves`; `filter.Involved{AuthorLogins, Labels, CommenterLogins []string; Mentioned bool}`; `filter.Involvement(in Involved, login string, cfg *config.Config) []string` (reasons in this order: `author`, person-field names in label order, `commented`, `mentioned`; nil when none); `filter.PersonField(label, login string, cfg *config.Config) (field string, ok bool)`; `Condition.Mentions map[string]bool` (issues mentioning the term's login, by id).

- [ ] **Step 1: Write the failing tests** in `internal/filter/involves_test.go`:

```go
package filter_test

import (
	"slices"
	"testing"
	"time"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/filter"
	"github.com/guilbep/git-board/internal/gitbugio"
)

func TestInvolvement(t *testing.T) {
	cfg := &config.Config{}
	in := filter.Involved{
		AuthorLogins:    []string{"Ann Example", "ann"},
		Labels:          []string{"assignee:guilbep", "reviewer:GUILBEP", "status:guilbep", "bug"},
		CommenterLogins: []string{"srossi", "guilbep"},
		Mentioned:       true,
	}
	// status isn't a person field: a value that happens to be a login
	if got, want := filter.Involvement(in, "guilbep", cfg), []string{"assignee", "reviewer", "commented", "mentioned"}; !slices.Equal(got, want) {
		t.Errorf("guilbep: %v, want %v", got, want)
	}
	if got := filter.Involvement(filter.Involved{AuthorLogins: in.AuthorLogins}, "ANN", cfg); !slices.Equal(got, []string{"author"}) {
		t.Errorf("ann: %v, want [author]", got)
	}
	if got := filter.Involvement(in, "nobody", cfg); got != nil {
		t.Errorf("nobody: %v, want none", got)
	}
	if got := filter.Involvement(in, "", cfg); got != nil {
		t.Errorf("no login: %v, want none", got)
	}
	if got := filter.Involvement(in, "guilbep", nil); len(got) == 0 {
		t.Error("a nil config still knows assignee by its name")
	}
}

func TestInvolvesTerm(t *testing.T) {
	cfg, now := &config.Config{}, time.Now()
	assigned := gitbugio.Issue{ID: "a1", Labels: []string{"assignee:guilbep"}}
	mentioned := gitbugio.Issue{ID: "b2"}
	other := gitbugio.Issue{ID: "c3", AuthorLogins: []string{"srossi"}}

	conds := filter.Parse("involves:@me")
	conds[0].Mentions = map[string]bool{"b2": true}
	for _, tc := range []struct {
		issue gitbugio.Issue
		want  bool
	}{{assigned, true}, {mentioned, true}, {other, false}} {
		if got := filter.Matches(tc.issue, conds, cfg, "guilbep", now); got != tc.want {
			t.Errorf("involves:@me on %s = %v, want %v", tc.issue.ID, got, tc.want)
		}
	}
	if !filter.Matches(other, filter.Parse("involves:srossi"), cfg, "guilbep", now) {
		t.Error("involves:srossi misses srossi's issue")
	}
	if filter.Matches(assigned, filter.Parse("-involves:@me"), cfg, "guilbep", now) {
		t.Error("-involves:@me keeps an issue you're assigned")
	}
	if !filter.RequiresMe(filter.Parse("involves:@me")) {
		t.Error("involves:@me needs to know who you are")
	}
}
```

And append to `internal/board/search_test.go`:

```go
// involves:, on a real repository: a person field, a mention found
// through the index, a comment by an identity carrying the login.
func TestInvolvesOnARepository(t *testing.T) {
	b := gitbugiotest.New(t)
	assigned, _ := b.NewIssue("Train legs", "", []string{"assignee:guilbep"})
	mentioned, _ := b.NewIssue("Buildings", "ping @guilbep", nil)
	commented, _ := b.NewIssue("Food", "", nil)
	other := gitbugiotest.NewPerson(t, b, "Guilbert (GitHub)", "guilbep")
	gitbugiotest.CommentAs(t, b, commented, other, time.Now().Unix(), "on it")
	if _, err := b.NewIssue("Mail", "guilbep.pierre@epfl.ch", nil); err != nil {
		t.Fatal(err)
	}

	got, err := board.LoadMatching(b, &config.Config{}, "involves:@me", "guilbep")
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, i := range got {
		ids[i.ID] = true
	}
	if !ids[assigned] || !ids[mentioned] || !ids[commented] || len(ids) != 3 {
		t.Errorf("involves:@me = %v, want %s, %s, %s", ids, assigned, mentioned, commented)
	}
}
```

(add `"time"` to `search_test.go`'s imports.)

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/filter/ ./internal/board/ -run 'TestInvolvement|TestInvolvesTerm|TestInvolvesOnARepository' -v`
Expected: build failure, `undefined: filter.Involved`.

- [ ] **Step 3: Implement in `internal/filter/filter.go`**

Extend the package comment's grammar list: after `author:login,` add `involves:login|@me (author, a person field, a comment, a mention), follows:@me (what your inbox follows),`.

Add the kinds:

```go
	KindInvolves Kind = "involves"
	KindFollows  Kind = "follows" // Task 5
```

Add to `Condition`:

```go
	// Mentions are, for involves: and follows:, the issues whose text
	// mentions the login, by id (board fills it); nil: none.
	Mentions map[string]bool
```

In `Parse`, before `default:`:

```go
		case fieldName == "involves":
			conditions = append(conditions, Condition{Kind: KindInvolves, Value: value, Negated: negated})
```

In `RequiresMe`, widen the kind test to `if c.Kind == KindField || c.Kind == KindAuthor || c.Kind == KindInvolves {`.

In `Matches`, `involves:` takes one login, no list: change the `values` condition to
`if c.Kind != KindText && c.Kind != KindNo && c.Kind != KindTitle && c.Kind != KindNumber && c.Kind != KindInvolves {`.

In `matchOne`, before `case KindField:`:

```go
	case KindInvolves:
		in := Involved{AuthorLogins: issue.AuthorLogins, Labels: issue.Labels, CommenterLogins: issue.CommenterLogins, Mentioned: c.Mentions[issue.ID]}
		ok = len(Involvement(in, Resolve(value, cfg, me, today), cfg)) > 0
```

And the shared function, at the end of the file (add `"slices"` to the imports):

```go
// Involved is what Involvement reads of an issue: from the list (an
// Issue) or from its thread (internal/inbox), so the two can't disagree.
type Involved struct {
	AuthorLogins    []string
	Labels          []string
	CommenterLogins []string
	Mentioned       bool // its description or a comment mentions the login
}

// Involvement is how login is in an issue, as reasons: author, a person
// field's name (assignee, reviewer…), commented, mentioned. None: it isn't.
func Involvement(in Involved, login string, cfg *config.Config) []string {
	if login == "" {
		return nil
	}
	is := func(l string) bool { return strings.EqualFold(l, login) }
	var res []string
	if slices.ContainsFunc(in.AuthorLogins, is) {
		res = append(res, "author")
	}
	for _, l := range in.Labels {
		if f, ok := PersonField(l, login, cfg); ok && !slices.Contains(res, f) {
			res = append(res, f)
		}
	}
	if slices.ContainsFunc(in.CommenterLogins, is) {
		res = append(res, "commented")
	}
	if in.Mentioned {
		res = append(res, "mentioned")
	}
	return res
}

// PersonField is the field a label names login in (assignee for
// assignee:guilbep), when that field holds people.
func PersonField(label, login string, cfg *config.Config) (string, bool) {
	if cfg == nil {
		cfg = &config.Config{}
	}
	f, v, ok := strings.Cut(label, ":")
	if !ok || login == "" || !strings.EqualFold(v, login) || cfg.TypeOf(f, nil) != config.TypePerson {
		return "", false
	}
	return f, true
}
```

- [ ] **Step 4: Fill the mentions in `internal/board/board.go`'s `prepare`**, after the `SearchText` block and before `return conditions, me, nil`:

```go
	if m, ok := b.(interface {
		Mentioning(string) (map[string]bool, error)
	}); ok {
		for i, c := range conditions {
			if c.Kind != filter.KindInvolves && c.Kind != filter.KindFollows {
				continue
			}
			login := c.Value
			if c.Kind == filter.KindFollows || login == "@me" {
				login = me
			}
			var err error
			if conditions[i].Mentions, err = m.Mentioning(login); err != nil {
				return nil, "", err
			}
		}
	}
```

- [ ] **Step 5: Run the tests to see them pass**

Run: `go test ./internal/filter/ ./internal/board/ -v -run 'TestInvolvement|TestInvolvesTerm|TestInvolvesOnARepository'` then `go test ./internal/...`
Expected: PASS.

- [ ] **Step 6: Document the term** in `docs/configuration.md`'s Filters table, after the `author:` row:

```markdown
| `involves:guilbep`, `involves:@me` | issues someone is in: the author, a person field (assignee, reviewer…), a comment, an @mention |
```

- [ ] **Step 7: Commit**

```bash
git add internal/filter internal/board docs/configuration.md
git commit -m "feat(inbox): involves:, the issues someone is in

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 3: The inbox file

**Files:**
- Create: `internal/inbox/state.go`
- Create: `internal/inbox/state_test.go`

**Interfaces:**
- Consumes: `gitbugio.Thread`, `(*Backend).Storage() repository.LocalStorage`.
- Produces: `inbox.State{Seen map[string][]string; Follow, Unfollow, Views []string; Fields bool}`; `inbox.Load(fs repository.LocalStorage, identityID string) (State, error)`; `inbox.Save(fs repository.LocalStorage, identityID string, s State) error`; `(*State).MarkSeen(th gitbugio.Thread)`; `(*State).SetFollow(id string, follow *bool)`; `(State).Set(id string) *bool`; `(*State).Prune(keep map[string]bool)`.

- [ ] **Step 1: Write the failing tests** in `internal/inbox/state_test.go`:

```go
package inbox_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/gitbugio"
	"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"
	"github.com/guilbep/git-board/internal/inbox"
)

func TestStateRoundTrip(t *testing.T) {
	fs := gitbugiotest.New(t).Storage()
	st, err := inbox.Load(fs, "me1")
	if err != nil || !reflect.DeepEqual(st, inbox.State{}) {
		t.Fatalf("no file yet: %+v, %v; want an empty State", st, err)
	}
	st.MarkSeen(gitbugio.Thread{Issue: gitbugio.Issue{ID: "a1"}, Events: []gitbugio.Event{{ID: "e1"}, {ID: "e2"}}})
	yes, no := true, false
	st.SetFollow("b2", &yes)
	st.SetFollow("c3", &no)
	st.Views, st.Fields = []string{"backend"}, true
	if err := inbox.Save(fs, "me1", st); err != nil {
		t.Fatal(err)
	}
	got, err := inbox.Load(fs, "me1")
	if err != nil || !reflect.DeepEqual(got, st) {
		t.Fatalf("Load after Save = %+v, %v; want %+v", got, err, st)
	}
	if other, _ := inbox.Load(fs, "me2"); !reflect.DeepEqual(other, inbox.State{}) {
		t.Errorf("another identity's file: %+v, want empty", other)
	}
}

func TestStateUnreadableIsAnError(t *testing.T) {
	fs := gitbugiotest.New(t).Storage()
	if err := fs.MkdirAll("git-board/inbox", 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := fs.Create("git-board/inbox/me1.json")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.Write([]byte("{not json"))
	_ = f.Close()
	if _, err := inbox.Load(fs, "me1"); err == nil || !strings.Contains(err.Error(), "me1.json") {
		t.Errorf("Load of a broken file: %v, want an error naming it", err)
	}
}

func TestFollowAndPrune(t *testing.T) {
	var st inbox.State
	yes, no := true, false
	st.SetFollow("a1", &yes)
	if s := st.Set("a1"); s == nil || !*s {
		t.Errorf("after Follow: %v", s)
	}
	st.SetFollow("a1", &no)
	if s := st.Set("a1"); s == nil || *s || len(st.Follow) != 0 {
		t.Errorf("after Unfollow: %v, follow %v", s, st.Follow)
	}
	st.SetFollow("a1", nil)
	if s := st.Set("a1"); s != nil || len(st.Unfollow) != 0 {
		t.Errorf("back to automatic: %v, unfollow %v", s, st.Unfollow)
	}

	st.Seen = map[string][]string{"old": {"e1"}, "new": {"e2"}}
	st.Prune(map[string]bool{"new": true})
	if !reflect.DeepEqual(st.Seen, map[string][]string{"new": {"e2"}}) {
		t.Errorf("Prune kept %v", st.Seen)
	}
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/inbox/ -v`
Expected: build failure, `no non-test Go files` / `undefined: inbox.State`.

- [ ] **Step 3: Write `internal/inbox/state.go`**

```go
// Package inbox is one identity's inbox: what it follows and has seen (its
// file, in git-bug's local storage), and the rows: what happened that
// concerns it (rows.go). Pure but for the file; the server gathers the
// threads.
package inbox

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/git-bug/git-bug/repository"

	"github.com/guilbep/git-board/internal/gitbugio"
)

// State is one identity's file.
type State struct {
	Seen     map[string][]string `json:"seen,omitempty"`     // issue id → the event ids seen
	Follow   []string            `json:"follow,omitempty"`   // issues followed by hand
	Unfollow []string            `json:"unfollow,omitempty"` // and unfollowed
	Views    []string            `json:"views,omitempty"`    // views followed, by name
	Fields   bool                `json:"fields,omitempty"`   // also field changes on issues you're in
}

func path(identityID string) string {
	return filepath.Join("git-board", "inbox", identityID+".json")
}

// Load reads identityID's file; none yet is an empty State.
func Load(fs repository.LocalStorage, identityID string) (State, error) {
	f, err := fs.Open(path(identityID))
	if errors.Is(err, os.ErrNotExist) {
		return State{}, nil
	}
	if err != nil {
		return State{}, err
	}
	defer f.Close()
	var s State
	if err := json.NewDecoder(f).Decode(&s); err != nil {
		return State{}, fmt.Errorf("reading %s: %w (deleting it starts over, your follows with it)", path(identityID), err)
	}
	return s, nil
}

// Save writes it through a temporary file and a rename: a crash leaves the
// old file or the new one, never half of one.
func Save(fs repository.LocalStorage, identityID string, s State) error {
	p := path(identityID)
	if err := fs.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	f, err := fs.Create(p + ".tmp")
	if err != nil {
		return err
	}
	if err := json.NewEncoder(f).Encode(s); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return fs.Rename(p+".tmp", p)
}

// MarkSeen takes every event th has now as seen, counted or not.
func (s *State) MarkSeen(th gitbugio.Thread) {
	if s.Seen == nil {
		s.Seen = map[string][]string{}
	}
	ids := make([]string, len(th.Events))
	for i, e := range th.Events {
		ids[i] = e.ID
	}
	s.Seen[th.ID] = ids
}

// SetFollow is Follow (true), Unfollow (false), or back to automatic (nil).
func (s *State) SetFollow(id string, follow *bool) {
	other := func(x string) bool { return x == id }
	s.Follow = slices.DeleteFunc(s.Follow, other)
	s.Unfollow = slices.DeleteFunc(s.Unfollow, other)
	switch {
	case follow == nil:
	case *follow:
		s.Follow = append(s.Follow, id)
	default:
		s.Unfollow = append(s.Unfollow, id)
	}
}

// Set is what you clicked for an issue: true, false, or nil (automatic).
func (s State) Set(id string) *bool {
	yes, no := true, false
	switch {
	case slices.Contains(s.Follow, id):
		return &yes
	case slices.Contains(s.Unfollow, id):
		return &no
	}
	return nil
}

// Prune forgets what was seen of the issues not in keep: out of the
// window, they can't show again.
func (s *State) Prune(keep map[string]bool) {
	for id := range s.Seen {
		if !keep[id] {
			delete(s.Seen, id)
		}
	}
}
```

The empty `State{}` in the tests compares equal to a decoded `{}` only because the slices and map stay nil: keep `omitempty` on every field, and don't initialise them in `Load`.

- [ ] **Step 4: Run the tests to see them pass**

Run: `go test ./internal/inbox/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/inbox
git commit -m "feat(inbox): one file per identity: what it follows and has seen

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 4: The rules: `Follows` and `Rows`

**Files:**
- Create: `internal/inbox/rows.go`
- Create: `internal/inbox/rows_test.go`

**Interfaces:**
- Consumes: `filter.Involved`, `filter.Involvement`, `filter.PersonField` (Task 2); `gitbugio.Mentions` (Task 1); `State` (Task 3).
- Produces: `inbox.Me{ID, Login string}` (json `id`, `login`) with `(Me).Is(p gitbugio.Person) bool`; `inbox.Window`; `inbox.Row{Issue gitbugio.Issue; Reasons, News []string; Who []gitbugio.Person; Latest time.Time; First string}` (json `issue`, `reasons`, `news`, `who`, `latest`, `first`); `inbox.Follows(me Me, st State, th gitbugio.Thread, views []string, cfg *config.Config) (following bool, reasons []string, set *bool)`; `inbox.Rows(me Me, st State, threads []gitbugio.Thread, inViews map[string][]string, cfg *config.Config, since time.Time) []Row` (newest first).

- [ ] **Step 1: Write the failing table test** in `internal/inbox/rows_test.go`. Each case is one rule of the spec's §2:

```go
package inbox_test

import (
	"slices"
	"testing"
	"time"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio"
	"github.com/guilbep/git-board/internal/inbox"
)

var (
	now   = time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
	since = now.Add(-inbox.Window)
	old   = since.Add(-48 * time.Hour) // before the window
	me    = inbox.Me{ID: "me1", Login: "guilbep"}
	meP   = gitbugio.Person{ID: "me1", Name: "Guilbert Pierre", Login: "guilbep"}
	me2   = gitbugio.Person{ID: "me2", Name: "Guilbert (GitHub)", Login: "GuilBep"} // a second identity of yours
	ann   = gitbugio.Person{ID: "ann", Name: "Ann", Login: "ann"}
	cfg   = &config.Config{}
)

func ev(kind, id string, who gitbugio.Person, at time.Time) gitbugio.Event {
	return gitbugio.Event{Kind: kind, ID: id, Author: who, At: at}
}

func comment(id string, who gitbugio.Person, at time.Time, body string) gitbugio.Event {
	e := ev("comment", id, who, at)
	e.Body = body
	return e
}

func labels(id string, who gitbugio.Person, at time.Time, added, removed []string) gitbugio.Event {
	e := ev("labels", id, who, at)
	e.Added, e.Removed = added, removed
	return e
}

func status(id string, who gitbugio.Person, at time.Time, s string) gitbugio.Event {
	e := ev("status", id, who, at)
	e.Status = s
	return e
}

// thread is an issue by author, created before the window, with labels
// and events after its creation.
func thread(id string, author gitbugio.Person, lbls []string, events ...gitbugio.Event) gitbugio.Thread {
	created := ev("created", id+"-0", author, old)
	return gitbugio.Thread{
		Issue:  gitbugio.Issue{ID: id, Title: id, Status: "open", Labels: lbls},
		Author: author,
		Events: append([]gitbugio.Event{created}, events...),
	}
}

func TestRows(t *testing.T) {
	for _, tc := range []struct {
		name    string
		th      gitbugio.Thread
		st      inbox.State
		views   []string
		news    []string // nil: no row
		reasons []string
	}{
		{name: "someone's comment on your assigned issue shows, yours doesn't",
			th:   thread("a", ann, []string{"assignee:guilbep"}, comment("a1", ann, now.Add(-time.Hour), "done?"), comment("a2", meP, now.Add(-30*time.Minute), "yes")),
			news: []string{"1 comment"}, reasons: []string{"assignee"}},
		{name: "a late arrival shows after you opened the issue",
			th:   thread("b", ann, []string{"assignee:guilbep"}, comment("b1", ann, now.Add(-26*time.Hour), "written offline yesterday")),
			st:   inbox.State{Seen: map[string][]string{"b": {"b-0"}}},
			news: []string{"1 comment"}, reasons: []string{"assignee"}},
		{name: "seen events don't show",
			th: thread("c", ann, []string{"assignee:guilbep"}, comment("c1", ann, now.Add(-time.Hour), "x")),
			st: inbox.State{Seen: map[string][]string{"c": {"c-0", "c1"}}}},
		{name: "added to a person field shows",
			th:   thread("d", ann, []string{"reviewer:guilbep"}, labels("d1", ann, now.Add(-2*time.Hour), []string{"reviewer:guilbep"}, nil)),
			news: []string{"added you as reviewer"}, reasons: []string{"reviewer"}},
		{name: "removed from a person field shows though you no longer follow",
			th:   thread("e", ann, nil, labels("e1", ann, now.Add(-2*time.Hour), nil, []string{"assignee:guilbep"})),
			news: []string{"removed you as assignee"}},
		{name: "a mention shows, and makes you follow",
			th:   thread("f", ann, nil, comment("f1", ann, now.Add(-time.Hour), "ping @guilbep")),
			news: []string{"1 comment", "mentioned you"}, reasons: []string{"mentioned"}},
		{name: "Unfollow hides activity",
			th: thread("g", ann, []string{"assignee:guilbep"}, comment("g1", ann, now.Add(-time.Hour), "x")),
			st: inbox.State{Unfollow: []string{"g"}}},
		{name: "a mention goes through Unfollow",
			th:   thread("h", ann, []string{"assignee:guilbep"}, comment("h1", ann, now.Add(-time.Hour), "@guilbep look")),
			st:   inbox.State{Unfollow: []string{"h"}},
			news: []string{"mentioned you"}, reasons: []string{"assignee", "mentioned"}},
		{name: "a sprint change is hidden with fields off",
			th: thread("i", ann, []string{"assignee:guilbep"}, labels("i1", ann, now.Add(-time.Hour), []string{"sprint:s43"}, []string{"sprint:s42"}))},
		{name: "a sprint change shows with fields on",
			th:   thread("j", ann, []string{"assignee:guilbep"}, labels("j1", ann, now.Add(-time.Hour), []string{"sprint:s43"}, []string{"sprint:s42"})),
			st:   inbox.State{Fields: true},
			news: []string{"1 field change"}, reasons: []string{"assignee"}},
		{name: "a title edit never shows",
			th: thread("k", ann, []string{"assignee:guilbep"}, ev("title", "k1", ann, now.Add(-time.Hour)))},
		{name: "Follow brings an issue in",
			th:   thread("l", ann, nil, comment("l1", ann, now.Add(-time.Hour), "x")),
			st:   inbox.State{Follow: []string{"l"}},
			news: []string{"1 comment"}, reasons: []string{"followed"}},
		{name: "a closed issue shows for a followed view",
			th:    thread("m", ann, nil, status("m1", ann, now.Add(-time.Hour), "closed")),
			views: []string{"backend"},
			news:  []string{"closed"}, reasons: []string{"view: backend"}},
		{name: "outside the window is ignored",
			th: thread("n", ann, []string{"assignee:guilbep"}, comment("n1", ann, since.Add(-time.Hour), "x"))},
		{name: "a second identity carrying your login is you",
			th: thread("o", ann, []string{"assignee:guilbep"}, comment("o1", me2, now.Add(-time.Hour), "from GitHub"))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows := inbox.Rows(me, tc.st, []gitbugio.Thread{tc.th}, map[string][]string{tc.th.ID: tc.views}, cfg, since)
			if tc.news == nil {
				if len(rows) != 0 {
					t.Fatalf("got %+v, want no row", rows)
				}
				return
			}
			if len(rows) != 1 {
				t.Fatalf("got %d rows, want 1", len(rows))
			}
			if !slices.Equal(rows[0].News, tc.news) || !slices.Equal(rows[0].Reasons, tc.reasons) {
				t.Errorf("news %v reasons %v, want %v %v", rows[0].News, rows[0].Reasons, tc.news, tc.reasons)
			}
		})
	}
}

func TestRowsOrderAndDetails(t *testing.T) {
	older := thread("p", ann, []string{"assignee:guilbep"}, comment("p1", ann, now.Add(-3*time.Hour), "x"), comment("p2", ann, now.Add(-2*time.Hour), "y"))
	newer := thread("q", ann, []string{"assignee:guilbep"}, comment("q1", ann, now.Add(-time.Hour), "z"))
	rows := inbox.Rows(me, inbox.State{}, []gitbugio.Thread{older, newer}, nil, cfg, since)
	if len(rows) != 2 || rows[0].Issue.ID != "q" || rows[1].Issue.ID != "p" {
		t.Fatalf("order: %+v, want q then p", rows)
	}
	p := rows[1]
	if p.First != "p1" || !p.Latest.Equal(now.Add(-2*time.Hour)) || len(p.Who) != 1 || p.Who[0].ID != "ann" || !slices.Equal(p.News, []string{"2 comments"}) {
		t.Errorf("p: first %s latest %v who %v news %v", p.First, p.Latest, p.Who, p.News)
	}
}

func TestFollows(t *testing.T) {
	th := thread("o", ann, []string{"assignee:guilbep"}, comment("o1", me2, now.Add(-time.Hour), "from GitHub"))
	following, reasons, set := inbox.Follows(me, inbox.State{}, th, nil, cfg)
	if !following || !slices.Equal(reasons, []string{"assignee", "commented"}) || set != nil {
		t.Errorf("Follows = %v %v %v, want true [assignee commented] nil", following, reasons, set)
	}
	following, _, set = inbox.Follows(me, inbox.State{Unfollow: []string{"o"}}, th, nil, cfg)
	if following || set == nil || *set {
		t.Errorf("unfollowed: %v %v", following, set)
	}
}
```

- [ ] **Step 2: Run it to see it fail**

Run: `go test ./internal/inbox/ -run 'TestRows|TestFollows' -v`
Expected: build failure, `undefined: inbox.Rows`.

- [ ] **Step 3: Write `internal/inbox/rows.go`**

```go
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
```

- [ ] **Step 4: Run the tests to see them pass**

Run: `go test ./internal/inbox/ -v`
Expected: PASS (all of `TestRows`'s subtests, `TestRowsOrderAndDetails`, `TestFollows`, and Task 3's).

- [ ] **Step 5: Commit**

```bash
git add internal/inbox
git commit -m "feat(inbox): the rows: what happened that concerns you

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 5: `follows:@me`, and the followed views

**Files:**
- Modify: `internal/filter/filter.go`
- Modify: `internal/board/board.go` (`Match`, and a new `FollowedViews`)
- Test: `internal/filter/involves_test.go` (append), `internal/board/search_test.go` (append)
- Modify: `docs/configuration.md`

**Interfaces:**
- Consumes: `inbox.Load`, `State` (Task 3); `filter.KindFollows` (declared in Task 2).
- Produces: `filter.Followed{Follow, Unfollow, InViews map[string]bool}`; `Condition.Followed *Followed`; `board.FollowedViews(b any, cfg *config.Config, views []string, me string, issues []gitbugio.Issue, today time.Time) (inViews map[string][]string, missing []string, err error)`.

- [ ] **Step 1: Write the failing tests.** Append to `internal/filter/involves_test.go`:

```go
func TestFollowsTerm(t *testing.T) {
	cfg, now := &config.Config{}, time.Now()
	assigned := gitbugio.Issue{ID: "a1", Labels: []string{"assignee:guilbep"}}
	unfollowed := gitbugio.Issue{ID: "b2", Labels: []string{"assignee:guilbep"}}
	followed := gitbugio.Issue{ID: "c3"}
	inView := gitbugio.Issue{ID: "d4"}
	nothing := gitbugio.Issue{ID: "e5"}

	conds := filter.Parse("follows:@me")
	conds[0].Followed = &filter.Followed{
		Follow:   map[string]bool{"c3": true},
		Unfollow: map[string]bool{"b2": true},
		InViews:  map[string]bool{"d4": true},
	}
	for _, tc := range []struct {
		issue gitbugio.Issue
		want  bool
	}{{assigned, true}, {unfollowed, false}, {followed, true}, {inView, true}, {nothing, false}} {
		if got := filter.Matches(tc.issue, conds, cfg, "guilbep", now); got != tc.want {
			t.Errorf("follows:@me on %s = %v, want %v", tc.issue.ID, got, tc.want)
		}
	}
	if filter.Matches(assigned, filter.Parse("follows:ann"), cfg, "guilbep", now) {
		t.Error("follows: is yours only: follows:ann matches nothing")
	}
	if !filter.RequiresMe(filter.Parse("follows:@me")) {
		t.Error("follows:@me needs to know who you are")
	}
}
```

Append to `internal/board/search_test.go` (add `"github.com/guilbep/git-board/internal/inbox"` to its imports):

```go
// follows:@me reads your inbox file; a followed view counts without its
// is: (a close still shows) and without its own follows: (no loop).
func TestFollowsOnARepository(t *testing.T) {
	b := gitbugiotest.New(t)
	assigned, _ := b.NewIssue("Train legs", "", []string{"assignee:guilbep"})
	unfollowed, _ := b.NewIssue("Buildings", "", []string{"assignee:guilbep"})
	followed, _ := b.NewIssue("Food", "", nil)
	closedInView, _ := b.NewIssue("Cache factors", "", []string{"area:backend"})
	gitbugiotest.CloseIssue(t, b, closedInView)
	if _, err := b.NewIssue("Unrelated", "", nil); err != nil {
		t.Fatal(err)
	}
	me, _, _ := b.Me()
	if err := inbox.Save(b.Storage(), me.ID, inbox.State{Follow: []string{followed}, Unfollow: []string{unfollowed}, Views: []string{"backend", "gone"}}); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Views: map[string]config.View{"backend": {Filter: "is:open area:backend follows:@me"}}}

	got, err := board.LoadMatching(b, cfg, "follows:@me", "guilbep")
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, i := range got {
		ids[i.ID] = true
	}
	if !ids[assigned] || !ids[followed] || !ids[closedInView] || ids[unfollowed] || len(ids) != 3 {
		t.Errorf("follows:@me = %v, want %s, %s, %s", ids, assigned, followed, closedInView)
	}

	all, _ := b.ListIssues()
	inViews, missing, err := board.FollowedViews(b, cfg, []string{"backend", "gone"}, "guilbep", all, time.Now())
	if err != nil || !slices.Equal(inViews[closedInView], []string{"backend"}) || !slices.Equal(missing, []string{"gone"}) {
		t.Errorf("FollowedViews = %v %v %v", inViews, missing, err)
	}
}
```

(add `"slices"` to `search_test.go`'s imports.)

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/filter/ ./internal/board/ -run 'TestFollowsTerm|TestFollowsOnARepository' -v`
Expected: build failure, `undefined: filter.Followed`.

- [ ] **Step 3: Implement the term in `internal/filter/filter.go`**

Add to `Condition`:

```go
	// Followed is, for follows:@me, what your inbox follows besides what
	// you're in (board fills it); nil: only what you're in.
	Followed *Followed
```

and the type, next to `Involved`:

```go
// Followed is your inbox's file as follows:@me needs it: what you
// followed and unfollowed by hand, and the issues of the views you follow.
type Followed struct {
	Follow, Unfollow, InViews map[string]bool
}
```

In `Parse`, before `default:`:

```go
		case fieldName == "follows":
			conditions = append(conditions, Condition{Kind: KindFollows, Value: value, Negated: negated})
```

In `RequiresMe`, widen to `if c.Kind == KindField || c.Kind == KindAuthor || c.Kind == KindInvolves || c.Kind == KindFollows {`; in `Matches`' `values` condition add `&& c.Kind != KindFollows`.

In `matchOne`, after `case KindInvolves:`:

```go
	case KindFollows: // yours only: follows:someone matches nothing
		f := c.Followed
		if f == nil {
			f = &Followed{}
		}
		in := Involved{AuthorLogins: issue.AuthorLogins, Labels: issue.Labels, CommenterLogins: issue.CommenterLogins, Mentioned: c.Mentions[issue.ID]}
		ok = value == "@me" && !f.Unfollow[issue.ID] &&
			(f.Follow[issue.ID] || f.InViews[issue.ID] || len(Involvement(in, me, cfg)) > 0)
```

- [ ] **Step 4: Fill it in `internal/board/board.go`.** Add imports `"errors"`, `"github.com/git-bug/git-bug/repository"`, `"github.com/guilbep/git-board/internal/inbox"`. In `Match`, after `prepare` and before the loop:

```go
	if err := fillFollows(b, cfg, conditions, me, issues, today); err != nil {
		return nil, err
	}
```

and below `Match`:

```go
// fillFollows gives follows:@me your inbox's file (internal/inbox): what
// you followed and unfollowed, and the issues of the views you follow.
func fillFollows(b any, cfg *config.Config, conditions []filter.Condition, me string, issues []gitbugio.Issue, today time.Time) error {
	if !slices.ContainsFunc(conditions, func(c filter.Condition) bool { return c.Kind == filter.KindFollows }) {
		return nil
	}
	repo, ok := b.(interface {
		Me() (gitbugio.Person, bool, error)
		Storage() repository.LocalStorage
	})
	if !ok {
		return errors.New("follows:@me needs a repository")
	}
	p, set, err := repo.Me()
	if err != nil {
		return err
	}
	if !set {
		return errors.New("follows:@me: choose who you write as first")
	}
	st, err := inbox.Load(repo.Storage(), p.ID)
	if err != nil {
		return err
	}
	inViews, _, err := FollowedViews(b, cfg, st.Views, me, issues, today)
	if err != nil {
		return err
	}
	f := &filter.Followed{Follow: setOf(st.Follow), Unfollow: setOf(st.Unfollow), InViews: map[string]bool{}}
	for id := range inViews {
		f.InViews[id] = true
	}
	for i := range conditions {
		if conditions[i].Kind == filter.KindFollows {
			conditions[i].Followed = f
		}
	}
	return nil
}

// FollowedViews is, by issue id, the followed views among issues each is
// in, and the followed views no longer in the config. A view matches
// without its is: (a close still shows) and its follows: (it can't follow
// itself).
func FollowedViews(b any, cfg *config.Config, views []string, me string, issues []gitbugio.Issue, today time.Time) (map[string][]string, []string, error) {
	inViews := map[string][]string{}
	var missing []string
	for _, name := range views {
		v, ok := cfg.Views[name]
		if !ok {
			missing = append(missing, name)
			continue
		}
		text := filter.Without(v.Filter, func(c filter.Condition) bool { return c.Kind == filter.KindIs || c.Kind == filter.KindFollows })
		matched, err := Match(b, cfg, text, me, issues, today)
		if err != nil {
			return nil, nil, err
		}
		for _, m := range matched {
			inViews[m.ID] = append(inViews[m.ID], name)
		}
	}
	return inViews, missing, nil
}

func setOf(ids []string) map[string]bool {
	res := make(map[string]bool, len(ids))
	for _, id := range ids {
		res[id] = true
	}
	return res
}
```

`internal/inbox` imports `internal/filter`, never `internal/board`: keep it that way, or this import cycles.

- [ ] **Step 5: Run the tests to see them pass**

Run: `go test ./internal/... -v -run 'TestFollowsTerm|TestFollowsOnARepository'` then `go test ./internal/... ./cmd/...`
Expected: PASS.

- [ ] **Step 6: Document it** in `docs/configuration.md`, after the `involves:` row:

```markdown
| `follows:@me` | what your inbox follows: `involves:@me`, the issues you followed and the views you follow, without those you unfollowed |
```

- [ ] **Step 7: Commit**

```bash
git add internal/filter internal/board docs/configuration.md
git commit -m "feat(inbox): follows:@me, what your inbox follows

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 6: The server's five endpoints

**Files:**
- Create: `cmd/git-board-server/inbox.go`
- Create: `cmd/git-board-server/inbox_test.go`
- Modify: `cmd/git-board-server/server.go` (`apiServer`, `newMux`)
- Modify: `cmd/git-board-server/events.go` (`eventFor`)

**Interfaces:**
- Consumes: everything above; `s.b.Me()`, `s.b.MyLogin()`, `s.b.Thread`, `s.b.ListIssues`, `s.b.Storage()`, `writeJSON`, `writeError`, `writeCoded`, `decode`.
- Produces (HTTP):
  - `GET /api/inbox` → `inboxResponse{me, rows, fields, views, views_missing, following}` (`following`: how many open issues `follows:@me` matches, for the empty state);
  - `POST /api/inbox/seen` `{"issues": [...]}` → `{}`;
  - `GET /api/inbox/follow/{id}` → `followResponse{following, reasons, set}`;
  - `PUT /api/inbox/follow/{id}` `{"follow": true|false|null}` → `followResponse`;
  - `PUT /api/inbox/settings` `{"views": [...], "fields": bool}` → `{}`.
  - Writes answer JSON (`{}`), not `204`: the web client's `request` always parses a body.
  - SSE: writes under `/api/inbox` publish `{"kind": "inbox"}`.

- [ ] **Step 1: Write the failing tests** in `cmd/git-board-server/inbox_test.go`:

```go
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/git-bug/git-bug/repository/repotest"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio"
	"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"
	"github.com/guilbep/git-board/internal/inbox"
)

func getInbox(t *testing.T, ts *httptest.Server) inboxResponse {
	t.Helper()
	var res inboxResponse
	if code := call(t, "GET", ts.URL+"/api/inbox", nil, &res); code != http.StatusOK {
		t.Fatalf("GET /api/inbox: %d", code)
	}
	return res
}

func TestInboxFlow(t *testing.T) {
	ts, s := newIssueServer(t) // writes as the test identity, me: guilbep
	id, _ := s.b.NewIssue("Train legs", "", []string{"assignee:guilbep"})
	ann := gitbugiotest.NewPerson(t, s.b, "Ann", "ann")
	gitbugiotest.CommentAs(t, s.b, id, ann, time.Now().Unix(), "first")

	res := getInbox(t, ts)
	// author: you opened it (the test identity); assignee: the label
	if len(res.Rows) != 1 || !slices.Equal(res.Rows[0].News, []string{"1 comment"}) || !slices.Equal(res.Rows[0].Reasons, []string{"author", "assignee"}) || res.Following != 1 {
		t.Fatalf("inbox: %+v", res)
	}

	if code := call(t, "POST", ts.URL+"/api/inbox/seen", map[string]any{"issues": []string{id}}, &struct{}{}); code != http.StatusOK {
		t.Fatalf("seen: %d", code)
	}
	if res := getInbox(t, ts); len(res.Rows) != 0 {
		t.Fatalf("after seen: %+v", res.Rows)
	}

	var f followResponse
	if code := call(t, "PUT", ts.URL+"/api/inbox/follow/"+id, map[string]any{"follow": false}, &f); code != http.StatusOK || f.Following || f.Set == nil || *f.Set {
		t.Fatalf("unfollow: %d %+v", code, f)
	}
	gitbugiotest.CommentAs(t, s.b, id, ann, time.Now().Unix(), "second")
	if res := getInbox(t, ts); len(res.Rows) != 0 {
		t.Fatalf("unfollowed, yet: %+v", res.Rows)
	}
	if code := call(t, "PUT", ts.URL+"/api/inbox/follow/"+id, map[string]any{"follow": nil}, &f); code != http.StatusOK || !f.Following || f.Set != nil {
		t.Fatalf("back to automatic: %d %+v", code, f)
	}
	if res := getInbox(t, ts); len(res.Rows) != 1 {
		t.Fatalf("followed again: %+v", res.Rows)
	}

	if code := call(t, "PUT", ts.URL+"/api/inbox/settings", map[string]any{"views": []string{"gone"}, "fields": true}, &struct{}{}); code != http.StatusOK {
		t.Fatalf("settings: %d", code)
	}
	if res := getInbox(t, ts); !res.Fields || !slices.Equal(res.ViewsMissing, []string{"gone"}) {
		t.Fatalf("settings: %+v", res)
	}
}

// codeOf is the error code a request answers.
func codeOf(t *testing.T, url string) (int, string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body struct{ Code string }
	_ = json.NewDecoder(resp.Body).Decode(&body)
	return resp.StatusCode, body.Code
}

func TestInboxNeedsWhoYouAre(t *testing.T) {
	noLogin := &apiServer{cfg: &config.Config{}, b: gitbugiotest.New(t)} // its identity has no login, no --me
	ts := httptest.NewServer(withCORS(newMux(noLogin, http.NotFoundHandler())))
	t.Cleanup(ts.Close)
	if code, c := codeOf(t, ts.URL+"/api/inbox"); code != http.StatusConflict || c != "no-login" {
		t.Errorf("no login: %d %q", code, c)
	}

	b, err := gitbugio.NewForRepo(repotest.NewGoGitRepo(t, false)) // no identity chosen
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	ts2 := httptest.NewServer(withCORS(newMux(&apiServer{cfg: &config.Config{}, b: b, me: "guilbep"}, http.NotFoundHandler())))
	t.Cleanup(ts2.Close)
	if code, c := codeOf(t, ts2.URL+"/api/inbox"); code != http.StatusConflict || c != "no-identity" {
		t.Errorf("no identity: %d %q", code, c)
	}
}

// Review Focus 1: switching who you write as switches inbox.
func TestInboxIsPerIdentity(t *testing.T) {
	ts, s := newIssueServer(t)
	id, _ := s.b.NewIssue("Train legs", "", []string{"assignee:guilbep"})
	first, _, _ := s.b.Me()
	ann := gitbugiotest.NewPerson(t, s.b, "Ann", "ann")

	if code := call(t, "POST", ts.URL+"/api/me", map[string]any{"id": ann.Id().String()}, nil); code != http.StatusOK {
		t.Fatalf("adopt ann: %d", code)
	}
	if code := call(t, "PUT", ts.URL+"/api/inbox/follow/"+id, map[string]any{"follow": false}, nil); code != http.StatusOK {
		t.Fatalf("ann unfollows: %d", code)
	}
	if code := call(t, "POST", ts.URL+"/api/me", map[string]any{"id": first.ID}, nil); code != http.StatusOK {
		t.Fatalf("adopt back: %d", code)
	}
	var f followResponse
	call(t, "GET", ts.URL+"/api/inbox/follow/"+id, nil, &f)
	if f.Set != nil || !f.Following {
		t.Errorf("ann's Unfollow reached the first identity's inbox: %+v", f)
	}
}

// Review Focus 2: two tabs marking at once both stick.
func TestInboxSeenFromTwoTabs(t *testing.T) {
	ts, s := newIssueServer(t)
	a, _ := s.b.NewIssue("A", "", []string{"assignee:guilbep"})
	b, _ := s.b.NewIssue("B", "", []string{"assignee:guilbep"})
	var wg sync.WaitGroup
	for _, id := range []string{a, b} {
		wg.Add(1)
		go func() { // no t.Fatal off the test's goroutine: plain requests
			defer wg.Done()
			req, _ := http.NewRequest("POST", ts.URL+"/api/inbox/seen", strings.NewReader(`{"issues":["`+id+`"]}`))
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Errorf("seen %s: %v", id, err)
				return
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Errorf("seen %s: %d", id, resp.StatusCode)
			}
		}()
	}
	wg.Wait()
	me, _, _ := s.b.Me()
	st, err := inbox.Load(s.b.Storage(), me.ID)
	if err != nil || len(st.Seen[a]) == 0 || len(st.Seen[b]) == 0 {
		t.Errorf("seen after two tabs: %v %v", st.Seen, err)
	}
}

// Review Focus 3 and 4: prefixes are stored whole; an unknown issue
// changes nothing.
func TestInboxTakesPrefixes(t *testing.T) {
	ts, s := newIssueServer(t)
	id, _ := s.b.NewIssue("Train legs", "", nil)
	if code := call(t, "PUT", ts.URL+"/api/inbox/follow/"+id[:7], map[string]any{"follow": true}, nil); code != http.StatusOK {
		t.Fatalf("follow by prefix: %d", code)
	}
	me, _, _ := s.b.Me()
	st, _ := inbox.Load(s.b.Storage(), me.ID)
	if !slices.Equal(st.Follow, []string{id}) {
		t.Errorf("stored %v, want the full id %s", st.Follow, id)
	}
	if code := call(t, "PUT", ts.URL+"/api/inbox/follow/deadbeef", map[string]any{"follow": true}, nil); code != http.StatusNotFound {
		t.Errorf("unknown issue: %d, want 404", code)
	}
	if code := call(t, "POST", ts.URL+"/api/inbox/seen", map[string]any{"issues": []string{"deadbeef"}}, nil); code != http.StatusNotFound {
		t.Errorf("unknown issue seen: %d, want 404", code)
	}
	if after, _ := inbox.Load(s.b.Storage(), me.ID); !slices.Equal(after.Follow, []string{id}) || len(after.Seen) != 0 {
		t.Errorf("an unknown issue changed the file: %+v", after)
	}
}
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./cmd/git-board-server/ -run 'TestInbox' -v`
Expected: build failure, `undefined: inboxResponse`.

- [ ] **Step 3: Write `cmd/git-board-server/inbox.go`**

```go
package main

import (
	"errors"
	"net/http"
	"time"

	"github.com/guilbep/git-board/internal/board"
	"github.com/guilbep/git-board/internal/gitbugio"
	"github.com/guilbep/git-board/internal/inbox"
)

// The inbox (internal/inbox): what happened that concerns you on the
// issues of the last 14 days. Your file is yours, not the repository's:
// it takes inboxMu, never writeMu, and answers during a forge sync.

type inboxResponse struct {
	Me           inbox.Me    `json:"me"`
	Rows         []inbox.Row `json:"rows"`
	Fields       bool        `json:"fields"`
	Views        []string    `json:"views"`
	ViewsMissing []string    `json:"views_missing"`
	Following    int         `json:"following"` // open issues follows:@me matches: the empty state's
}

type followResponse struct {
	Following bool     `json:"following"`
	Reasons   []string `json:"reasons"`
	Set       *bool    `json:"set"` // what you clicked; null: automatic
}

// inboxMe is who the inbox is for, or the answer saying why there's none.
func (s *apiServer) inboxMe(w http.ResponseWriter) (inbox.Me, bool) {
	p, ok, err := s.b.Me()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return inbox.Me{}, false
	}
	if !ok {
		writeCoded(w, http.StatusConflict, "no-identity", errors.New("the inbox is the identity you write as: choose one in Settings"))
		return inbox.Me{}, false
	}
	login := s.me
	if login == "" {
		login = s.b.MyLogin()
	}
	if login == "" {
		writeCoded(w, http.StatusConflict, "no-login", errors.New("your identity has no forge login, so no field and no mention can name you: git bug bridge new tags it, or start the server with --me"))
		return inbox.Me{}, false
	}
	return inbox.Me{ID: p.ID, Login: login}, true
}

func (s *apiServer) loadInbox(id string) (inbox.State, error) {
	s.inboxMu.Lock()
	defer s.inboxMu.Unlock()
	return inbox.Load(s.b.Storage(), id)
}

// changeInbox loads your file, changes it and saves it, in one hold of
// inboxMu: two tabs writing at once both stick.
func (s *apiServer) changeInbox(id string, change func(*inbox.State)) error {
	s.inboxMu.Lock()
	defer s.inboxMu.Unlock()
	st, err := inbox.Load(s.b.Storage(), id)
	if err != nil {
		return err
	}
	change(&st)
	return inbox.Save(s.b.Storage(), id, st)
}

// recent is the issues updated within the window.
func recent(all []gitbugio.Issue, since time.Time) []gitbugio.Issue {
	var res []gitbugio.Issue
	for _, i := range all {
		if !i.UpdatedAt.Before(since) {
			res = append(res, i)
		}
	}
	return res
}

func (s *apiServer) handleInbox(w http.ResponseWriter, r *http.Request) {
	me, ok := s.inboxMe(w)
	if !ok {
		return
	}
	res, err := s.inbox(me, time.Now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// inbox gathers the window's threads and the followed views they're in,
// and asks inbox.Rows.
// ponytail: recomputed on every request, the header's count included;
// keep the last result per identity until the refs (RefsFingerprint), the
// file or the config change, if a big repository makes it slow.
func (s *apiServer) inbox(me inbox.Me, now time.Time) (inboxResponse, error) {
	st, err := s.loadInbox(me.ID)
	if err != nil {
		return inboxResponse{}, err
	}
	cfg := s.config()
	since := now.Add(-inbox.Window)
	all, err := s.b.ListIssues()
	if err != nil {
		return inboxResponse{}, err
	}
	window := recent(all, since)
	threads := make([]gitbugio.Thread, 0, len(window))
	for _, i := range window {
		th, err := s.b.Thread(i.ID)
		if err != nil {
			return inboxResponse{}, err
		}
		threads = append(threads, th)
	}
	inViews, missing, err := board.FollowedViews(s.b, cfg, st.Views, me.Login, window, now)
	if err != nil {
		return inboxResponse{}, err
	}
	following, err := board.Match(s.b, cfg, "follows:@me is:open", me.Login, all, now)
	if err != nil {
		return inboxResponse{}, err
	}
	res := inboxResponse{Me: me, Rows: inbox.Rows(me, st, threads, inViews, cfg, since), Fields: st.Fields,
		Views: st.Views, ViewsMissing: missing, Following: len(following)}
	if res.Rows == nil {
		res.Rows = []inbox.Row{}
	}
	if res.Views == nil {
		res.Views = []string{}
	}
	if res.ViewsMissing == nil {
		res.ViewsMissing = []string{}
	}
	return res, nil
}

// handleInboxSeen marks every event the issues have now as seen: an issue
// opened, or Mark all read.
func (s *apiServer) handleInboxSeen(w http.ResponseWriter, r *http.Request) {
	me, ok := s.inboxMe(w)
	if !ok {
		return
	}
	var req struct {
		Issues []string `json:"issues"`
	}
	if !decode(w, r, &req) {
		return
	}
	threads := make([]gitbugio.Thread, 0, len(req.Issues))
	for _, id := range req.Issues {
		th, err := s.b.Thread(id)
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
		threads = append(threads, th)
	}
	all, err := s.b.ListIssues()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	keep := map[string]bool{}
	for _, i := range recent(all, time.Now().Add(-inbox.Window)) {
		keep[i.ID] = true
	}
	err = s.changeInbox(me.ID, func(st *inbox.State) {
		for _, th := range threads {
			st.MarkSeen(th)
		}
		st.Prune(keep)
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, struct{}{})
}

// follow is whether you follow the issue (an id or a prefix), and why.
func (s *apiServer) follow(me inbox.Me, th gitbugio.Thread) (followResponse, error) {
	st, err := s.loadInbox(me.ID)
	if err != nil {
		return followResponse{}, err
	}
	cfg := s.config()
	inViews, _, err := board.FollowedViews(s.b, cfg, st.Views, me.Login, []gitbugio.Issue{th.Issue}, time.Now())
	if err != nil {
		return followResponse{}, err
	}
	following, reasons, set := inbox.Follows(me, st, th, inViews[th.ID], cfg)
	if reasons == nil {
		reasons = []string{}
	}
	return followResponse{Following: following, Reasons: reasons, Set: set}, nil
}

func (s *apiServer) handleFollow(w http.ResponseWriter, r *http.Request) {
	me, ok := s.inboxMe(w)
	if !ok {
		return
	}
	th, err := s.b.Thread(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	res, err := s.follow(me, th)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleSetFollow is Follow (true), Unfollow (false), back to automatic
// (null), stored under the issue's full id.
func (s *apiServer) handleSetFollow(w http.ResponseWriter, r *http.Request) {
	me, ok := s.inboxMe(w)
	if !ok {
		return
	}
	var req struct {
		Follow *bool `json:"follow"`
	}
	if !decode(w, r, &req) {
		return
	}
	th, err := s.b.Thread(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err := s.changeInbox(me.ID, func(st *inbox.State) { st.SetFollow(th.ID, req.Follow) }); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	res, err := s.follow(me, th)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *apiServer) handleInboxSettings(w http.ResponseWriter, r *http.Request) {
	me, ok := s.inboxMe(w)
	if !ok {
		return
	}
	var req struct {
		Views  []string `json:"views"`
		Fields bool     `json:"fields"`
	}
	if !decode(w, r, &req) {
		return
	}
	if err := s.changeInbox(me.ID, func(st *inbox.State) { st.Views, st.Fields = req.Views, req.Fields }); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, struct{}{})
}
```

- [ ] **Step 4: Wire it.** In `server.go`'s `apiServer`, after `checked map[string]time.Time`:

```go
	// inboxMu guards the inbox files (inbox.go): one read-change-write at
	// a time
	inboxMu sync.Mutex
```

In `newMux`, after the `/api/me` routes:

```go
	mux.HandleFunc("GET /api/inbox", s.handleInbox)
	mux.HandleFunc("POST /api/inbox/seen", s.handleInboxSeen)
	mux.HandleFunc("GET /api/inbox/follow/{id}", s.handleFollow)
	mux.HandleFunc("PUT /api/inbox/follow/{id}", s.handleSetFollow)
	mux.HandleFunc("PUT /api/inbox/settings", s.handleInboxSettings)
```

In `events.go`'s `eventFor`, before `default:`:

```go
	case strings.HasPrefix(p, "/api/inbox"): // read, followed: your inbox's count, in every tab
		e.Kind = "inbox"
```

and extend the comment on `event` to name it: `event is what changed: issues (ID: which one, when the request named it), config (.git-board.toml, a view), sync, settings, inbox (yours: what you read or follow).`

- [ ] **Step 5: Run the tests to see them pass**

Run: `go test ./cmd/git-board-server/ -run 'TestInbox' -v -race` then `go test ./... -race`
Expected: PASS, no race reported.

- [ ] **Step 6: Commit**

```bash
git add cmd/git-board-server
git commit -m "feat(inbox): the server's inbox: rows, seen, follow, settings

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 7: The web client, and which remotes to check

**Files:**
- Modify: `web/src/api/board.ts` (append after the remotes block, line ~352)
- Create: `web/src/inboxcheck.ts`
- Create: `web/src/inboxcheck.test.ts`
- Create: `web/src/inbox.ts`
- Modify: `web/src/live.ts` (`LiveEvent.kind`)
- Modify: `web/package.json` (the `test` script)

**Interfaces:**
- Consumes: Task 6's endpoints; `Remote`, `remoteAction`, `ApiError` (existing).
- Produces: `InboxRow`, `InboxResponse`, `FollowState` types; `getInbox()`, `markSeen(issues)`, `getFollow(id)`, `setFollow(id, follow)`, `setInboxSettings(views, fields)`; `inboxcheck.ts`: `FRESH`, `toCheck(remotes, now, force): string[]`, `toPull(remotes): {name, issues}[]`, `checkFailure(name, code, message): string`; `inbox.ts`: `inbox` ref, `inboxError` ref, `loadInbox()`.

- [ ] **Step 1: Write the failing test** `web/src/inboxcheck.test.ts`:

```ts
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { FRESH, checkFailure, toCheck, toPull } from './inboxcheck.ts';
import type { Remote } from './api/board';

const remote = (name: string, checked: string | undefined, receive = 0): Remote => ({
  name, url: '', push_urls: [], tracked: true, issues: { send: 0, receive }, people: { send: 0, receive: 0 }, ...(checked ? { checked } : {}),
});

test('opening the inbox checks the remotes not checked lately; Refresh checks them all', () => {
  const now = Date.parse('2026-10-17T10:00:00Z');
  const rs = [
    remote('origin', new Date(now - FRESH + 1000).toISOString()), // a minute ago: a reload
    remote('upstream', new Date(now - FRESH - 1000).toISOString()),
    remote('backup', undefined), // never checked
  ];
  assert.deepEqual(toCheck(rs, now, false), ['upstream', 'backup']);
  assert.deepEqual(toCheck(rs, now, true), ['origin', 'upstream', 'backup']);
});

test('a remote with issues to receive offers Pull', () => {
  assert.deepEqual(toPull([remote('origin', undefined, 3), remote('upstream', undefined, 0)]), [{ name: 'origin', issues: 3 }]);
});

// Review Focus 5: a forge sync holds the remotes; the page says so.
test('a check refused during a forge sync says so', () => {
  assert.equal(checkFailure('origin', 'syncing', 'a sync with the forges is running'), 'origin: a forge sync is running: check again when it ends');
  assert.equal(checkFailure('upstream', '', 'fetch upstream: ssh: handshake failed'), "upstream: couldn't check: fetch upstream: ssh: handshake failed");
});
```

Add `src/inboxcheck.test.ts` to the end of the `test` script's file list in `web/package.json`.

- [ ] **Step 2: Run it to see it fail**

Run: `cd web && npm test`
Expected: FAIL, `Cannot find module './inboxcheck.ts'`.

- [ ] **Step 3: Write `web/src/inboxcheck.ts`**

```ts
// Which git remotes the inbox checks on opening (a fetch: nothing local
// changes), which offer Pull, and what a refused check says. Pure: the
// page calls the server.
import type { Remote } from './api/board';

// FRESH: a remote checked this recently isn't checked again on opening
// the inbox (a reload); Refresh checks regardless.
export const FRESH = 5 * 60 * 1000;

export function toCheck(remotes: Remote[], now: number, force: boolean): string[] {
  return remotes.filter((r) => force || !r.checked || now - Date.parse(r.checked) > FRESH).map((r) => r.name);
}

// toPull: the remotes with issues to receive, for "3 issues new · Pull".
export function toPull(remotes: Remote[]): { name: string; issues: number }[] {
  return remotes.filter((r) => r.issues.receive > 0).map((r) => ({ name: r.name, issues: r.issues.receive }));
}

// checkFailure: a remote's check refused. syncing: a forge sync holds the
// remotes until it ends (cmd/git-board-server beginWrite).
export function checkFailure(name: string, code: string, message: string): string {
  if (code === 'syncing') return `${name}: a forge sync is running: check again when it ends`;
  return `${name}: couldn't check: ${message}`;
}
```

- [ ] **Step 4: Run it to see it pass**

Run: `cd web && npm test`
Expected: PASS, every test file.

- [ ] **Step 5: Add the API calls** to `web/src/api/board.ts`, after `remoteAction`:

```ts
// The inbox (cmd/git-board-server/inbox.go): what happened that concerns
// you on the issues of the last 14 days, newest first.
export interface InboxRow {
  issue: Issue;
  reasons: string[]; // why you follow it; none: addressed to you only
  news: string[]; // "2 comments", "closed", "added you as reviewer"…
  who: Person[];
  latest: string;
  first: string; // the oldest of those events: the panel scrolls there
}

export interface InboxResponse {
  me: { id: string; login: string };
  rows: InboxRow[];
  fields: boolean; // also field changes on issues you're in
  views: string[]; // views followed
  views_missing: string[]; // followed, no longer in the config
  following: number; // open issues your inbox follows
}

export interface FollowState {
  following: boolean;
  reasons: string[];
  set: boolean | null; // what you clicked; null: automatic
}

export function getInbox(): Promise<InboxResponse> {
  return request('/api/inbox');
}

export function markSeen(issues: string[]): Promise<object> {
  return request('/api/inbox/seen', json('POST', { issues }));
}

const followPath = (id: string) => `/api/inbox/follow/${encodeURIComponent(id)}`;

export function getFollow(id: string): Promise<FollowState> {
  return request(followPath(id));
}

export function setFollow(id: string, follow: boolean | null): Promise<FollowState> {
  return request(followPath(id), json('PUT', { follow }));
}

export function setInboxSettings(views: string[], fields: boolean): Promise<object> {
  return request('/api/inbox/settings', json('PUT', { views, fields }));
}
```

- [ ] **Step 6: Write `web/src/inbox.ts`**, the ref the header and the page share:

```ts
// The inbox, shared by the header (its count) and the inbox page. Loading
// never checks the remotes: the page does that (inboxcheck.ts).
import { ref } from 'vue';
import { ApiError, getInbox, type InboxResponse } from '@/api/board';

export const inbox = ref<InboxResponse | null>(null);
// no-identity, no-login: the page says why; the header shows no count
export const inboxError = ref<{ code: string; message: string } | null>(null);

export async function loadInbox(): Promise<void> {
  try {
    inbox.value = await getInbox();
    inboxError.value = null;
  } catch (e) {
    inbox.value = null;
    inboxError.value = { code: e instanceof ApiError ? e.code : '', message: e instanceof Error ? e.message : String(e) };
  }
}
```

In `web/src/live.ts`, widen the kind: `kind: 'issues' | 'config' | 'sync' | 'settings' | 'inbox';`.

- [ ] **Step 7: Type-check and commit**

Run: `cd web && npm run typecheck && npm test`
Expected: no errors; PASS.

```bash
git add web/src/api/board.ts web/src/inboxcheck.ts web/src/inboxcheck.test.ts web/src/inbox.ts web/src/live.ts web/package.json
git commit -m "feat(inbox): the web's inbox calls, and which remotes to check

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 8: The Inbox page

**Files:**
- Create: `web/src/pages/InboxPage.vue`
- Modify: `web/src/router/routes.ts`
- Modify: `web/src/components/IssuePanel.vue` (props at line 157; a watch after `useIssue`)

**Interfaces:**
- Consumes: Task 7 (`inbox`, `inboxError`, `loadInbox`, `markSeen`, `setInboxSettings`, `getRemotes`, `remoteAction`, `toCheck`, `toPull`, `checkFailure`); `AppHeader`, `IssuePanel`, `GbIcon`; `namespaces` (`@/facets`); `getFields`, `getViews`; `initials`, `when` (`@/me`); `panelMode` (`@/panelsize`); `onLive`, `debounced` (`@/live`).
- Produces: the `/inbox` route; `IssuePanel`'s optional `at` prop (an event id to scroll to once loaded).

- [ ] **Step 1: Give `IssuePanel` its `at`.** Add to its `defineProps`: `at?: string; // an event to scroll to once loaded: the inbox's first new one`. After the `useIssue(...)` line, add (import `nextTick` and `watch` from `vue` if not already):

```ts
// the inbox opens an issue at its first new event
watch(
  () => thread.value?.id,
  (id) => {
    if (id && props.at) void nextTick(() => reveal(props.at!));
  },
);
```

- [ ] **Step 2: Add the route** in `web/src/router/routes.ts`, after `/insights`:

```ts
  { path: '/inbox', component: () => import('@/pages/InboxPage.vue') },
```

- [ ] **Step 3: Write `web/src/pages/InboxPage.vue`**

```vue
<!-- The inbox: what happened that concerns you on the issues of the last
     14 days (cmd/git-board-server/inbox.go), newest first. Opening it
     checks the git remotes (a fetch: nothing local changes) and offers
     Pull; opening a row opens the issue and marks it read. Read rows stay,
     dimmed, until Refresh, so the list doesn't jump while you read. -->
<template>
  <AppHeader :views="viewNames" active="" />
  <main class="gb-page gb-inbox">
    <div class="epfl-eyebrow">Inbox</div>
    <h1 class="h3">What happened that concerns you</h1>
    <p class="gb-lead">On the issues you follow, since you last opened them.</p>

    <div class="gb-inbox-bar">
      <div class="gb-inbox-status" role="status">
        <span v-for="c in checks" :key="c.name" :class="{ 'gb-inbox-fail': c.failure }">
          <template v-if="c.failure">{{ c.failure }}</template>
          <template v-else>
            Checked <strong>{{ c.name }}</strong> {{ c.when }}
            <template v-if="c.incoming">
              · {{ c.incoming }} {{ c.incoming === 1 ? 'issue' : 'issues' }} new ·
              <button type="button" class="epfl-button gb-inbox-pull" :disabled="busy" @click="pull(c.name)">Pull</button>
            </template>
          </template>
        </span>
      </div>
      <div class="gb-inbox-actions">
        <button type="button" class="epfl-button epfl-button--secondary" :disabled="busy" @click="refresh(true)">Refresh</button>
        <button type="button" class="epfl-button epfl-button--secondary" :disabled="!unread.length" @click="markAll">Mark all read</button>
        <button type="button" class="gb-settingslink" aria-label="Inbox settings" :aria-expanded="menuOpen" @click="menuOpen = !menuOpen">
          <GbIcon name="settings" small />
        </button>
      </div>
      <div v-if="menuOpen && data" class="gb-inbox-menu" role="group" aria-label="Inbox settings">
        <label class="gb-inbox-check">
          <input type="checkbox" :checked="data.fields" @change="saveSettings(data.views, !data.fields)" />
          <span><strong>Also field changes on issues I'm in</strong><br /><span class="gb-hint">Sprint, priority, status… where you're assignee, reviewer, pm…</span></span>
        </label>
        <fieldset class="gb-inbox-views">
          <legend><strong>Follow views</strong></legend>
          <label v-for="v in viewNames" :key="v" class="gb-inbox-check">
            <input type="checkbox" :checked="data.views.includes(v)" @change="toggleView(v)" /> {{ v }}
          </label>
          <span v-for="v in data.views_missing" :key="v" class="gb-inbox-check gb-hint">
            {{ v }} (no longer exists)
            <button type="button" class="gb-inbox-x" :aria-label="`Stop following ${v}`" @click="toggleView(v)">×</button>
          </span>
        </fieldset>
      </div>
    </div>

    <p v-if="error?.code === 'no-identity'" class="gb-inbox-need">
      The inbox shows what concerns the identity you write as, and none is chosen here yet.
      <router-link to="/settings/you">Choose who you write as</router-link>
    </p>
    <p v-else-if="error?.code === 'no-login'" class="gb-inbox-need">
      Tell git-board your forge login: without it, no field and no mention can name you. Run <code>git bug bridge new</code>, or start the server with <code>--me your-login</code>.
    </p>
    <p v-else-if="error" class="gb-error">{{ error.message }}</p>

    <ul v-if="rows.length" class="gb-inbox-rows">
      <li v-for="r in rows" :key="r.issue.id" :class="{ 'gb-inbox-read': read.has(r.issue.id) }">
        <button type="button" class="gb-inbox-row" @click="open(r)">
          <span class="gb-inbox-dot" aria-hidden="true" />
          <GbIcon :name="r.issue.status === 'closed' ? 'closed' : 'open'" small />
          <span class="gb-inbox-main">
            <span class="gb-inbox-title"><span v-if="r.issue.forges?.[0]?.number" class="gb-inbox-num">#{{ r.issue.forges?.[0]?.number }}</span> {{ r.issue.title }}</span>
            <span class="gb-inbox-news">
              {{ r.news.join(' · ') }}
              <span v-for="x in r.reasons" :key="x" class="gb-inbox-chip">{{ x }}</span>
            </span>
          </span>
          <span class="gb-inbox-who">
            <span v-for="p in r.who" :key="p.id" class="gb-avatar gb-avatar-sm gb-avatar-dark" :title="p.name">{{ initials(p.name) }}</span>
          </span>
          <span class="gb-hint gb-inbox-when">{{ when(r.latest, true) }}</span>
        </button>
      </li>
    </ul>
    <div v-else-if="data && !error" class="gb-inbox-empty">
      <h2 class="h4">Nothing new for you</h2>
      <p class="gb-hint">{{ followingLine }}</p>
    </div>
  </main>

  <q-dialog
    v-model="detailOpen"
    :position="panelMode === 'side' ? 'right' : 'standard'"
    :full-height="panelMode === 'side'"
    :class="`gb-dialog-${panelMode}`"
    :backdrop-filter="panelMode === 'wide' ? 'blur(6px)' : undefined"
    no-route-dismiss
    no-focus
    @hide="setIssue(undefined)"
  >
    <IssuePanel
      v-if="detailId"
      :id="detailId"
      :key="detailId"
      :names="names"
      :fields="fields"
      :title="detailRow?.issue.title ?? ''"
      crumb="Inbox"
      :position="index >= 0 ? `${index + 1} of ${rows.length}` : ''"
      :has-prev="index > 0"
      :has-next="index >= 0 && index < rows.length - 1"
      :at="detailRow?.first"
      @close="detailOpen = false"
      @prev="step(-1)"
      @next="step(1)"
      @open="(id: string) => setIssue(id)"
    />
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import AppHeader from '@/components/AppHeader.vue';
import GbIcon from '@/components/GbIcon.vue';
import IssuePanel from '@/components/IssuePanel.vue';
import { ApiError, getFields, getRemotes, getViews, markSeen, remoteAction, setInboxSettings, type InboxRow, type Remote } from '@/api/board';
import { namespaces } from '@/facets';
import type { Fields } from '@/fields';
import { inbox, inboxError, loadInbox } from '@/inbox';
import { checkFailure, toCheck, toPull } from '@/inboxcheck';
import { debounced, onLive } from '@/live';
import { initials, when } from '@/me';
import { panelMode } from '@/panelsize';

const route = useRoute();
const router = useRouter();

const viewNames = ref<string[]>([]);
const fields = ref<Fields>({});
const data = computed(() => inbox.value);
const error = computed(() => inboxError.value);
const rows = ref<InboxRow[]>([]);
const read = ref(new Set<string>());
const unread = computed(() => rows.value.filter((r) => !read.value.has(r.issue.id)).map((r) => r.issue.id));
const names = computed(() => namespaces(rows.value.map((r) => r.issue), fields.value));
const menuOpen = ref(false);
const busy = ref(false);

const remotes = ref<Remote[]>([]);
const failures = ref<Record<string, string>>({});
const checks = computed(() => {
  const incoming = new Map(toPull(remotes.value).map((p) => [p.name, p.issues]));
  return remotes.value.map((r) => ({
    name: r.name,
    when: r.checked ? when(r.checked, true) : 'not yet',
    incoming: incoming.get(r.name) ?? 0,
    failure: failures.value[r.name] ?? '',
  }));
});

const followingLine = computed(() => {
  const d = data.value;
  if (!d) return '';
  const parts = [`Following ${d.following} open ${d.following === 1 ? 'issue' : 'issues'}`];
  if (d.views.length) parts.push(`${d.views.length === 1 ? 'view' : 'views'} ${d.views.join(', ')}`);
  return parts.join(' · ');
});

// reload: the server's rows; dropRead (Refresh) forgets the rows read
// since, else they stay, dimmed, where they were.
async function reload(dropRead: boolean) {
  await loadInbox();
  const fresh = inbox.value?.rows ?? [];
  if (dropRead) {
    read.value = new Set();
    rows.value = fresh;
    return;
  }
  const kept = rows.value.filter((r) => read.value.has(r.issue.id) && !fresh.some((f) => f.issue.id === r.issue.id));
  rows.value = [...fresh, ...kept].sort((a, b) => Date.parse(b.latest) - Date.parse(a.latest));
}

// refresh: check the remotes (all of them when force: Refresh), then the rows.
async function refresh(force: boolean) {
  busy.value = true;
  try {
    remotes.value = (await getRemotes()).remotes;
    const toFetch = toCheck(remotes.value, Date.now(), force);
    const results = await Promise.allSettled(toFetch.map((n) => remoteAction(n, 'fetch')));
    const failed: Record<string, string> = {};
    results.forEach((res, i) => {
      const name = toFetch[i]!;
      if (res.status === 'rejected') {
        const e = res.reason as unknown;
        failed[name] = checkFailure(name, e instanceof ApiError ? e.code : '', e instanceof Error ? e.message : String(e));
      }
    });
    failures.value = failed;
    remotes.value = (await getRemotes()).remotes;
  } catch {
    // no remotes listed: the rows still show
  } finally {
    busy.value = false;
  }
  await reload(force);
}

async function pull(name: string) {
  busy.value = true;
  try {
    remotes.value = (await remoteAction(name, 'pull')).remotes;
    delete failures.value[name];
  } catch (e) {
    failures.value = { ...failures.value, [name]: e instanceof Error ? `${name}: couldn't pull: ${e.message}` : String(e) };
  } finally {
    busy.value = false;
  }
  await reload(false);
}

async function open(r: InboxRow) {
  setIssue(r.issue.id);
  if (read.value.has(r.issue.id)) return;
  read.value = new Set([...read.value, r.issue.id]);
  await markSeen([r.issue.id]);
  await loadInbox(); // the header's count: this tab's own write comes back skipped (live.ts)
}

async function markAll() {
  const ids = unread.value;
  read.value = new Set([...read.value, ...ids]);
  await markSeen(ids);
  await loadInbox();
}

async function saveSettings(views: string[], f: boolean) {
  await setInboxSettings(views, f);
  await reload(false);
}

function toggleView(v: string) {
  const d = data.value;
  if (!d) return;
  const views = d.views.includes(v) ? d.views.filter((x) => x !== v) : [...d.views, v];
  void saveSettings(views, d.fields);
}

// The issue panel: ?issue=<id>, as on the board; j / k go through the rows.
const detailOpen = ref(false);
const detailId = ref('');
const detailRow = computed(() => rows.value.find((r) => r.issue.id === detailId.value));
const index = computed(() => rows.value.findIndex((r) => r.issue.id === detailId.value));
function setIssue(id: string | undefined) {
  void router.replace({ query: { ...route.query, issue: id } });
}
function step(delta: number) {
  const next = rows.value[index.value + delta];
  if (next) void open(next);
}
watch(
  () => route.query.issue,
  (id) => {
    if (typeof id === 'string' && id) {
      detailId.value = id;
      detailOpen.value = true;
    } else detailOpen.value = false;
  },
  { immediate: true },
);

const reloadSoon = debounced(() => void reload(false));
let stopLive = () => {};
onMounted(async () => {
  stopLive = onLive((e) => {
    if (e.kind === 'issues' || e.kind === 'inbox') reloadSoon();
  });
  try {
    viewNames.value = Object.keys(await getViews()).sort();
    fields.value = await getFields();
  } catch {
    // no config: no views to follow, fields shown as labels
  }
  await refresh(false);
});
onBeforeUnmount(() => stopLive());
</script>

<style scoped>
.gb-inbox { max-width: 60rem; }
.gb-inbox-bar { position: relative; display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-2) var(--space-3); padding: var(--space-2) 0; border-top: var(--border-w) solid var(--border); border-bottom: var(--border-w) solid var(--border); }
.gb-inbox-status { display: flex; flex-direction: column; gap: var(--space-1); flex: 1 1 20rem; font-size: .875rem; color: var(--fg-muted); }
.gb-inbox-status strong { color: var(--fg); }
.gb-inbox-fail { color: var(--danger-fg); }
.gb-inbox-pull { min-height: 2rem; padding: 0 var(--space-2); }
.gb-inbox-actions { display: flex; flex-wrap: wrap; gap: var(--space-2); }
.gb-inbox-menu { position: absolute; right: 0; top: 100%; z-index: 5; width: min(20rem, calc(100vw - 2rem)); margin-top: var(--space-2); padding: var(--space-3); display: flex; flex-direction: column; gap: var(--space-3); background: var(--bg); border: var(--border-w) solid var(--border-strong); }
.gb-inbox-views { display: flex; flex-direction: column; gap: var(--space-2); margin: 0; padding: 0; border: 0; }
.gb-inbox-check { display: flex; gap: var(--space-2); align-items: flex-start; font-size: .875rem; }
.gb-inbox-x { width: 1.75rem; height: 1.75rem; border: var(--border-w) solid var(--border); background: var(--bg); color: var(--fg); cursor: pointer; }
.gb-inbox-need { margin: var(--space-4) 0; }
.gb-inbox-rows { list-style: none; margin: 0; padding: 0; }
.gb-inbox-rows li { border-bottom: var(--border-w) solid var(--border-subtle); }
.gb-inbox-row { display: flex; flex-wrap: wrap; align-items: flex-start; gap: var(--space-2) var(--space-3); width: 100%; padding: var(--space-3) var(--space-2); border: 0; background: transparent; color: var(--fg); font: inherit; text-align: left; cursor: pointer; }
.gb-inbox-row:hover { background: var(--bg-subtle); }
.gb-inbox-dot { width: .5rem; height: .5rem; margin-top: .4rem; border-radius: 50%; background: var(--primary); }
.gb-inbox-main { display: flex; flex-direction: column; gap: var(--space-1); flex: 999 1 24rem; min-width: 0; }
.gb-inbox-title { font-weight: var(--w-bold); }
.gb-inbox-num { font-family: var(--font-mono); font-weight: normal; color: var(--fg-muted); }
.gb-inbox-news { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-1) var(--space-2); font-size: .875rem; }
.gb-inbox-chip { padding: 0 .375rem; border: var(--border-w) solid var(--border); border-radius: var(--radius); font-size: .75rem; color: var(--fg-muted); }
.gb-inbox-who { display: flex; gap: 2px; }
.gb-inbox-when { min-width: 5rem; text-align: right; }
.gb-inbox-read { opacity: .55; }
.gb-inbox-read .gb-inbox-dot { background: transparent; }
.gb-inbox-read .gb-inbox-title { font-weight: normal; }
.gb-inbox-empty { padding: var(--space-5) 0; text-align: center; }
</style>
```

- [ ] **Step 4: Type-check and lint**

Run: `cd web && npm run typecheck && npm run lint`
Expected: no errors. (If `lint` isn't a script in `web/package.json`, typecheck alone.)

- [ ] **Step 5: Try it by hand.** Build and run against a repo with recent activity (a scratch clone of git-board's own issues will do; never co2-calculator's remotes, and nothing sends email):

```bash
make build   # or: cd web && npm run build && cd .. && go build ./cmd/git-board-server
./git-board-server --addr localhost:8090
```

Open `http://localhost:8090/#/inbox`. Check: the status line lists each remote as checked; a row opens the panel scrolled to its first new event and dims; Mark all read dims all; Refresh drops dimmed rows; the gear menu's checkbox and views save and survive a reload; with no identity chosen, the page says so.

- [ ] **Step 6: Commit**

```bash
git add web/src/pages/InboxPage.vue web/src/router/routes.ts web/src/components/IssuePanel.vue
git commit -m "feat(inbox): the inbox page

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 9: The header's count, the tab title, the palette

**Files:**
- Modify: `web/src/components/AppHeader.vue` (nav after the Conflicts link, line ~29; script)
- Modify: `web/src/components/GbIcon.vue` (`IconName`, `paths`)
- Modify: `web/src/components/CommandPalette.vue` (entries, line ~156)

**Interfaces:**
- Consumes: `inbox`, `loadInbox` (Task 7); `onLive`, `debounced`.
- Produces: the `inbox` icon.

- [ ] **Step 1: Add the icon** in `GbIcon.vue`: append `| 'inbox'` to `IconName`, and to `paths`:

```ts
  inbox: ['M22 12h-6l-2 3h-4l-2-3H2', 'M5.45 5.11 2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.45-6.89A2 2 0 0 0 16.76 4H7.24a2 2 0 0 0-1.79 1.11Z'],
```

- [ ] **Step 2: The palette entry** in `CommandPalette.vue`, after the `insights` entry:

```ts
  { key: 'inbox', label: 'Inbox', group: 'Commands', icon: 'inbox', act: go('/inbox') },
```

- [ ] **Step 3: The header link.** In `AppHeader.vue`'s nav, after the Conflicts `router-link`:

```vue
      <router-link to="/inbox" active-class="active">Inbox<span v-if="inboxCount" class="nav-count" :aria-label="`, ${inboxCount} new`">{{ inboxCount }}</span></router-link>
```

In its script, add `watch` to the `vue` import and:

```ts
import { inbox, loadInbox } from '@/inbox';
import { debounced, onLive } from '@/live';

const inboxCount = computed(() => inbox.value?.rows.length ?? 0);
// the tab says it too: "(3) git-board"
const baseTitle = document.title.replace(/^\(\d+\) /, '');
watch(inboxCount, (n) => (document.title = n ? `(${n}) ${baseTitle}` : baseTitle), { immediate: true });
// ponytail: every issues event reloads the inbox, debounced; the server
// recomputes it each time (inbox.go says when to keep it instead)
const reloadInbox = debounced(() => void loadInbox());
let stopLive = () => {};
```

In its `onMounted`, add:

```ts
  if (!inbox.value) void loadInbox();
  stopLive = onLive((e) => {
    if (e.kind === 'issues' || e.kind === 'inbox') reloadInbox();
  });
```

and in `onBeforeUnmount`'s callback, call `stopLive()` next to the `removeEventListener`:

```ts
onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKey);
  stopLive();
});
```

- [ ] **Step 4: Type-check, build, try it**

Run: `cd web && npm run typecheck && npm test`
Expected: no errors; PASS. In the running app: the header's **Inbox** shows the count and the tab reads "(n) git-board"; opening a row in the inbox lowers both; a comment by someone else (`git bug comment new` as another identity, from a second clone) raises them within a second; ⌘K → "Inbox" goes there.

- [ ] **Step 5: Commit**

```bash
git add web/src/components/AppHeader.vue web/src/components/GbIcon.vue web/src/components/CommandPalette.vue
git commit -m "feat(inbox): the header's Inbox count, the tab title, the palette

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 10: Follow, in the issue's side column

**Files:**
- Modify: `web/src/components/IssueSide.vue`
- Modify: `docs/implementation-plan.md` (a status update)

**Interfaces:**
- Consumes: `getFollow`, `setFollow`, `FollowState`, `ApiError` (Task 7).

- [ ] **Step 1: Merge main first.** `IssueSide.vue` has work in flight there (branch `issuelink`: Link / Unlink / Take the forge's version).

```bash
git log --oneline main | grep -i -m3 "link"
git merge main
```

Expected: the `issuelink` work is in `main` (a commit about Link/Unlink). If it isn't yet, stop and wait: don't edit `IssueSide.vue` until it is.

- [ ] **Step 2: Add the block.** In `IssueSide.vue`'s template, as the last section of `.gb-side`:

```vue
    <section v-if="follow" class="gb-side-sec" aria-labelledby="side-inbox">
      <h3 id="side-inbox" class="gb-section-title">Inbox</h3>
      <p class="gb-follow-state"><strong>{{ follow.following ? 'Following' : 'Not following' }}</strong> <span class="gb-hint">{{ followBecause }}</span></p>
      <div class="gb-follow-actions">
        <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="followBusy" @click="setFollowTo(!follow.following)">{{ follow.following ? 'Unfollow' : 'Follow' }}</button>
        <button v-if="follow.set !== null" type="button" class="gb-linkbtn" :disabled="followBusy" @click="setFollowTo(null)">Back to automatic</button>
      </div>
    </section>
    <p v-else-if="followError" class="gb-hint">{{ followError }}</p>
```

In its script (add `ref`, `computed`, `watch` to the `vue` import if missing; fold the names below into its existing `@/api/board` import rather than adding a second one):

```ts
import { ApiError, getFollow, setFollow, type FollowState } from '@/api/board';

// Inbox: whether you follow this issue, and why (cmd/git-board-server/inbox.go)
const follow = ref<FollowState | null>(null);
const followError = ref('');
const followBusy = ref(false);
async function loadFollow() {
  try {
    follow.value = await getFollow(props.id);
    followError.value = '';
  } catch (e) {
    follow.value = null;
    // no identity, no login: the inbox page says why; anything else, here
    const quiet = e instanceof ApiError && (e.code === 'no-identity' || e.code === 'no-login');
    followError.value = quiet ? '' : e instanceof Error ? e.message : String(e);
  }
}
watch(() => [props.id, props.labels.join(','), props.events.length], loadFollow, { immediate: true });
async function setFollowTo(v: boolean | null) {
  followBusy.value = true;
  try {
    follow.value = await setFollow(props.id, v);
  } catch (e) {
    followError.value = e instanceof Error ? e.message : String(e);
  } finally {
    followBusy.value = false;
  }
}
const followBecause = computed(() => {
  const f = follow.value;
  if (!f) return '';
  if (f.set === true) return 'You follow it.';
  if (f.set === false) return 'You unfollowed it.';
  return f.reasons.length ? `Because: ${f.reasons.join(' · ')}` : "You're not in it.";
});
```

In its `<style scoped>`:

```css
.gb-follow-state { margin: 0 0 var(--space-2); }
.gb-follow-actions { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-2); }
.gb-follow-auto { min-height: 2.75rem; padding: 0 var(--space-1); border: 0; background: none; color: var(--fg); font: inherit; text-decoration: underline; cursor: pointer; }
```

- [ ] **Step 3: Type-check and try it**

Run: `cd web && npm run typecheck && npm test`
Expected: no errors; PASS. In the app: an issue you're assigned to says "Following · Because: assignee"; Unfollow → "Not following · You unfollowed it." with Back to automatic; the inbox stops showing its comments; Back to automatic restores both.

- [ ] **Step 4: Status update.** Append to `docs/implementation-plan.md`:

```markdown
## Status update (YYYY-MM-DD): the inbox, on `prd/notify`

- The email-notifications PRD was grilled and replaced by an inbox you
  check (`docs/prd/inbox.md`): the forge mirror already emails followers,
  sending needs a server that doesn't exist, Outlook mangles replies.
- `involves:` and `follows:@me` are filter terms, shared by the inbox, the
  board and ⌘K (`internal/filter`'s `Involvement`).
- `internal/inbox`: one file per identity in git-bug's local storage,
  seen as event ids (a comment pushed a day late still shows), a 14-day
  window; `Rows` holds every rule.
- The page checks the git remotes on opening and offers Pull; it never
  pulls by itself.
```

(with today's date).

- [ ] **Step 5: Run everything and commit**

Run: `go test ./... -race && cd web && npm run typecheck && npm test`
Expected: PASS.

```bash
git add web/src/components/IssueSide.vue docs/implementation-plan.md
git commit -m "feat(inbox): follow or unfollow an issue from its side column

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```
