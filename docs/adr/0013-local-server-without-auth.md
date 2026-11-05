# ADR 0013: The web server has no login: loopback by default, host and origin checked

- Status: accepted (recorded 2026-11-04, decided earlier)
- Date: 2026-09-02 ("No auth in v1, matching git-bug's own `webui`
  precedent", `docs/implementation-plan.md` › Status update (2026-09-02):
  web UI landed). The host check against DNS rebinding came later, by 2026-10-20 at the
  latest (the shallow clone's oldest commit already has it).
- Sources: `internal/server/serve.go:6-7`, `internal/server/cors.go`,
  `README.md` › "No auth in v1".

## Context

`git board` starts a server per repository on the developer's machine
(ADR 0012). The server reads and writes the board, runs git, starts
agents in worktrees, and through the forge sync posts to GitHub and GitLab
with the user's tokens (ADR 0010). Who writes is `board.me`, the clone's
own setting (ADR 0011), not a login. A browser on the same machine visits
other sites, which can send requests to `localhost`.

## Decision

**The server has no accounts and no login. It trusts whoever can reach its
port, and makes sure only this machine's own pages can.**

- **Loopback by default:** `127.0.0.1` and `[::1]`, port 8080 or the next
  free one (`internal/server/serve.go:62-74`, `:126-170` `listen`,
  `loopbacks`). `--addr` opens it wider, by the user's choice; the names
  it exposes are recorded (`exposed`).
- **Every request must name this server in `Host`**, reads included, so a
  page that points its own domain at 127.0.0.1 (DNS rebinding) is refused
  (`internal/server/cors.go:47-58` `withGuard`, `servedHost`).
- **Writes must come from this server's own origin:** a foreign `Origin`
  is refused (`cors.go:64-67`), and a write's body must be JSON, which a
  cross-site page can't send without a CORS preflight that only the dev
  server (`localhost:9000`) passes (`cors.go:11-19`, `:68-79`). An upload,
  which can't be JSON, needs the page's own `X-Git-Board-Upload` header.
- Clients without an `Origin` (curl, scripts) are the user's own.
- Upgrade path when that stops being enough: a reverse proxy doing auth in
  front, or a token check in a middleware (`README.md`, "No auth in v1").

## Alternatives considered

Partly recorded (no auth, following git-bug's `webui`); the rest
reconstructed. Scored on: nothing to set up (30), safe on a laptop with a
browser open (30), usable by a team on a network (20), cost (20).

| Option | Score | Why not |
| --- | --- | --- |
| **No login, loopback, host and origin guard (this decision)** | 80 | |
| No login and no guard (the v1 of 2026-09-02) | 45 | Any site the user visits could write the board and post to forges with the user's tokens |
| A token in the URL or a cookie set at start (Jupyter's way) | 65 | Safer against other local users; a link to copy, and one more thing to explain; not built |
| Accounts and login (OIDC, forge OAuth) | 40 | A user store, sessions, and a server to run: the server git-board avoids |
| Unix socket instead of TCP | 35 | Browsers can't open one |

## Consequences

Good:

- `git board` opens a working board with no setup.
- Another site can't read or write it through the user's browser.

Bad, and accepted:

- Anyone who can reach the port can read and write every issue, and post
  to forges as the user. `--addr 0.0.0.0` on an untrusted network exposes
  all of it. On every interface the server still answers only IP literals
  and this machine's own name (`internal/server/cors.go`, since
  2026-11-04), so DNS rebinding stays refused there too; running a
  branch's tests and signing their results (`/tests/run`, `/checks/setup`)
  and testing or copying to a datastore answer only on localhost.
- Another local user on a shared machine can reach `127.0.0.1`.
- Attribution is `board.me`, which the page asks for and anyone with
  access can change: writes are signed by a claim, not an identity.
- `git-board html` publishes a static, unauthenticated view in the same
  spirit (`README.md`).

## Amendment (2026-11-05): no code runs from beyond loopback

A review of v2's server found that two JSON requests ran any shell
command: `PATCH /api/config` set `[release] build`, and `POST
/api/releases` ran it with `sh -c`, from any client of a server started
with `--addr 0.0.0.0`. "Anyone who can reach the port can write every
issue" was accepted above; running code as the user never was, and
`--addr`'s help says it exposes the board and the forge tokens, not a
shell.

**The rule:** a request that makes this machine run code, or changes
what code a later run runs, passes one gate, v1's `localWrite`
(`internal/server/refs.go`), now in v2 too (`next/surface/http/guard.go`
`runsCode`, `code.go` `codeKeys`). It is refused when the server
listens beyond loopback, whoever asks, and otherwise asks a same-host
`Origin`, the board's own page (a script without an `Origin` is refused
too). In v2 it gates the releases' cut, promotion, adoption, deploy and
rollback, and a config write that changes `[release]`, `[cycle.tests]`,
`[cycle.checks]` or `cycle.signers` (docs/rewrite/api.md, "The code
gate"); in v1 the tests' run, the checks' setup and a branch made (as
before), and since v0.11.7 the release routes and a config write that
changes such a key. A server shown on a network serves the board; the
releases are cut from the machine's own page.

## Left open

- **A hosted git-board** (ADR 0002 › Context: "a hosted git-board", "people
  who never clone") would serve people over a network: it needs auth, and
  this ADR doesn't give it. The upgrade path above is a note, not a
  decision.
- `internal/server/serve.go:29` still documents `Serve` as serving "the
  git-bug repository in the working directory" (ADR 0003).
