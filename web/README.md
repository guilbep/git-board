# git-board web UI

One board page talking to `cmd/git-board-server`'s JSON API: the saved
views of `.git-board.toml` as the header navigation (`#/?view=<name>`), a
filter/group/sort bar, each view in its `layout` (`board`: one column per
group, in the group field's configured option order; `table`: the CLI's
columns, one per configured field), and an issue side panel that edits
fields (single: a select of the configured options; multi: chips). See the
root [README.md](../README.md#web-ui) for the full picture and the no-auth
caveat.

## EPFL design system

The look is the ENAC IT4R EPFL design system
([EPFL-ENAC/epfl-design-skill](https://github.com/EPFL-ENAC/epfl-design-skill)),
vendored by copy in `epfl-design/` from commit
`55fc0a8b7fcff0bf5533923a88ed66ab41023c8d` (2026-10-09, merge of PR #5):
its `tokens/`, `css/` and `kits/quasar/epfl-quasar-brand.css`, unmodified,
in the same layout. `public/epfl-logo.svg` is its `assets/svg/epfl-logo.svg`.
`src/css/app.css` loads `css/epfl.css` then the Quasar brand mapping;
`web/epfl.go` embeds the same files for `git-board html`'s static site.

A copy, not the submodule its INSTALL.md suggests: `go install` and module
downloads never see submodule content, and the static site embeds these
files. To update, copy those paths again from a newer commit and change the
commit above.

Rules worth knowing before touching a style: tokens only (`var(--…)`, no
raw hex, no `px`, use `rem`), Suisse Int'l only (it loads from
`enac-it-fonts.epfl.ch`, for `*.epfl.ch` and, on the EPFL network or VPN,
`localhost`; elsewhere text falls back to Arial, which is expected; never
add font files), sentence case, no emoji, no gradients or card shadows,
2px radii, hover changes color or border only, 2px focus outline with an
offset. The skill's adherence linter checks the CSS:

```bash
# from the repo root, with the skill cloned next to this repo
npx --package=stylelint@16 --package=postcss-html -- \
  stylelint "web/src/**/*.{css,vue}" \
  --config ../epfl-design-skill/adherence/.stylelintrc.json
```

## Development

```bash
npm install
npm run dev       # Quasar dev server, http://localhost:9000
```

The dev server needs `cmd/git-board-server` running separately on `:8080`
(`go run ./cmd/git-board-server` from the repo root) — `src/api/board.ts`
points there via CORS when `import.meta.env.DEV` is true. Same-origin (no
CORS, no separate process) once built and embedded for production.

## Production build

```bash
npm run build      # writes web/dist/spa (Quasar's default SPA output dir)
```

Then `go build ./cmd/git-board-server` from the repo root embeds `web/dist`
(`web/embed.go`) and serves `dist/spa` from it, into a single binary that
serves both the API and the UI.

## Type-check and test

```bash
npm run typecheck   # vue-tsc --noEmit
npm test            # node --test src/facets.test.ts
```

The filters and sort (`src/facets.ts`) run in the page, so they're tested
here, with Node's own test runner (Node strips the types; no Vitest). The
API handlers are covered on the Go side (`cmd/git-board-server/server_test.go`).
