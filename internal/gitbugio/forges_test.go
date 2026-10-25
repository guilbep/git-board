package gitbugio_test

import (
	"maps"
	"strings"
	"testing"

	"github.com/guilbep/git-board/core/forge"
	"github.com/guilbep/git-board/core/forge/fake"
	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio"
	"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"
)

// run is a run with these forges, each named by its host's first word.
func run(fs ...*fake.Forge) gitbugio.ForgeRun {
	var r gitbugio.ForgeRun
	for _, f := range fs {
		name, _, _ := strings.Cut(f.Key(), ".")
		r.Forges = append(r.Forges, gitbugio.NamedForge{Name: name, Forge: f})
	}
	return r
}

// Get brings the forge's issues in and sends nothing; Send sends and gets
// nothing.
func TestGetAndSendAreOneWay(t *testing.T) {
	t.Parallel()
	b := gitbugiotest.New(t)
	f := fake.New("github.com/o/r")
	f.Add(forge.Issue{Title: "from the forge", Body: "body", Author: forge.Account{ID: "7", Login: "alice"}})
	gitbugiotest.NewIssue(t, b, "from git-board")

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

func counts(events []gitbugio.SyncEvent) map[string]int {
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
	b := gitbugiotest.New(t)
	gh, gl := fake.New("github.com/o/r"), fake.New("gitlab.example.com/7")
	bob := forge.Account{ID: "8", Login: "bob"}
	gh.Add(forge.Issue{Title: "from GitHub", Author: forge.Account{ID: "7", Login: "alice"},
		Comments: []forge.Comment{{Body: "bob's", Author: bob}}})
	gitbugiotest.NewIssue(t, b, "from git-board")
	before, err := b.Store().Refs("refs/")
	if err != nil {
		t.Fatal(err)
	}

	var dry []gitbugio.SyncEvent
	r := run(gh, gl)
	r.DryRun, r.See = true, func(e gitbugio.SyncEvent) { dry = append(dry, e) }
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

	var real []gitbugio.SyncEvent
	r.DryRun, r.See = false, func(e gitbugio.SyncEvent) { real = append(real, e) }
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
	b := gitbugiotest.New(t)
	gh := fake.New("github.com/o/r")
	alice := forge.Account{ID: "7", Login: "alice"}
	gh.Add(forge.Issue{Title: "one", Author: alice})
	if _, err := b.GetForges(t.Context(), run(gh)); err != nil {
		t.Fatal(err)
	}
	gh.Grant(alice)
	issues, _ := b.ListIssues()
	p := issues[0].AuthorID
	for _, is := range []string{gitbugiotest.NewIssue(t, b, "two"), gitbugiotest.NewIssue(t, b, "three")} {
		gitbugiotest.CommentAs(t, b, is, gitbugio.Person{ID: p}, 1, "by alice")
	}
	var dry []gitbugio.SyncEvent
	r := run(gh)
	r.DryRun, r.See = true, func(e gitbugio.SyncEvent) { dry = append(dry, e) }
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
	b := gitbugiotest.New(t)
	for _, login := range []string{"alice", "bob"} {
		gitbugiotest.Token(t, b, "https://github.com", login, "tok-"+login)
	}
	if _, _, err := b.GitHubToken(); err == nil || !strings.Contains(err.Error(), "no GitHub login") {
		t.Errorf("no login: err %v, want no GitHub login", err)
	}
	gitbugiotest.SetMyLogin(t, b, "carol")
	if _, tok, err := b.GitHubToken(); err == nil || err.Error() != "no GitHub token for carol" || tok != "" {
		t.Errorf("carol, no token: %q %v, want no GitHub token for carol", tok, err)
	}
	bob := gitbugiotest.NewPerson(t, b, "Bob", "bob")
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
	b := gitbugiotest.New(t)
	forges := []config.Forge{
		{Name: "gh", Kind: "github", Host: "github.com", Project: "o/r", Login: "alice"},
		{Name: "gl", Kind: "gitlab", Host: "gitlab.example.com", Project: "7", Login: "bob"},
	}
	gitbugiotest.Token(t, b, "https://gitlab.example.com", "bob", "gl-secret")
	gitbugiotest.Token(t, b, "https://github.com", "mallory", "not-alice's")
	for host, want := range map[string]string{"github.com": "", "gitlab.example.com": "gl-secret", "elsewhere.example": ""} {
		if got := b.ForgeToken(forges, host); got != want {
			t.Errorf("ForgeToken(%s) = %q, want %q", host, got, want)
		}
	}
	if got := b.Credentials(forges); len(got) != 1 || got[0] != (gitbugio.Credential{ID: "gl", Target: "gitlab", Host: "gitlab.example.com", Login: "bob"}) {
		t.Errorf("Credentials = %+v", got)
	}
}
