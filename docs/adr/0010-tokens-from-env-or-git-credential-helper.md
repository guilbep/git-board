# ADR 0010: No secret is stored, and the server answers on localhost

- Status: accepted; merged 2026-11-09 from ADR 0010 (decided 2026-10-24)
  and ADR 0013 (decided 2026-09-02, amended 2026-11-05); amended
  2026-11-09 with the login in the browser (section 5, #101)
- Date: 2026-09-02, 2026-10-24, 2026-11-05, 2026-11-09
- Absorbs [ADR 0013](0013-local-server-without-auth.md) (a local server
  with no login, and its code gate).
- Spec: [Forges › Accounts and tokens](../spec/forges.md). Guide:
  [Forges](../guide/forges.md).

## Context

The sync and git-board's own forge calls need a token per account and
per forge host. Anyone can edit the committed `.git-board.toml` in a pull
request. CI has its tokens in its environment. Developers usually keep a
token in git's credential helper already.

`git board` starts a server per repository on the developer's machine.
The server reads and writes the board, runs git and posts to the forges
with the user's tokens. A browser on the same machine visits other sites,
and those sites can send requests to `localhost`.

## Decision

### 1. A token comes from the environment, else from git's credential helper

git-board stores no token itself, in git or anywhere.

- An environment variable counts only for its own host. `GITHUB_TOKEN` is
  for `github.com`, `GITHUB_ENTERPRISE_TOKEN` for `GITHUB_ENTERPRISE_HOST`
  and `GITLAB_TOKEN` for `GITLAB_HOST` (default `gitlab.com`).
  `GH_TOKEN` is never read (`forge/github/token.go`).
- Else the token comes from `git credential fill` for the host, with the
  account as `username`. A helper with nothing for that login is asked
  once more for the host alone.
- The helper never prompts, and has 10 seconds at most
  (`forge/credential.go`).
- A person's token counts only when the forge says that it is theirs.
- Settings and `git board forge login` store a new token with
  `git credential approve` (`app/tokens.go`). No page shows a token back.
- A base URL is `https://`, except on loopback.
- The GitHub Project bridge takes the GitHub forge's token. It is the
  posting account's of the first GitHub table, else yours on the host,
  else any account's on `github.com`.

The S3 keys for attachments are plain environment variables of the server
([ADR 0002](0002-datastores.md), section 7).

### 2. The server has no login, and only this machine's pages reach it

The server has no accounts. It trusts whoever reaches its port, and makes
sure that only this machine's own pages can.

- It listens on `127.0.0.1` and `[::1]`, port 8080 by default
  (`surface/http/serve.go`). `--addr` opens it wider, by the user's
  choice.
- Every request names this server in `Host`, reads included. So a page
  that points its own domain at 127.0.0.1 (DNS rebinding) is refused.
  Beyond loopback, the server answers only IP literals and this
  machine's own name.
- A write comes from this server's own origin, and its body is JSON. A
  page of another site cannot send JSON without a CORS preflight, which
  fails. An upload needs the page's `X-Git-Board-Upload` header instead.
- A client without an `Origin` (curl, a script) is the user's own.
- Who writes is `board.me`, the clone's own setting
  ([ADR 0011](0011-shared-settings-in-git-board-toml.md)).

### 3. No code runs from beyond loopback

A request that makes this machine run code, or changes what code a later
run runs, passes one gate (`surface/http/guard.go`, `runsCode`). The gate
covers the releases' cut, promotion, adoption, deploy and rollback, and a
configuration write that changes `[release]`, `[cycle.tests]`,
`[cycle.checks]` or `cycle.signers` (`surface/http/code.go`).

The gate refuses the request when the server listens beyond loopback,
whoever asks. Otherwise the request must come from loopback, with the
board's own page as its `Origin`. A script without an `Origin` is
refused.

### 4. What reaches a secret or another store answers on localhost alone

Some routes answer on a loopback address alone (`localRoutes`,
`loopbackOnly`):

- a datastore's value, which can hold a password, and a copy into one
- a token stored in git's credential helper, pasted or from the login in
  the browser (section 5)
- a git remote added
- a configuration file started from a template's path or URL, which
  reads this machine's files or the network. A built-in template's name
  answers anywhere.

### 5. On github.com, a person logs in in the browser

A pasted token makes a person create a token on the forge, choose its
scopes, and copy it. On `github.com`, git-board also uses the OAuth device
flow (RFC 8628) of its own OAuth App (`forge/github/device.go`,
`app/device.go`).

1. git-board asks GitHub for a code, and shows the code and GitHub's
   address.
2. The person types the code on GitHub and approves the scopes `repo`
   and `project`.
3. git-board asks GitHub whose the token is, and stores it for that login
   with `git credential approve`, as in section 1.

The rules of sections 1 and 4 stay:

- The token goes to git's credential helper only. The device code and the
  token never reach the page, a log or a file.
- The web routes answer on loopback alone (routes 62 and 63).
- The flow uses an OAuth App, not a GitHub App. An OAuth App's token does
  not expire, so git-board keeps no refresh token.
- The app's client id is public, and a build sets it
  (`make dist GITHUB_CLIENT_ID=…`). A build without it shows no button
  and keeps the pasted token.
- A GitHub Enterprise host and GitLab keep the pasted token. An
  Enterprise host needs its own OAuth App. A GitLab token from the flow
  expires within hours, and needs a refresh token, which is a secret to
  keep.
- `git board forge login <forge>` uses the flow on a terminal. `--paste`
  and a token on stdin keep the pasted way.

## Consequences

Good:

- Most developer machines need no configuration. CI sets one variable.
- No file that git-board writes holds a token.
- An edited `.git-board.toml` cannot send a CI token to another host.
- `git board` opens a working board with no setup, and another site
  cannot read or write it through the user's browser.
- A server shown on a network serves the board, and runs no code for
  that network.

Bad, and accepted:

- What a helper holds is opaque. A keychain entry without a username
  serves every login on that host, and only the forge says whose it is.
- A helper that hangs costs 10 seconds, and counts as no token.
- Anyone who reaches the port reads and writes every issue, and posts to
  forges as the user. `--addr 0.0.0.0` on an untrusted network exposes
  all of it.
- Another local user on a shared machine reaches `127.0.0.1`.
- Writes carry a claim (`board.me`), not an identity.
- A hosted git-board needs authentication, which this ADR does not give.
  A reverse proxy that authenticates is the path.
- The login in the browser needs an OAuth App that the project owns. Its
  owner can see how many people use it, and can revoke it for all.
