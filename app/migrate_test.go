package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/guilbep/git-board/domain/issues/issuesync"
	"github.com/guilbep/git-board/forge"
	"github.com/guilbep/git-board/forge/fake"
	"github.com/guilbep/git-board/kernel"
)

// The forge-mapping migration: the preview counts the issues linked on
// each declared forge; the run sets the assignees, the field's home
// there, and takes the old labels off the forge's issue.
func TestMigrateField(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	toml := "[fields.assignee]\n\n[forges.gh]\nkind = \"github\"\nproject = \"o/r\"\nhost = \"example.com\"\nsync = true\n\n[forges.lab]\nkind = \"gitlab\"\nproject = \"9\"\nhost = \"lab.example.com\"\n\n[forges.lab.fields.assignee]\nto = \"label\"\n"
	if err := os.WriteFile(filepath.Join(dir, ".git-board.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := Open(t.Context(), dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	me, err := issuesync.CreatePerson(t.Context(), b.Kernel, issuesync.NewPerson("", "me", ""))
	if err != nil {
		t.Fatal(err)
	}
	b.Issues.Me = kernel.ID(me.ID)
	is, err := issuesync.CreateIssue(t.Context(), b.Kernel, issuesync.NewCreate(me.ID, "Here", "made here"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Issues.ChangeLabels(t.Context(), is.ID, []string{"assignee:ann"}, nil); err != nil {
		t.Fatal(err)
	}
	f := fake.New("example.com/o/r")
	if _, err := b.SyncForges(t.Context(), WaySync, ForgeRun{Forges: []NamedForge{{Name: "gh", Forge: f}}, AllowCreate: true}); err != nil {
		t.Fatal(err)
	}
	if len(f.All()) != 1 {
		t.Fatalf("%d forge issues, want the one posted", len(f.All()))
	}
	ref := forge.Ref{Number: f.All()[0].Number}
	// the label as the old home had it, on the forge
	if err := f.ChangeLabels(t.Context(), ref, []string{"assignee:ann"}, nil); err != nil {
		t.Fatal(err)
	}

	plans, err := b.MigrateField(t.Context(), MigrateReq{Field: "assignee", DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 2 || plans[0].Forge != "gh" || plans[0].Target != forge.TargetAssignees || plans[0].Issues != 1 ||
		plans[1].Forge != "lab" || plans[1].Target != forge.TargetLabel || plans[1].Issues != 0 {
		t.Fatalf("preview %+v", plans)
	}
	if got, _ := f.Get(ref.Number); !slices.Contains(got.Labels, "assignee:ann") {
		t.Fatalf("the preview wrote: %+v", got)
	}

	var said []string
	open := func(_ context.Context, tb forge.Table) (forge.Forge, error) {
		if tb.Name != "gh" {
			t.Errorf("opened %s, a forge where the field is still a label", tb.Name)
		}
		return f, nil
	}
	plans, err = b.MigrateField(t.Context(), MigrateReq{Field: "assignee", Open: open, Say: func(l string) { said = append(said, l) }})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := f.Get(ref.Number)
	if slices.Contains(got.Labels, "assignee:ann") || len(got.Assignees) != 1 || got.Assignees[0].Login != "ann" {
		t.Errorf("forge issue after the run: labels %q, assignees %+v", got.Labels, got.Assignees)
	}
	if plans[0].Labels != 1 || len(said) != 1 || said[0] != "gh (o/r): 1 issues moved to assignees, 1 labels off the issues" {
		t.Errorf("plans %+v, said %q", plans, said)
	}
	if _, err := b.MigrateField(t.Context(), MigrateReq{Field: "nosuch", DryRun: true}); err == nil || Classify(err) != Usage {
		t.Errorf("a field nothing configures: %v", err)
	}
}

// Two GitLab tables on one host: an issue linked on one is migrated
// there alone, never on the other project's issue of the same number.
func TestMigrateTwoGitLabProjectsOneHost(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	toml := "[fields.assignee]\n\n[forges.a]\nkind = \"gitlab\"\nproject = \"11\"\nhost = \"lab.example.com\"\nsync = true\n\n[forges.b]\nkind = \"gitlab\"\nproject = \"22\"\nhost = \"lab.example.com\"\nsync = true\n"
	if err := os.WriteFile(filepath.Join(dir, ".git-board.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := Open(t.Context(), dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	me, err := issuesync.CreatePerson(t.Context(), b.Kernel, issuesync.NewPerson("", "me", ""))
	if err != nil {
		t.Fatal(err)
	}
	b.Issues.Me = kernel.ID(me.ID)
	is, err := issuesync.CreateIssue(t.Context(), b.Kernel, issuesync.NewCreate(me.ID, "Here", "made here"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Issues.ChangeLabels(t.Context(), is.ID, []string{"assignee:ann"}, nil); err != nil {
		t.Fatal(err)
	}
	fa, fb := fake.New("lab.example.com/11"), fake.New("lab.example.com/22")
	if _, err := b.SyncForges(t.Context(), WaySync, ForgeRun{Forges: []NamedForge{{Name: "a", Forge: fa}}, AllowCreate: true}); err != nil {
		t.Fatal(err)
	}
	if len(fa.All()) != 1 {
		t.Fatalf("%d issues on a, want the one posted", len(fa.All()))
	}
	n := fa.All()[0].Number
	// project 22's own issue of the same number, not linked here
	other := fb.Add(forge.Issue{Number: n, Title: "theirs", Labels: []string{"assignee:ann"}, Assignees: []forge.Account{{Login: "zoe"}}})

	plans, err := b.MigrateField(t.Context(), MigrateReq{Field: "assignee", DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 2 || plans[0].Forge != "a" || plans[0].Issues != 1 || plans[1].Forge != "b" || plans[1].Issues != 0 {
		t.Fatalf("preview %+v", plans)
	}
	open := func(_ context.Context, tb forge.Table) (forge.Forge, error) {
		if tb.Name == "a" {
			return fa, nil
		}
		return fb, nil
	}
	if _, err := b.MigrateField(t.Context(), MigrateReq{Field: "assignee", Open: open}); err != nil {
		t.Fatal(err)
	}
	if got, _ := fb.Get(other.Number); !slices.Contains(got.Labels, "assignee:ann") || len(got.Assignees) != 1 || got.Assignees[0].Login != "zoe" {
		t.Fatalf("project 22's #%s was written: labels %q, assignees %+v", n, got.Labels, got.Assignees)
	}
	if got, _ := fa.Get(n); slices.Contains(got.Labels, "assignee:ann") {
		t.Fatalf("project 11's #%s kept its label: %q", n, got.Labels)
	}
}
