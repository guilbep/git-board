# git-board's web UI

The web app `git board` serves (ADR 0012, ADR 0021), over its HTTP API
([docs/rewrite/api.md](../docs/rewrite/api.md)): the board page
(the saved views as tabs, a filter, group and sort bar, each view in its
`layout`: `table`, `board`, `roadmap`), the issue panel, the inbox and
Settings. What changed from v1's web app, and why:
[docs/rewrite/web.md](../docs/rewrite/web.md).

`src/api` is the one typed client: a module per part of api.md, every
write from the version read (`If-Match`), one error shape. No component
calls `fetch()`.

## EPFL design system

The look is the ENAC IT4R EPFL design system
([EPFL-ENAC/epfl-design-skill](https://github.com/EPFL-ENAC/epfl-design-skill)),
vendored by copy in `epfl-design/` from commit
`55fc0a8b7fcff0bf5533923a88ed66ab41023c8d` (2026-10-09, merge of PR #5):
its `tokens/`, `css/` and `kits/quasar/epfl-quasar-brand.css` (no longer
loaded: the app has no Quasar since ADR 0023), unmodified,
in the same layout. `public/epfl-logo.svg` is its `assets/svg/epfl-logo.svg`.
`src/css/app.css` loads `css/epfl.css`;

A copy, not the submodule its INSTALL.md suggests: `go install` and module
downloads never see submodule content. To update, copy those paths again from a newer commit and change the
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
npm run dev       # Vite's dev server, http://localhost:9000
```

The dev server needs `git board serve` running separately on `:8080`
(`go run ./cmd/git-board serve` from the repo root, on a
throwaway board): `src/api/http.ts` points there when `import.meta.env.DEV`
is true; the server takes the dev server's origin (ADR 0013).

## Production build

```bash
npm run build      # writes web/dist/spa
```

Then `go build ./cmd/git-board` embeds `web/dist`
(`embed.go`) and serves `dist/spa` from it.

## Type-check and test

```bash
npm run typecheck   # vue-tsc --noEmit
npm test            # node --test 'src/**/*.test.ts'
npm run e2e         # Playwright, e2e/: make web-e2e from the root
```

The pure modules (no `@/` imports) are tested with Node's own runner, which
strips the types. The browser tests (`e2e/`) run on `git board serve`
over throwaway boards that `e2e/serve.mjs` makes: a git board with a GitHub
origin and a fake GitHub, and a Markdown board.
