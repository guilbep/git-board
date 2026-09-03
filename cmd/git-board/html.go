package main

import (
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/internal/board"
	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio"
	"github.com/guilbep/git-board/internal/tui"
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
trusted-network stance as "git-board-server", see README. A view whose
filter uses "@me" resolves once at generation time against --me/
GIT_BOARD_ME, the same way it does for cmd/git-board-server's single
shared "current user" — not per-viewer, there's no login here.`,
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

	b, err := gitbugio.OpenCwd()
	if err != nil {
		return err
	}
	defer b.Close()

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", outputDir, err)
	}

	names := make([]string, 0, len(cfg.Views))
	for name := range cfg.Views {
		names = append(names, name)
	}
	sort.Strings(names)

	generated := time.Now()
	for _, name := range names {
		if err := writeViewPage(outputDir, name, cfg.Views[name], b, cfg, me, generated); err != nil {
			return fmt.Errorf("view %q: %w", name, err)
		}
		fmt.Fprintf(out, "%s.html\n", name)
	}

	if err := writeIndexPage(outputDir, names, generated); err != nil {
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
	Layout    string
	Buckets   []htmlBucket
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
func writeViewPage(outputDir, name string, view config.View, b *gitbugio.Backend, cfg *config.Config, me string, generated time.Time) error {
	issues, err := board.LoadMatching(b, cfg, view.Filter, me)
	if err != nil {
		return err
	}
	board.SortByField(issues, view.Sort, cfg)

	data := pageData{
		ViewName:  name,
		Layout:    view.Layout,
		Buckets:   toHTMLBuckets(board.BucketBy(issues, view.Group), cfg),
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
			known, other := tui.ParseLabels(issue.Labels, cfg.Fields)
			issues[j] = htmlIssue{
				ID:     shortID(issue.ID),
				Title:  issue.Title,
				Status: issue.Status,
				Known:  known,
				Other:  other,
			}
		}
		out[i] = htmlBucket{Group: bucket.Group, Issues: issues}
	}
	return out
}

func writeIndexPage(outputDir string, viewNames []string, generated time.Time) error {
	f, err := os.Create(filepath.Join(outputDir, "index.html"))
	if err != nil {
		return err
	}
	defer f.Close()
	return indexTmpl.Execute(f, struct {
		Views     []string
		Generated time.Time
	}{Views: viewNames, Generated: generated})
}

var tmplFuncs = template.FuncMap{
	"displayName": tui.DisplayFieldName,
	"join":        func(ss []string, sep string) string { return strings.Join(ss, sep) },
}

// pageCSS is shared by every generated page — inlined, no external assets,
// so the site works from a plain rsync target with no build step.
const pageCSS = `
body { font-family: system-ui, sans-serif; margin: 2rem; color: #1a1a1a; }
a { color: #0b5fff; }
table { border-collapse: collapse; width: 100%; margin-bottom: 1.5rem; }
th, td { border: 1px solid #ddd; padding: 0.4rem 0.6rem; text-align: left; vertical-align: top; }
th { background: #f5f5f5; }
h2 { margin-top: 2rem; }
.fields div { font-size: 0.9rem; }
.field-name { font-weight: 600; }
.other { color: #777; font-size: 0.85rem; }
.board { display: flex; gap: 1rem; align-items: flex-start; overflow-x: auto; }
.column { flex: 0 0 260px; background: #fafafa; border: 1px solid #ddd; border-radius: 6px; padding: 0.6rem; }
.card { background: #fff; border: 1px solid #ddd; border-radius: 4px; padding: 0.5rem; margin-bottom: 0.5rem; }
.card .title { font-weight: 600; }
footer { margin-top: 2rem; color: #999; font-size: 0.8rem; }
`

const pageTmplSrc = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>{{.ViewName}} — git-board</title>
<style>` + pageCSS + `</style>
</head>
<body>
<p><a href="index.html">&larr; all views</a></p>
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
<table>
<thead><tr><th>Title</th><th>Status</th><th>Fields</th><th>Other labels</th></tr></thead>
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
{{end}}
{{end}}
<footer>Generated {{.Generated.Format "2006-01-02 15:04:05 MST"}} by git-board html — read-only, no auth.</footer>
</body>
</html>
`

const indexTmplSrc = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>git-board</title>
<style>` + pageCSS + `</style>
</head>
<body>
<h1>git-board</h1>
{{if .Views}}
<ul>
{{range .Views}}<li><a href="{{.}}.html">{{.}}</a></li>{{end}}
</ul>
{{else}}
<p>No views configured in .git-board.toml.</p>
{{end}}
<footer>Generated {{.Generated.Format "2006-01-02 15:04:05 MST"}} by git-board html.</footer>
</body>
</html>
`

var (
	pageTmpl  = template.Must(template.New("page").Funcs(tmplFuncs).Parse(pageTmplSrc))
	indexTmpl = template.Must(template.New("index").Funcs(tmplFuncs).Parse(indexTmplSrc))
)
