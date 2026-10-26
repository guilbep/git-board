package gitbugio_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/guilbep/git-board/core/issue"
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
		is, err := issue.Load(b.Store(), id)
		if err != nil {
			t.Fatal(err)
		}
		return is.Labels
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
	me, _, _ := b.Me()
	if _, err := issue.Do(t.Context(), b.Store(), id, issue.ChangeLabels(me.ID, []string{"kind:bug"}, nil)); err != nil {
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

// An assignee label in any case, as a forge may send it, reads as
// assignee:, once, and goes with any of its spellings.
func TestAssigneeInAnyCase(t *testing.T) {
	t.Parallel()
	b := gitbugiotest.New(t)
	id := gitbugiotest.NewIssue(t, b, "x")
	gitbugiotest.RawLabels(t, b, id, "Assignee:bob", "ASSIGNEE:bob", "assignee:ann")
	if got := sorted(b.GetLabels(id)); !slices.Equal(got, []string{"assignee:ann", "assignee:bob"}) {
		t.Fatalf("labels = %q", got)
	}
	if got, err := b.ApplyLabelChange(id, []string{"assignee:bob"}, []string{"assignee:ann"}); err != nil || !slices.Equal(got, []string{"assignee:bob"}) {
		t.Fatalf("after adding bob again, removing ann: %q, %v", got, err)
	}
	if got, err := b.ApplyLabelChange(id, nil, []string{"assignee:bob"}); err != nil || len(got) != 0 {
		t.Fatalf("after removing bob: %q, %v", got, err)
	}
	if l := b.Labels(); slices.ContainsFunc(l, func(x string) bool { return strings.HasPrefix(x, "Assignee") || strings.HasPrefix(x, "ASSIGNEE") }) {
		t.Errorf("Labels = %q", l)
	}
}

func sorted(l []string, err error) []string {
	if err != nil {
		panic(err)
	}
	return slices.Sorted(slices.Values(l))
}
