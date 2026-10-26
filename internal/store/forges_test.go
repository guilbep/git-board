package store_test

import (
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/guilbep/git-board/core/forge"
	"github.com/guilbep/git-board/core/forge/fake"
	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/store"
	"github.com/guilbep/git-board/internal/store/storetest"
)

// run is a run with these forges, each named by its host's first word,
// with consent to create (TestCreateNeedsConsent has the run without).
func run(fs ...*fake.Forge) store.ForgeRun {
	r := store.ForgeRun{AllowCreate: true}
	for _, f := range fs {
		name, _, _ := strings.Cut(f.Key(), ".")
		r.Forges = append(r.Forges, store.NamedForge{Name: name, Forge: f})
	}
	return r
}

// Get brings the forge's issues in and sends nothing; Send sends and gets
// nothing.
func TestGetAndSendAreOneWay(t *testing.T) {
	t.Parallel()
	b := storetest.New(t)
	f := fake.New("github.com/o/r")
	f.Add(forge.Issue{Title: "from the forge", Body: "body", Author: forge.Account{ID: "7", Login: "alice"}})
	storetest.NewIssue(t, b, "from git-board")

	if _, err := b.GetForges(t.Context(), run(f)); err != nil {
		t.Fatal(err)
	}
	if issues, _ := b.ListIssues(); len(issues) != 2 || len(f.Writes()) != 0 {
		t.Fatalf("after Get: %d issues here, %d writes to the forge; want 2 and none", len(issues), len(f.Writes()))
	}
	f.Add(forge.Issue{Title: "later on the forge", Author: forge.Account{ID: "7", Login: "alice"}})
	if _, err := b.SendForges(t.Context(), run(f)); err != nil {
		t.Fatal(err)
	}
	if got, ok := f.Get("3"); !ok || got.Title != "from git-board" {
		t.Errorf("Send didn't create the issue on the forge: %+v", f.All())
	}
	if issues, _ := b.ListIssues(); len(issues) != 2 {
		t.Errorf("Send got %d issues, want the 2 it had", len(issues))
	}
}

func counts(events []store.SyncEvent) map[string]int {
	out := map[string]int{}
	for _, e := range events {
		out[e.Forge+" "+e.What]++
	}
	return out
}

// A dry run writes nothing, here or on a forge, and counts what the run
// would do: what one forge has goes to the other, a comment by someone
// with their own token too. The run then does what it said.
func TestDryRunWritesNothing(t *testing.T) {
	t.Parallel()
	b := storetest.New(t)
	gh, gl := fake.New("github.com/o/r"), fake.New("gitlab.example.com/7")
	bob := forge.Account{ID: "8", Login: "bob"}
	gh.Add(forge.Issue{Title: "from GitHub", Author: forge.Account{ID: "7", Login: "alice"},
		Comments: []forge.Comment{{Body: "bob's", Author: bob}}})
	storetest.NewIssue(t, b, "from git-board")
	before, err := b.Store().Refs("refs/")
	if err != nil {
		t.Fatal(err)
	}

	var dry []store.SyncEvent
	r := run(gh, gl)
	r.DryRun, r.See = true, func(e store.SyncEvent) { dry = append(dry, e) }
	report, err := b.SyncForges(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := b.Store().Refs("refs/")
	if !maps.Equal(before, after) || len(gh.Writes()) != 0 || len(gl.Writes()) != 0 {
		t.Fatalf("a dry run wrote: refs %v -> %v, writes %v %v", before, after, gh.Writes(), gl.Writes())
	}
	want := map[string]int{"github created": 1, "github posted": 1, "gitlab posted": 2, "gitlab comment": 1, "gitlab pushed": 1}
	if got := counts(dry); !maps.Equal(got, want) {
		t.Errorf("the check counted %v, want %v", got, want)
	}

	var real []store.SyncEvent
	r.DryRun, r.See = false, func(e store.SyncEvent) { real = append(real, e) }
	got, err := b.SyncForges(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}
	if got != report || !maps.Equal(counts(real), counts(dry)) {
		t.Errorf("the run did %+v %v, the check said %+v %v", got, counts(real), report, counts(dry))
	}
	if len(gl.All()) != 2 {
		t.Errorf("GitLab has %d issues, want 2", len(gl.All()))
	}
}

// A person with their own token writes as themselves, in a dry run too:
// the views As gives are counted, and their made-up ids never collide.
func TestDryRunThroughAPersonsToken(t *testing.T) {
	t.Parallel()
	b := storetest.New(t)
	gh := fake.New("github.com/o/r")
	alice := forge.Account{ID: "7", Login: "alice"}
	gh.Add(forge.Issue{Title: "one", Author: alice})
	if _, err := b.GetForges(t.Context(), run(gh)); err != nil {
		t.Fatal(err)
	}
	gh.Grant(alice)
	issues, _ := b.ListIssues()
	p := issues[0].AuthorID
	for _, is := range []string{storetest.NewIssue(t, b, "two"), storetest.NewIssue(t, b, "three")} {
		storetest.CommentAs(t, b, is, store.Person{ID: p}, 1, "by alice")
	}
	var dry []store.SyncEvent
	r := run(gh)
	r.DryRun, r.See = true, func(e store.SyncEvent) { dry = append(dry, e) }
	if _, err := b.SendForges(t.Context(), r); err != nil {
		t.Fatal(err)
	}
	if got := counts(dry); got["github posted"] != 2 || got["github comment"] != 2 || len(gh.Writes()) != 0 {
		t.Errorf("counted %v, writes %v", got, gh.Writes())
	}
}

// The token git-board acts on GitHub with is the one of whoever writes
// from it: their github.com account's login, never another person's
// token.
func TestGitHubToken(t *testing.T) {
	t.Parallel()
	b := storetest.New(t)
	for _, login := range []string{"alice", "bob"} {
		storetest.Token(t, b, "https://github.com", login, "tok-"+login)
	}
	if _, _, err := b.GitHubToken(); err == nil || !strings.Contains(err.Error(), "no GitHub login") {
		t.Errorf("no login: err %v, want no GitHub login", err)
	}
	storetest.SetMyLogin(t, b, "carol")
	if _, tok, err := b.GitHubToken(); err == nil || err.Error() != "no GitHub token for carol" || tok != "" {
		t.Errorf("carol, no token: %q %v, want no GitHub token for carol", tok, err)
	}
	bob := storetest.NewPerson(t, b, "Bob", "bob")
	if err := b.Adopt(bob.ID); err != nil {
		t.Fatal(err)
	}
	if login, tok, err := b.GitHubToken(); err != nil || login != "bob" || tok != "tok-bob" {
		t.Errorf("bob: %q %q %v, want bob's token", login, tok, err)
	}
}

// A forge's token is its posting account's, on its host; Credentials say
// which forges have one, never the token.
func TestForgeToken(t *testing.T) {
	t.Parallel()
	b := storetest.New(t)
	forges := []config.Forge{
		{Name: "gh", Kind: "github", Host: "github.com", Project: "o/r", Login: "alice"},
		{Name: "gl", Kind: "gitlab", Host: "gitlab.example.com", Project: "7", Login: "bob"},
	}
	storetest.Token(t, b, "https://gitlab.example.com", "bob", "gl-secret")
	storetest.Token(t, b, "https://github.com", "mallory", "not-alice's")
	for host, want := range map[string]string{"github.com": "", "gitlab.example.com": "gl-secret", "elsewhere.example": ""} {
		if got := b.ForgeToken(forges, host); got != want {
			t.Errorf("ForgeToken(%s) = %q, want %q", host, got, want)
		}
	}
	if got := b.Credentials(forges); len(got) != 1 || got[0] != (store.Credential{ID: "gl", Target: "gitlab", Host: "gitlab.example.com", Login: "bob"}) {
		t.Errorf("Credentials = %+v", got)
	}
}

// A check's copy left a day ago, by a run killed mid-way, goes with the
// next check; a newer one, maybe another run's, stays.
func TestScratchRemovesOldCopies(t *testing.T) {
	t.Parallel()
	b := storetest.New(t)
	board := filepath.Join(b.Store().CommonDir(), "board")
	old, recent := filepath.Join(board, "check-old"), filepath.Join(board, "check-recent")
	for _, d := range []string{old, recent} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	day := time.Now().Add(-25 * time.Hour)
	if err := os.Chtimes(old, day, day); err != nil {
		t.Fatal(err)
	}
	_, done, err := store.Scratch(b, t.Context())
	if err != nil {
		t.Fatal(err)
	}
	done()
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("the old copy stays: %v", err)
	}
	if _, err := os.Stat(recent); err != nil {
		t.Errorf("the recent copy went: %v", err)
	}
}

// Creations on a forge the repository never synced with, or MaxCreate or
// more on one, wait for consent: the run tells them (held) and creates
// none there, its other writes made. A check counts them as made, held.
func TestCreateNeedsConsent(t *testing.T) {
	t.Parallel()
	b := storetest.New(t)
	storetest.NewIssue(t, b, "Export CSV")
	storetest.NewIssue(t, b, "Import CSV")
	f := fake.New("github.com/o/r")
	r := run(f)
	r.AllowCreate = false
	var held []store.SyncEvent
	r.See = func(e store.SyncEvent) {
		if e.What == "held" {
			held = append(held, e)
		}
	}
	r.DryRun = true
	if _, err := b.SyncForges(t.Context(), r); err != nil {
		t.Fatal(err)
	}
	if len(held) != 1 || held[0].Ops != 2 || held[0].Forge != "github" || len(f.All()) != 0 {
		t.Fatalf("check: held %+v, forge %d issues", held, len(f.All()))
	}
	held, r.DryRun = nil, false
	if _, err := b.SyncForges(t.Context(), r); err != nil {
		t.Fatal(err)
	}
	if len(held) != 1 || len(f.All()) != 0 {
		t.Fatalf("run without consent: held %+v, forge %d issues", held, len(f.All()))
	}
	held, r.AllowCreate = nil, true
	if _, err := b.SyncForges(t.Context(), r); err != nil {
		t.Fatal(err)
	}
	if len(held) != 0 || len(f.All()) != 2 {
		t.Fatalf("run with consent: held %+v, forge %d issues", held, len(f.All()))
	}
}
