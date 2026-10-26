package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/release"
	"github.com/guilbep/git-board/internal/release/releasetest"
	"github.com/guilbep/git-board/internal/store"
)

const htmlTestConfig = `
[fields.status]
single = true
options = ["backlog", "ready", "done"]

[fields.prio]
single = true
options = ["p0", "p1"]

[fields.assignee]
single = false

[views.current-sprint]
layout = "board"
filter = "is:open"
group = "status"
sort = "prio asc"

[views.backlog]
layout = "table"
filter = "is:open status:backlog"
sort = "prio asc"
`

// newHTMLTestRepo seeds a real on-disk git-bug repo with four open issues
// spread across two statuses/prios (so grouping/sorting has something real
// to show), a raw non-field label ("needs-triage", to check known-vs-other
// rendering), and one closed issue that no configured view's filter should
// ever surface. Two of the open issues share status:ready: prio:p0 ("Speed
// up build") is created BEFORE prio:p1 ("Fix invoice bug"). git-bug's
// unsorted listing order is newest-first (query.NewQuery's default is
// OrderByCreation/OrderDescending — checked, not assumed), so without an
// explicit sort the p1 issue would list before the p0 one — the opposite of
// the view's "sort = \"prio asc\"" — letting a sort test tell real
// board.SortByField output apart from mere listing order. Chdirs the test
// into it and writes htmlTestConfig, mirroring newTestRepo in cmd_test.go
// but with real [views.*] entries.
func newHTMLTestRepo(t *testing.T) {
	t.Helper()
	dir, b := seedRepo(t)
	newLabeled := func(title string, labels ...string) string {
		id, err := b.NewIssue(title, "", labels)
		if err != nil {
			t.Fatalf("new issue %q: %v", title, err)
		}
		return id
	}
	newLabeled("Speed up build", "status:ready", "prio:p0")
	newLabeled("Fix invoice bug", "status:ready", "prio:p1", "assignee:guilbep", "needs-triage")
	newLabeled("Groom backlog item", "status:backlog", "prio:p0")
	if err := b.CloseBug(newLabeled("Write specs", "status:done")); err != nil {
		t.Fatalf("close issue: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, configPath), []byte(htmlTestConfig), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Chdir(dir)
}

func readGenerated(t *testing.T, siteDir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(siteDir, name))
	if err != nil {
		t.Fatalf("reading %s: %v", name, err)
	}
	return string(data)
}

func TestHTMLGeneratesOnePageAndIndexPerView(t *testing.T) {
	newHTMLTestRepo(t)
	site := filepath.Join(t.TempDir(), "site")

	out, err := execute(t, []string{"html", "-o", site})
	if err != nil {
		t.Fatalf("execute: %v\noutput:\n%s", err, out)
	}

	for _, name := range []string{"current-sprint.html", "backlog.html", "index.html"} {
		if _, err := os.Stat(filepath.Join(site, name)); err != nil {
			t.Errorf("expected %s to exist: %v", name, err)
		}
	}

	index := readGenerated(t, site, "index.html")
	if !strings.Contains(index, `href="current-sprint.html"`) {
		t.Errorf("index.html missing link to current-sprint.html:\n%s", index)
	}
	if !strings.Contains(index, `href="backlog.html"`) {
		t.Errorf("index.html missing link to backlog.html:\n%s", index)
	}
}

// The current-sprint view is is:open, so it must show both open issues and
// never the closed "Write specs" bug — the filter isn't reimplemented here,
// just applied via internal/board same as everywhere else.
func TestHTMLRespectsViewFilter(t *testing.T) {
	newHTMLTestRepo(t)
	site := filepath.Join(t.TempDir(), "site")

	if _, err := execute(t, []string{"html", "-o", site}); err != nil {
		t.Fatalf("execute: %v", err)
	}

	page := readGenerated(t, site, "current-sprint.html")
	if !strings.Contains(page, "Fix invoice bug") {
		t.Errorf("expected open issue in current-sprint.html:\n%s", page)
	}
	if !strings.Contains(page, "Groom backlog item") {
		t.Errorf("expected open issue in current-sprint.html:\n%s", page)
	}
	if strings.Contains(page, "Write specs") {
		t.Errorf("closed issue should not appear in an is:open view:\n%s", page)
	}

	// backlog's filter narrows further to status:backlog — "Fix invoice bug"
	// (status:ready) must not appear even though it's open.
	backlogPage := readGenerated(t, site, "backlog.html")
	if !strings.Contains(backlogPage, "Groom backlog item") {
		t.Errorf("expected status:backlog issue in backlog.html:\n%s", backlogPage)
	}
	if strings.Contains(backlogPage, "Fix invoice bug") {
		t.Errorf("status:ready issue should be excluded from the backlog view's filter:\n%s", backlogPage)
	}
}

// Known fields (status, prio, assignee) must render with their display name
// and value, never as raw "status:ready" label text; the one genuinely
// unrecognized label ("needs-triage") is the only raw label allowed through.
func TestHTMLRendersKnownFieldsNotRawLabels(t *testing.T) {
	newHTMLTestRepo(t)
	site := filepath.Join(t.TempDir(), "site")

	if _, err := execute(t, []string{"html", "-o", site}); err != nil {
		t.Fatalf("execute: %v", err)
	}

	page := readGenerated(t, site, "current-sprint.html")
	for _, want := range []string{"Status:", "ready", "Prio:", "p1", "Assignee:", "guilbep", "needs-triage"} {
		if !strings.Contains(page, want) {
			t.Errorf("expected %q in current-sprint.html:\n%s", want, page)
		}
	}
	if strings.Contains(page, "status:ready") {
		t.Errorf("raw label %q leaked into rendered output:\n%s", "status:ready", page)
	}
	if strings.Contains(page, "prio:p1") {
		t.Errorf("raw label %q leaked into rendered output:\n%s", "prio:p1", page)
	}
}

// current-sprint groups by status ("board" layout): the two open issues
// carry different status values, so they land in separate columns/buckets —
// board.BucketBy's behavior, not reimplemented here.
func TestHTMLGroupingReflectsBoardBucketBy(t *testing.T) {
	newHTMLTestRepo(t)
	site := filepath.Join(t.TempDir(), "site")

	if _, err := execute(t, []string{"html", "-o", site}); err != nil {
		t.Fatalf("execute: %v", err)
	}

	page := readGenerated(t, site, "current-sprint.html")
	// One column per configured group value; each present exactly once as a
	// column heading, distinct from the "Status:" field label rendered per
	// card, so anchor on the <h2> group heading form.
	if !strings.Contains(page, "<h2>ready</h2>") {
		t.Errorf("expected a %q group column heading:\n%s", "ready", page)
	}
	if !strings.Contains(page, "<h2>backlog</h2>") {
		t.Errorf("expected a %q group column heading:\n%s", "backlog", page)
	}
	if !strings.Contains(page, `class="board"`) {
		t.Errorf("expected board-layout columns in current-sprint.html:\n%s", page)
	}

	// backlog.html has no "board" layout — it renders a flat table, not
	// board columns, even though it also happens to bucket into one group.
	backlogPage := readGenerated(t, site, "backlog.html")
	if strings.Contains(backlogPage, `class="board"`) {
		t.Errorf("table-layout view should not render board columns:\n%s", backlogPage)
	}
	if !strings.Contains(backlogPage, "<table>") {
		t.Errorf("expected a table in backlog.html:\n%s", backlogPage)
	}
}

// current-sprint's "ready" column holds two issues created in prio:p1-then-
// prio:p0 order — the opposite of the view's "sort = \"prio asc\"". If the
// output merely reflected creation/listing order, "Fix invoice bug" (p1)
// would come first; board.SortByField must reorder it after "Speed up
// build" (p0).
func TestHTMLSortReflectsBoardSortByField(t *testing.T) {
	newHTMLTestRepo(t)
	site := filepath.Join(t.TempDir(), "site")

	if _, err := execute(t, []string{"html", "-o", site}); err != nil {
		t.Fatalf("execute: %v", err)
	}

	page := readGenerated(t, site, "current-sprint.html")
	p0Index := strings.Index(page, "Speed up build")
	p1Index := strings.Index(page, "Fix invoice bug")
	if p0Index < 0 || p1Index < 0 {
		t.Fatalf("expected both ready-status issues in current-sprint.html:\n%s", page)
	}
	if p0Index > p1Index {
		t.Errorf("expected prio:p0 (%q) before prio:p1 (%q) under \"sort = prio asc\", got p0 at %d, p1 at %d:\n%s",
			"Speed up build", "Fix invoice bug", p0Index, p1Index, page)
	}
}

func TestHTMLNoViewsConfiguredWritesEmptyIndex(t *testing.T) {
	newTestRepo(t) // testConfig in cmd_test.go has no [views.*]
	site := filepath.Join(t.TempDir(), "site")

	out, err := execute(t, []string{"html", "-o", site})
	if err != nil {
		t.Fatalf("execute: %v\noutput:\n%s", err, out)
	}

	index := readGenerated(t, site, "index.html")
	if !strings.Contains(index, "No views configured") {
		t.Errorf("expected the no-views message in index.html:\n%s", index)
	}
}

func TestHTMLDefaultsOutputToSiteDir(t *testing.T) {
	newHTMLTestRepo(t)

	if _, err := execute(t, []string{"html"}); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if _, err := os.Stat(filepath.Join(".", defaultHTMLOutput, "index.html")); err != nil {
		t.Errorf("expected %s/index.html with no -o flag: %v", defaultHTMLOutput, err)
	}
}

// The page inlines the EPFL design system standalone: the token file's
// relative @import "fonts.css" can't resolve on a published site, so its
// @font-face rules (Suisse Int'l from enac-it-fonts.epfl.ch) must be inlined
// in its place, and the logo must be a data URI, not a file to ship.
func TestHTMLInlinesEPFLDesignSystem(t *testing.T) {
	newHTMLTestRepo(t)
	site := filepath.Join(t.TempDir(), "site")

	if _, err := execute(t, []string{"html", "-o", site}); err != nil {
		t.Fatalf("execute: %v", err)
	}

	for _, name := range []string{"index.html", "current-sprint.html"} {
		page := readGenerated(t, site, name)
		if strings.Contains(page, "@import") {
			t.Errorf("%s: unresolvable @import left in the inlined CSS", name)
		}
		for _, want := range []string{
			"https://enac-it-fonts.epfl.ch/SuisseIntl-Regular-WebS.woff2",
			`class="epfl-header"`,
			`mask: url("data:image/svg+xml;base64,`,
		} {
			if !strings.Contains(page, want) {
				t.Errorf("%s: expected %q", name, want)
			}
		}
	}
	if page := readGenerated(t, site, "current-sprint.html"); !strings.Contains(page, `class="active" aria-current="page">current-sprint<`) {
		t.Errorf("current-sprint.html: expected its own nav entry marked active")
	}
}

// TestHTMLWritesReleases: with [release], the export has releases.html — the
// environment strip, the releases table and each release's notes inline —
// and without it, no such file is written.
func TestHTMLWritesReleases(t *testing.T) {
	r, _, fx := releasetest.NewRunner(t)
	t.Chdir(fx.App)

	c := releasetest.Git(t, fx.App, "rev-list", "--max-parents=0", "HEAD")
	fx.Tag(t, c, "v1.0.1-rc.1")
	rec, err := r.B.CreateRelease(store.ReleaseRecord{Commit: c})
	if err != nil {
		t.Fatalf("CreateRelease: %v", err)
	}
	if err := r.B.SetReleaseNotes(rec.ID, "Notes first line.\n\nAnd more."); err != nil {
		t.Fatalf("SetReleaseNotes: %v", err)
	}
	for i := range fx.ConfigBare {
		fx.SetOverlay(t, i, "stage", release.Deployed{Commit: c, Version: "v1.0.1-rc.1"})
	}
	if err := os.WriteFile(filepath.Join(fx.App, configPath), []byte(fx.Config(t)), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	// one git-bug cache per repo: release the fixture's before the command
	// opens its own
	if err := r.B.Close(); err != nil {
		t.Fatalf("close backend: %v", err)
	}

	site := filepath.Join(t.TempDir(), "site")
	out, err := execute(t, []string{"html", "-o", site})
	if err != nil {
		t.Fatalf("execute: %v\noutput:\n%s", err, out)
	}
	page := readGenerated(t, site, "releases.html")
	for _, want := range []string{"v1.0.1-rc.1", "stage", "Notes first line."} {
		if !strings.Contains(page, want) {
			t.Errorf("expected %q in releases.html:\n%s", want, page)
		}
	}

	// without [release], the export writes no releases.html
	newHTMLTestRepo(t)
	plain := filepath.Join(t.TempDir(), "site")
	if _, err := execute(t, []string{"html", "-o", plain}); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if _, err := os.Stat(filepath.Join(plain, "releases.html")); err == nil {
		t.Errorf("releases.html written without [release]")
	}
}
