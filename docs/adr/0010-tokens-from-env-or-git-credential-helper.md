# ADR 0010: Forge tokens come from the environment, else git's credential helper

- Status: accepted (recorded 2026-11-04, decided earlier)
- Date: 2026-10-24 (`docs/prd/core.md` › The data model, in brief, and
  Answered by guilbep: "Tokens: environment variables, else git's
  credential helper"). Built in M4 and M5, 2026-10-25.
- Spec: `docs/spec/forges.md` › Accounts and tokens; guide
  `docs/guide/forges.md` › Tokens.

## Context

Sync and git-board's own forge calls (images, assignable people, pull
requests, labels) need a token per account and per forge host. git-bug
kept tokens in its own bridge configuration, per identity. Settings live in
the committed `.git-board.toml` (ADR 0011), which anyone can edit in a pull
request. CI has tokens in its environment; developers usually already have
one in git's credential helper (macOS keychain, Git Credential Manager,
libsecret).

## Decision

**A token is read from an environment variable, only for that variable's
own host; otherwise from `git credential fill` for the host, with the
account as `username`. git-board stores none itself.**

- `GITHUB_TOKEN` is for `github.com` only; `GITHUB_ENTERPRISE_TOKEN` for
  `GITHUB_ENTERPRISE_HOST` only; `GITLAB_TOKEN` for `GITLAB_HOST`
  (default `gitlab.com`). `GH_TOKEN` is never read
  (`core/forge/github/token.go:18-29`, `core/forge/gitlab/gitlab.go:27`,
  `:60`). So a pull request that points a forge at another host can't
  send a token there.
- The helper never prompts: `GIT_TERMINAL_PROMPT=0`, empty `GIT_ASKPASS`,
  `GCM_INTERACTIVE=never`, 10 s at most (`core/forge/credential.go:13-35`).
  A helper with nothing for the login is asked once more without one
  (`:41-52`).
- A person's token counts only when the forge says it is theirs (`As`,
  `core/forge/forge.go:97`).
- Settings stores a new token with `git credential approve`
  (`internal/store/tokens.go:60` `StoreToken`). No token is shown back.
- A base URL must be `https://`, except loopback (`docs/spec/forges.md` › Accounts and tokens).
- The core keeps no token, in git or anywhere (`docs/spec/forges.md`).
  No keyring library (`docs/prd/core.md`).

## Alternatives considered

Partly recorded: the PRD rejects a keyring library; the rest is
reconstructed. Scored on: no secret in the repository (35), works in CI
and on laptops with no setup (30), cost (20), one rule for every caller
(15).

| Option | Score | Why not |
| --- | --- | --- |
| **Env var for its own host, else git's credential helper (this decision)** | 85 | |
| A keyring library (`zalando/go-keyring` and the like) | 55 | A dependency per OS, often cgo; a second store beside the one git already uses |
| Tokens in git config or a git-board file | 30 | Plaintext on disk, one `git config --list` away; easy to commit |
| Delegate to `gh` and `glab` | 45 | Two CLIs to install and authenticate; `gh` may hold another or a narrower token (`internal/ghlabels/ghlabels.go:32-34`) |
| git-bug's per-identity bridge tokens | 25 | Left with git-bug (ADR 0003) |

## Consequences

Good:

- Nothing to configure on most developer machines; CI sets one variable.
- No token in any file git-board writes.
- An edited `.git-board.toml` can't exfiltrate a CI token to another host.

Bad, and accepted:

- What a helper holds is opaque: a keychain entry without a username is
  used for any login on that host, and checked only by `As`.
- A helper that hangs costs 10 s and is treated as no token.
- Hosts other than github.com, `GITHUB_ENTERPRISE_HOST` and `GITLAB_HOST`
  can't take a token from the environment.

## Left open

- **Drift: the GitHub Project sync uses `gh`'s own login.**
  `internal/ghbridge/ghbridge.go:139-142` (`FetchItems`) and
  `internal/ghbridge/sync.go:789-790` shell out to `gh`, "assumed already
  authenticated with at least read:project scope", and `CONTRIBUTING.md:18-20`
  says so. That path doesn't follow this rule: its token is whatever `gh`
  holds. The labels moved off `gh` to the forge token
  (`internal/ghlabels/ghlabels.go:32-34`); the Project sync hasn't.
- **Drift:** `README.md` › What else to install lists `gh` for
  `lint --create-labels`, while `CONTRIBUTING.md:18-20` and the code say
  the labels use the forge sync's token.
- **Drift:** `CONTRIBUTING.md:88-93` still describes tokens on git-bug
  identities (`git bug bridge auth add-token`).
- The S3 keys for attachments follow another rule: plain environment
  variables only (ADR 0015).
