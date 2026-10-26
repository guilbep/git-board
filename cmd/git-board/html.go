package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/internal/board"
	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/release"
	"github.com/guilbep/git-board/internal/store"
	"github.com/guilbep/git-board/internal/tui"
	boardweb "github.com/guilbep/git-board/web"
)

const defaultHTMLOutput = "site"

func newHtmlCmd() *cobra.Command {
	var output, me string

	cmd := &cobra.Command{
		Use:   "html",
		Short: "Generate a static, read-only HTML site, one page per view",
		Long: `Generate a static HTML site: one page per configured [views.*] in
.git-board.toml, plus an index.html linking to all of them. Read-only —
writes nothing to git-bug, unlike everything else in this CLI except
ls/view/get/lint.

Meant to be cron-published so PM/non-dev users can browse the board
without a CLI or the (also-unauthenticated) live web UI, e.g. a periodic
job running:

  git-board html -o /var/www/board && rsync -a --delete /var/www/board/ host:/srv/board/

-o's directory is never emptied first, so --delete on the rsync side (not
just -a) matters once a view gets renamed or removed — otherwise its old
page lingers on the published copy with no link to it.

No auth, no JS required to view (plain server-rendered HTML) — same
trusted-network stance as the web UI ("git-board serve"), see README. A
view whose filter uses "@me" resolves once at generation time against
--me/GIT_BOARD_ME, the same way it does for the server's single shared
"current user" — not per-viewer, there's no login here.`,
		Example: `  git-board html
  git-board html -o /var/www/board --me guilbep`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runHTML(cmd.OutOrStdout(), output, resolveMe(me))
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", defaultHTMLOutput, "output directory")
	meFlag(cmd, &me)
	return cmd
}

func runHTML(out io.Writer, outputDir, me string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	b, err := store.OpenCwd()
	if err != nil {
		return err
	}
	defer b.Close()

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", outputDir, err)
	}

	names := cfg.ViewNames()

	generated := time.Now()
	for _, name := range names {
		if err := writeViewPage(outputDir, name, names, cfg.Views[name], b, cfg, me, generated); err != nil {
			return fmt.Errorf("view %q: %w", name, err)
		}
		fmt.Fprintf(out, "%s.html\n", name)
	}

	releases := cfg.Release.Enabled()
	if releases {
		if err := writeReleasesPage(outputDir, names, b, cfg, generated); err != nil {
			return fmt.Errorf("releases: %w", err)
		}
		fmt.Fprintln(out, "releases.html")
	}

	if err := writeIndexPage(outputDir, names, generated, releases); err != nil {
		return fmt.Errorf("index: %w", err)
	}
	fmt.Fprintln(out, "index.html")
	return nil
}

// htmlIssue is one issue ready to render: known fields already split from
// raw labels via tui.ParseLabels (the same recognition logic the TUI's
// detail view uses), so the page never spams "ns:value" label text.
type htmlIssue struct {
	ID     string
	Title  string
	Status string
	Known  []tui.KnownField
	Other  []string
}

type htmlBucket struct {
	Group  string
	Issues []htmlIssue
}

type pageData struct {
	ViewName  string
	Views     []string // every view, for the header navigation
	Layout    string
	Buckets   []htmlBucket
	Releases  bool // the project has [release]: the header links releases.html
	Generated time.Time
}

// writeViewPage loads and renders exactly one view, via internal/board's
// LoadMatching/SortByField/BucketBy — the same filter/sort/group semantics
// the CLI's ls/view and the web API already use. No filtering/sorting/
// grouping logic is reimplemented here.
//
// An empty view.Filter is passed through as-is (matching runView's own
// behavior in cmd.go — LoadMatching then applies no conditions, so every
// issue, open or closed, matches). The web API's handleListIssues disagrees
// with runView here: it defaults an empty filter to board.DefaultFilter
// ("is:open"). The two existing consumers already disagree with each
// other; this follows "view" since "html" renders configured views, the
// same thing "view" does.
func writeViewPage(outputDir, name string, views []string, view config.View, b *store.Backend, cfg *config.Config, me string, generated time.Time) error {
	issues, err := board.LoadMatching(b, cfg, view.Filter, me)
	if err != nil {
		return err
	}
	board.SortByField(issues, view.Sort, cfg)

	data := pageData{
		ViewName:  name,
		Views:     views,
		Layout:    view.Layout,
		Buckets:   toHTMLBuckets(board.BucketBy(issues, view.Group), cfg),
		Releases:  cfg.Release.Enabled(),
		Generated: generated,
	}

	f, err := os.Create(filepath.Join(outputDir, name+".html"))
	if err != nil {
		return err
	}
	defer f.Close()
	return pageTmpl.Execute(f, data)
}

func toHTMLBuckets(buckets []board.Bucket, cfg *config.Config) []htmlBucket {
	out := make([]htmlBucket, len(buckets))
	for i, bucket := range buckets {
		issues := make([]htmlIssue, len(bucket.Issues))
		for j, issue := range bucket.Issues {
			known, other := tui.ParseLabels(issue.Labels)
			issues[j] = htmlIssue{
				ID:     shortID(issue.ID),
				Title:  issue.Headline(),
				Status: issue.Status,
				Known:  known,
				Other:  other,
			}
		}
		out[i] = htmlBucket{Group: bucket.Group, Issues: issues}
	}
	return out
}

func writeIndexPage(outputDir string, viewNames []string, generated time.Time, releases bool) error {
	f, err := os.Create(filepath.Join(outputDir, "index.html"))
	if err != nil {
		return err
	}
	defer f.Close()
	return indexTmpl.Execute(f, struct {
		ViewName  string // none: the index is no view (pageHeader marks none active)
		Views     []string
		Releases  bool
		Generated time.Time
	}{Views: viewNames, Releases: releases, Generated: generated})
}

// releasesHTMLData is the releases.html page: the environment strip and one
// row per release, then the release notes (internal/release Runner.Notes).
type releasesHTMLData struct {
	ViewName  string // "Releases", so the header marks it active
	Views     []string
	Releases  bool
	Envs      []releasesHTMLEnv
	Rows      []releasesHTMLRow
	Notes     string
	Generated time.Time
}

type releasesHTMLEnv struct {
	Name    string
	Version string
	Class   string // gb-envmark gb-envmark-<role>, the strip's chip
}

type releasesHTMLRow struct {
	Tag    string
	Date   string
	RunsIn string // the environments running it, comma-separated
}

// writeReleasesPage writes releases.html: the environment strip and the
// releases table, each release's notes inline, as static HTML with the
// export's stylesheet. It reads the release state offline (no fetch), as
// the export must: the runner's offline Status.
func writeReleasesPage(outputDir string, views []string, b *store.Backend, cfg *config.Config, generated time.Time) error {
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	run, err := release.New(cfg, dir, b)
	if err != nil {
		return err
	}
	run.Offline = true
	ctx := context.Background()
	st, err := run.Status(ctx)
	if err != nil {
		return err
	}
	notes, err := run.Notes(ctx, true)
	if err != nil {
		return err
	}
	data := releasesHTMLData{ViewName: "Releases", Views: views, Releases: true, Notes: notes, Generated: generated}
	for _, e := range st.Envs {
		data.Envs = append(data.Envs, releasesHTMLEnv{Name: e.Name, Version: e.Version, Class: envMarkClass(e.Role)})
	}
	for _, r := range st.Releases {
		data.Rows = append(data.Rows, releasesHTMLRow{Tag: r.Tag.Name, Date: r.Tag.Date.Format("2006-01-02"), RunsIn: strings.Join(r.RunsIn, ", ")})
	}
	f, err := os.Create(filepath.Join(outputDir, "releases.html"))
	if err != nil {
		return err
	}
	defer f.Close()
	return releasesTmpl.Execute(f, data)
}

// envMarkClass is the strip chip's class for an environment's role, the
// same marks the web UI uses (web/src/releases.ts envMark): the last is
// prod, a candidate is stage, everything else dev.
func envMarkClass(role string) string {
	switch role {
	case "last":
		return "gb-envmark gb-envmark-prod"
	case "candidate":
		return "gb-envmark gb-envmark-stage"
	default:
		return "gb-envmark"
	}
}

var tmplFuncs = template.FuncMap{
	"displayName": tui.DisplayFieldName,
	"join":        func(ss []string, sep string) string { return strings.Join(ss, sep) },
}

// pageCSS is shared by every generated page — inlined, no external assets,
// so the site works from a plain rsync target with no build step. It is
// the EPFL design system (tokens, fonts, header/layout classes; see
// web/epfl.go) plus this page's own layout, written in its tokens only:
// no raw colors or px, borders not shadows, 2px radii.
//
// The logo is inlined as a data URI and drawn as a mask over
// var(--primary), like the web UI's header: git-board.svg is drawn
// black, the logo is red.
var pageCSS = boardweb.EPFLCSS + `
.logo-mark { display: block; height: 2rem; aspect-ratio: 1; background: var(--primary); mask: url("data:image/svg+xml;base64,` + base64.StdEncoding.EncodeToString(boardweb.LogoSVG) + `") center / contain no-repeat; }
body { margin: 0; }
.app-name { margin-left: var(--space-3); padding-left: var(--space-3); border-left: var(--border-w) solid var(--border); font-weight: var(--w-bold); font-size: var(--fs-h5); line-height: var(--lh-tight); white-space: nowrap; }
.epfl-breadcrumb { padding-left: var(--gutter); padding-right: var(--gutter); }
main { padding: var(--space-4) var(--gutter) var(--space-5); }
main > h1 { font-size: var(--fs-h3); margin: 0 0 var(--space-4); overflow-wrap: anywhere; }
h2 { font-size: var(--fs-h5); font-weight: var(--w-bold); line-height: var(--lh-normal); margin: var(--space-5) 0 var(--space-3); }
.table-scroll { overflow-x: auto; border: var(--border-w) solid var(--border); margin-bottom: var(--space-4); }
table { border-collapse: collapse; width: 100%; font-size: var(--fs-sm); line-height: var(--lh-normal); }
th, td { padding: var(--space-2) var(--space-3); text-align: left; vertical-align: top; border-bottom: var(--border-w) solid var(--border-subtle); }
th { background: var(--bg-subtle); border-bottom-color: var(--border); white-space: nowrap; }
tbody tr:last-child td { border-bottom: 0; }
td:first-child { min-width: 18rem; }
.fields div { font-size: var(--fs-sm); }
.field-name { color: var(--fg-muted); }
.other { color: var(--fg-muted); font-size: var(--fs-xs); }
.board { display: grid; grid-auto-flow: column; grid-auto-columns: minmax(16rem, 1fr); gap: var(--space-3); align-items: start; overflow-x: auto; padding-bottom: var(--space-3); }
.column { background: var(--bg-subtle); border: var(--border-w) solid var(--border-subtle); padding: var(--space-3); }
.column h2 { margin: 0 0 var(--space-3); font-size: var(--fs-sm); text-transform: uppercase; letter-spacing: .04em; }
.card { display: grid; gap: var(--space-1); background: var(--bg); border: var(--border-w) solid var(--border); border-radius: var(--radius); padding: var(--space-2) var(--space-3) var(--space-3); margin-bottom: var(--space-2); }
.card .title { font-size: 0.9375rem; font-weight: var(--w-bold); line-height: var(--lh-normal); overflow-wrap: anywhere; }
.views { list-style: none; margin: 0; padding: 0; border-top: var(--border-w) solid var(--border); max-width: 40rem; }
.views li { border-bottom: var(--border-w) solid var(--border); }
.views a { display: block; padding: var(--space-3) 0; font-weight: var(--w-bold); text-decoration: none; transition: color var(--t-fast); }
.views a:hover { color: var(--primary-hover); }
a:focus-visible { outline: var(--focus-w) solid var(--primary-hover); outline-offset: 0.125rem; }
footer { margin: 0 var(--gutter); padding: var(--space-3) 0 var(--space-4); border-top: var(--border-w) solid var(--border-subtle); color: var(--fg-muted); font-size: var(--fs-sm); }
/* ---- releases: the environment strip and the notes (releases.html) ---- */
.gb-envline { display: flex; flex-wrap: wrap; align-items: center; gap: .375rem; margin-bottom: var(--space-4); }
.gb-envmark { display: inline-flex; align-items: center; gap: .25rem; padding: 0 .4375rem; border: 1px solid var(--border-strong); border-radius: var(--radius); font-size: .75rem; font-weight: 700; line-height: 1.25rem; letter-spacing: .02em; white-space: nowrap; color: var(--fg); background: var(--bg); }
.gb-envmark-prod { background: var(--bg-dark); border-color: var(--bg-dark); color: var(--fg-inverse); }
.gb-envmark-stage { background: var(--tertiary); border-color: var(--tertiary); color: var(--on-tertiary); }
.gb-note { color: var(--fg-muted); font-size: var(--fs-sm); }
.notes { margin: 0; padding: var(--space-3); background: var(--bg-subtle); border: var(--border-w) solid var(--border); font-family: var(--font-mono); font-size: var(--fs-sm); line-height: var(--lh-normal); white-space: pre-wrap; overflow-wrap: anywhere; }
@media (max-width: 47.99rem) {
  .epfl-header { flex-wrap: wrap; height: auto; padding: var(--space-3) var(--space-3) 0; }
  .epfl-nav { flex-basis: 100%; margin-left: calc(-1 * var(--space-3)); margin-right: calc(-1 * var(--space-3)); padding: 0 var(--space-2); overflow-x: auto; }
  .epfl-nav a { white-space: nowrap; }
  .epfl-breadcrumb, main { padding-left: var(--space-3); padding-right: var(--space-3); }
  footer { margin: 0 var(--space-3); }
}
`

// pageHead is the <head> every page shares, after its <title>: viewport,
// the font origin preconnect (INSTALL.md of the EPFL design system;
// crossorigin, or the font server answers 403) and the inlined CSS.
var pageHead = `<meta name="viewport" content="width=device-width, initial-scale=1">
<link rel="preconnect" href="https://enac-it-fonts.epfl.ch" crossorigin>
<style>` + pageCSS + `</style>
</head>`

// pageHeader is the EPFL header: logo, app name and one nav entry per
// view, the current one marked with the red dot (class="active").
const pageHeader = `<header class="epfl-header" role="banner">
<a class="epfl-logo" href="index.html"><span class="logo-mark" role="img" aria-label="git-board"></span></a>
<span class="app-name">git-board</span>
<nav class="epfl-nav" aria-label="Views">{{range .Views}}<a href="{{.}}.html"{{if eq . $.ViewName}} class="active" aria-current="page"{{end}}>{{.}}</a>{{end}}{{if .Releases}}<a href="releases.html"{{if eq .ViewName "Releases"}} class="active" aria-current="page"{{end}}>Releases</a>{{end}}</nav>
</header>`

var pageTmplSrc = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>{{.ViewName}} — git-board</title>
` + pageHead + `
<body>
` + pageHeader + `
<nav class="epfl-breadcrumb" aria-label="Breadcrumb"><a href="index.html">All views</a><span class="sep">›</span><span>{{.ViewName}}</span></nav>
<main>
<h1>{{.ViewName}}</h1>
{{if eq .Layout "board"}}
<div class="board">
{{range .Buckets}}
  <div class="column">
    <h2>{{if .Group}}{{.Group}}{{else}}(all){{end}}</h2>
    {{range .Issues}}
    <div class="card">
      <div class="title">{{.Title}}</div>
      <div class="fields">{{range .Known}}<div><span class="field-name">{{displayName .Name}}:</span> {{join .Values ", "}}</div>{{end}}</div>
      {{if .Other}}<div class="other">{{join .Other ", "}}</div>{{end}}
    </div>
    {{end}}
  </div>
{{end}}
</div>
{{else}}
{{range .Buckets}}
{{if .Group}}<h2>{{.Group}}</h2>{{end}}
<div class="table-scroll">
<table>
<thead><tr><th>Title</th><th>State</th><th>Fields</th><th>Other labels</th></tr></thead>
<tbody>
{{range .Issues}}
<tr>
<td>{{.Title}}</td>
<td>{{.Status}}</td>
<td class="fields">{{range .Known}}<div><span class="field-name">{{displayName .Name}}:</span> {{join .Values ", "}}</div>{{end}}</td>
<td class="other">{{join .Other ", "}}</td>
</tr>
{{end}}
</tbody>
</table>
</div>
{{end}}
{{end}}
</main>
<footer>Generated {{.Generated.Format "2006-01-02 15:04:05 MST"}} by git-board html — read-only, no auth.</footer>
</body>
</html>
`

var indexTmplSrc = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>git-board</title>
` + pageHead + `
<body>
` + pageHeader + `
<main>
<div class="epfl-eyebrow">git-board</div>
<h1>Views</h1>
{{if .Views}}
<ul class="views">
{{range .Views}}<li><a href="{{.}}.html">{{.}}</a></li>{{end}}
</ul>
{{else}}
<p>No views configured in .git-board.toml.</p>
{{end}}
</main>
<footer>Generated {{.Generated.Format "2006-01-02 15:04:05 MST"}} by git-board html.</footer>
</body>
</html>
`

var releasesTmplSrc = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Releases — git-board</title>
` + pageHead + `
<body>
` + pageHeader + `
<nav class="epfl-breadcrumb" aria-label="Breadcrumb"><a href="index.html">All views</a><span class="sep">›</span><span>Releases</span></nav>
<main>
<h1>Releases</h1>
<div class="gb-envline" aria-label="Environments">
{{range .Envs}}<span class="{{.Class}}">{{.Name}}</span>{{if .Version}}<span class="gb-note">{{.Version}}</span>{{end}}{{end}}
</div>
<div class="table-scroll">
<table>
<thead><tr><th>Release</th><th>Date</th><th>Runs in</th></tr></thead>
<tbody>
{{range .Rows}}
<tr>
<td class="gb-mono">{{.Tag}}</td>
<td>{{.Date}}</td>
<td>{{if .RunsIn}}{{.RunsIn}}{{else}}nowhere{{end}}</td>
</tr>
{{end}}
</tbody>
</table>
</div>
{{if .Notes}}<h2>Release notes</h2>
<pre class="notes">{{.Notes}}</pre>{{end}}
</main>
<footer>Generated {{.Generated.Format "2006-01-02 15:04:05 MST"}} by git-board html — read-only, no auth.</footer>
</body>
</html>
`

var (
	pageTmpl     = template.Must(template.New("page").Funcs(tmplFuncs).Parse(pageTmplSrc))
	indexTmpl    = template.Must(template.New("index").Funcs(tmplFuncs).Parse(indexTmplSrc))
	releasesTmpl = template.Must(template.New("releases").Funcs(tmplFuncs).Parse(releasesTmplSrc))
)
