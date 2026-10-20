package gitbugio_test

import (
	"slices"
	"testing"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"
)

// Flat kinds are stored as git-bug and the forges have them, bug, and read
// as the kind field, kind:bug, everywhere a label comes out.
func TestFlatKindsAreTheKindField(t *testing.T) {
	t.Parallel()
	b := gitbugiotest.New(t)
	b.SetFlatKinds(&config.Config{Branches: config.Branches{Kinds: map[string]string{"bug": "fix", "feat": "feat"}}})
	raw := func(id string) []string {
		t.Helper()
		bug, err := b.TestCache().Bugs().ResolvePrefix(id)
		if err != nil {
			t.Fatal(err)
		}
		var res []string
		for _, l := range bug.Snapshot().Labels {
			res = append(res, l.String())
		}
		slices.Sort(res)
		return res
	}
	sorted := func(l []string, err error) []string {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return slices.Sorted(slices.Values(l))
	}

	id, err := b.NewIssue("x", "", []string{"kind:feat", "status:ready"})
	if err != nil {
		t.Fatal(err)
	}
	if got := raw(id); !slices.Equal(got, []string{"feat", "status:ready"}) {
		t.Errorf("stored %q, want feat", got)
	}
	if got := sorted(b.GetLabels(id)); !slices.Equal(got, []string{"kind:feat", "status:ready"}) {
		t.Errorf("read %q, want kind:feat", got)
	}
	if got := sorted(b.ApplyLabelChange(id, []string{"kind:bug"}, []string{"kind:feat"})); !slices.Equal(got, []string{"kind:bug", "status:ready"}) {
		t.Errorf("changed to %q", got)
	}
	if got := raw(id); !slices.Equal(got, []string{"bug", "status:ready"}) {
		t.Errorf("stored %q, want bug", got)
	}

	// a kind:bug written before, beside bug: read once, removed with it
	bug, _ := b.TestCache().Bugs().ResolvePrefix(id)
	if _, _, err := bug.ChangeLabels([]string{"kind:bug"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := bug.Commit(); err != nil {
		t.Fatal(err)
	}
	if got := sorted(b.GetLabels(id)); !slices.Equal(got, []string{"kind:bug", "status:ready"}) {
		t.Errorf("both spellings read %q", got)
	}
	if _, err := b.ApplyLabelChange(id, []string{"kind:feat"}, []string{"kind:bug"}); err != nil {
		t.Fatal(err)
	}
	if got := raw(id); !slices.Equal(got, []string{"feat", "status:ready"}) {
		t.Errorf("stored %q, want feat alone", got)
	}

	if l := b.Labels(); slices.Contains(l, "feat") || !slices.Contains(l, "kind:feat") {
		t.Errorf("Labels = %q", l)
	}
	th, err := b.Thread(id)
	if err != nil {
		t.Fatal(err)
	}
	var added []string
	for _, e := range th.Events {
		added = append(added, e.Added...)
	}
	if slices.Contains(added, "feat") || !slices.Contains(added, "kind:feat") {
		t.Errorf("thread's labels added %q", added)
	}
	hist, err := b.History(nil)
	if err != nil {
		t.Fatal(err)
	}
	added = nil
	for _, c := range hist[0].Changes {
		added = append(added, c.Added...)
	}
	if slices.Contains(added, "feat") || !slices.Contains(added, "kind:feat") {
		t.Errorf("history's labels added %q", added)
	}

	// kind:* labels: nothing translated
	b.SetFlatKinds(&config.Config{Fields: map[string]config.Field{"kind": {Options: []string{"bug", "feat"}}},
		Branches: config.Branches{Kinds: map[string]string{"bug": "fix", "feat": "feat"}}})
	if got := sorted(b.GetLabels(id)); !slices.Equal(got, []string{"feat", "status:ready"}) {
		t.Errorf("kind:* form read %q, want feat as is", got)
	}
}
