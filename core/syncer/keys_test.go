package syncer

import (
	"maps"
	"reflect"
	"strings"
	"testing"

	"github.com/guilbep/git-board/core/forge"
	"github.com/guilbep/git-board/core/forge/fake"
	"github.com/guilbep/git-board/core/gitstore"
	"github.com/guilbep/git-board/core/issue"
)

// keyed carries the keys step, anchor and resolved by their own names, and
// restores them; a marker deleted clears the anchor.
func keyed() Options {
	names := []string{"step", "anchor", "resolved"}
	return Options{
		Carry: func(m map[string]string) string {
			var kv []string
			for _, k := range names {
				if m[k] != "" {
					kv = append(kv, k+"="+m[k])
				}
			}
			return strings.Join(kv, " ")
		},
		Restore: func(carried string, marked bool, m map[string]string) map[string]string {
			out := map[string]string{}
			if !marked {
				if m["anchor"] != "" {
					out["anchor"] = ""
				}
				return out
			}
			got := map[string]string{}
			for _, kv := range strings.Fields(carried) {
				k, v, _ := strings.Cut(kv, "=")
				got[k] = v
			}
			for _, k := range names {
				if got[k] != m[k] {
					out[k] = got[k]
				}
			}
			return out
		},
	}
}

// commented makes an issue with one comment carrying meta, synced once.
func commented(t *testing.T, opt Options, meta map[string]string) (*fakeRun, *issue.Issue) {
	t.Helper()
	s, f := newStore(t), fake.New(key)
	is, err := issue.Create(t.Context(), s, issue.NewCreate("a", "T", "B"))
	if err != nil {
		t.Fatal(err)
	}
	if is, err = issue.Do(t.Context(), s, is.ID, issue.NewCommentMeta("a", "the question", meta)); err != nil {
		t.Fatal(err)
	}
	fr := &fakeRun{t: t, opt: opt, f: f, s: s}
	fr.sync()
	return fr, is
}

type fakeRun struct {
	t   *testing.T
	opt Options
	f   *fake.Forge
	s   *gitstore.Store
}

func (fr *fakeRun) sync() Report {
	fr.t.Helper()
	r, err := Sync(fr.t.Context(), fr.s, fr.opt, fr.f)
	if err != nil {
		fr.t.Fatal(err)
	}
	return r
}

func (fr *fakeRun) forgeComment() forge.Comment {
	fr.t.Helper()
	all := fr.f.All()
	if len(all) != 1 || len(all[0].Comments) != 1 {
		fr.t.Fatalf("forge = %+v", all)
	}
	return all[0].Comments[0]
}

func (fr *fakeRun) editComment(change func(string) string) {
	fr.f.Edit(fr.f.All()[0].Number, func(i *forge.Issue) { i.Comments[0].Body = change(i.Comments[0].Body) })
}

// A comment's marker carries its keys; a second run changes nothing.
func TestMarkerKeys(t *testing.T) {
	t.Parallel()
	fr, is := commented(t, keyed(), map[string]string{"step": "7", "anchor": "body@v1/expected#3"})
	want := "<!-- git-board comment " + is.Comments[0].ID + " step=7 anchor=body@v1/expected#3 -->"
	if got := fr.forgeComment().Body; !strings.HasSuffix(got, "\n\n"+want) {
		t.Fatalf("posted %q, want it ending %q", got, want)
	}
	if r := fr.sync(); r.Pushed != 0 || r.Posted != 0 {
		t.Errorf("second run = %+v", r)
	}
	if got := only(t, fr.s); len(got.Comments) != 1 || !maps.Equal(got.Comments[0].Meta, is.Comments[0].Meta) {
		t.Errorf("comments = %+v", got.Comments)
	}
}

// A crash between posting a keyed comment and recording it: the next run
// finds it by its marker and posts it once.
func TestMarkerKeysAfterACrash(t *testing.T) {
	t.Parallel()
	s, f := newStore(t), fake.New(key)
	is, err := issue.Create(t.Context(), s, issue.NewCreate("a", "T", "B"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = issue.Do(t.Context(), s, is.ID, issue.NewCommentMeta("a", "q", map[string]string{"step": "1"})); err != nil {
		t.Fatal(err)
	}
	f.After(func(w fake.Write) error {
		if w.Op == "create-comment" {
			f.After(nil)
			return fake.ErrCrash
		}
		return nil
	})
	if _, err := Sync(t.Context(), s, keyed(), f); err == nil {
		t.Fatal("no crash")
	}
	if _, err := Sync(t.Context(), s, keyed(), f); err != nil {
		t.Fatal(err)
	}
	if n := len(f.All()[0].Comments); n != 1 {
		t.Errorf("posted %d times", n)
	}
	if got := only(t, s); len(got.Comments) != 1 {
		t.Errorf("comments = %+v", got.Comments)
	}
}

// A metadata change alone edits the forge comment's marker, once.
func TestKeysFollowMeta(t *testing.T) {
	t.Parallel()
	fr, is := commented(t, keyed(), map[string]string{"step": "1", "anchor": "body@v1/expected"})
	c := is.Comments[0]
	if _, err := issue.Do(t.Context(), fr.s, is.ID, issue.SetMeta("a", c.ID, map[string]string{"resolved": "v2"})); err != nil {
		t.Fatal(err)
	}
	if r := fr.sync(); r.Pushed != 1 {
		t.Errorf("run = %+v, want one edit", r)
	}
	if got := fr.forgeComment().Body; !strings.HasSuffix(got, " step=1 anchor=body@v1/expected resolved=v2 -->") {
		t.Errorf("forge comment %q", got)
	}
	if r := fr.sync(); r.Pushed != 0 {
		t.Errorf("third run = %+v", r)
	}
}

// The forge edits the keys in a comment sync posted: the pull sets them.
func TestForgeEditRestores(t *testing.T) {
	t.Parallel()
	fr, _ := commented(t, keyed(), map[string]string{"step": "1", "anchor": "body@v1/expected"})
	fr.editComment(func(b string) string { return strings.Replace(b, " -->", " resolved=v2 -->", 1) })
	fr.sync()
	got := only(t, fr.s).Comments[0]
	if want := map[string]string{"step": "1", "anchor": "body@v1/expected", "resolved": "v2"}; !reflect.DeepEqual(got.Meta, want) {
		t.Errorf("meta = %v, want %v", got.Meta, want)
	}
	if got.Body != "the question" {
		t.Errorf("body %q", got.Body)
	}
	if r := fr.sync(); r.Pushed != 0 {
		t.Errorf("next run = %+v", r)
	}
}

// The forge deletes the marker: the comment is the forge's text, its anchor
// cleared as Restore says.
func TestMarkerDeletedOnTheForge(t *testing.T) {
	t.Parallel()
	fr, _ := commented(t, keyed(), map[string]string{"step": "1", "anchor": "body@v1/expected"})
	fr.editComment(func(b string) string { return "the question, edited there" })
	fr.sync()
	got := only(t, fr.s).Comments[0]
	if got.Body != "the question, edited there" || got.Meta["anchor"] != "" || got.Meta["step"] != "1" {
		t.Errorf("comment = %q %v", got.Body, got.Meta)
	}
	if r := fr.sync(); r.Pushed != 0 {
		t.Errorf("next run = %+v", r)
	}
}

// Another account's text ending with a git-board marker and keys is text.
func TestStrangerKeysAreText(t *testing.T) {
	t.Parallel()
	s, f := newStore(t), fake.New(key)
	one := sample()
	body := "copied\n\n<!-- git-board comment 3f7a9c2e41b6d08a5e1f93c7b2d4a6e8f0c1b3d5:1 step=1 anchor=body@v1/x -->"
	one.Comments[0].Body = body
	f.Add(one)
	if _, err := Sync(t.Context(), s, keyed(), f); err != nil {
		t.Fatal(err)
	}
	got := only(t, s).Comments
	if len(got) != 1 || got[0].Body != body || len(got[0].Meta) != 0 {
		t.Errorf("comments = %+v", got)
	}
}

func newFake() *fake.Forge { return fake.New(key) }
