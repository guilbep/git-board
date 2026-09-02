# git-board web UI

One board page (view/filter picker, grouped table, click-to-edit detail
dialog) talking to `cmd/git-board-server`'s JSON API. See the root
[README.md](../README.md#web-ui) for the full picture and the no-auth
caveat.

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
npm run build      # writes web/dist (flat — see quasar.config.ts distDir)
```

Then `go build ./cmd/git-board-server` from the repo root embeds `web/dist`
(`web/embed.go`) into a single binary that serves both the API and the UI.

## Type-check

```bash
npm run typecheck   # vue-tsc --noEmit
```

No test framework came with the generated scaffold (no Vitest, no test
script in `package.json`) — not added for this MVP. `internal/board`'s
sort/group logic and the API handlers are covered on the Go side instead
(`cmd/git-board-server/server_test.go`).
