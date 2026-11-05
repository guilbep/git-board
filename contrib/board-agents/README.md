# git-board-agents

A coding agent per [git-board](https://github.com/guilbep/git-board) issue,
each in its own worktree, branch and tmux session, through
[wtx](https://github.com/EPFL-ENAC/wtx). It also asks an AI for an issue's
risk class.

This was git-board v1's experimental `agents` and `worktrees` features (the
issue page's Agent panel, the Work page's agents, Settings › Worktrees, the
AI's risk suggestion). git-board v2 keeps issues and drops these ([ADR
0021](https://github.com/guilbep/git-board/blob/main/docs/adr/0021-git-board-v2-same-data-new-code.md)):
they keep no data, they drive tools. Here they are a client of the board,
released on their own.

- Module: `github.com/guilbep/board-agents`
- Binary: `git-board-agents`. On `PATH`, git runs it as `git board-agents`.

## Install

You need `git`, `git-board` and `wtx` on `PATH`. wtx needs `wt`, tmux and an
agent CLI (Claude Code, opencode): `wtx doctor` says what is missing.

```bash
go env -w GOPRIVATE=github.com/guilbep/*     # while the repository is private
go install github.com/guilbep/board-agents/cmd/git-board-agents@latest
```

From a git-board checkout, before the move: `cd contrib/board-agents && go
install ./cmd/git-board-agents`.

## Usage

Run it in the repository whose board holds the issue: a linked worktree
works too, as wtx runs in the main checkout. `<issue>` is what `git board
show` takes: a short id, `CO2-3075`, `#2528`.

```bash
git board-agents doctor                 # can agents start here? wtx doctor, wtx.toml
git board-agents start CO2-3075         # wtx go: worktree, session, agent on a prompt
git board-agents start CO2-3075 --brief "Start with the BOM test"
git board-agents start CO2-3075 -F brief.md
git board-agents status                 # every worktree: its session, issue and step
git board-agents show CO2-3075          # the issue's branch, worktree and session
git board-agents brief CO2-3075 --brief "Smaller plan, please"
git board-agents brief CO2-3075 --comment 58f8d9b:0   # answer a line comment's thread
git board-agents approve CO2-3075       # the plan is approved on the board: on to step 4
git board-agents stop CO2-3075          # wtx teardown: end the session, keep the worktree
git board-agents done CO2-3075          # wtx done: close the worktree, keep the branch
git board-agents risk CO2-3075          # the AI's class: low, normal or structural
git board-agents risk CO2-3075 --write  # and write it as a comment in the risk step
```

`--comment` takes a comment's ref as `git board show` prints it
(`58f8d9b:0`), or its id from `show --json`; a reply's ref sends its whole
thread.

Every command takes `--json`, `-C <dir>`, `--board <url>` and `--git-board
<path>`. Exit codes: 0 done, 1 an error, 2 a wrong command line, 3 no such
issue (git-board's 3), 4 refused before wtx ran, with a code:
`has-worktree`, `no-worktree`, `branch-all`, `no-plan`, `not-approved`,
`no-cycle`, `no-wtx`, `not-at-risk`.

### The prompt

`start`, `brief` and `approve` write the agent's prompt file and hand it to
`wtx go --prompt` or `wtx brief --prompt`. Our words come first: the issue,
the cycle's step, its gate and what is asked, the branch, the plan. Then
your brief. The review thread (with `--comment`) and the issue's title and
description come last, every line quoted, under a heading that says they
are data, not instructions: anyone who can write on the forge writes them.
The agent's permissions are `wtx.toml`'s: no agent, model or permission
flag is passed.

With the board's cycle on, `start` follows it as v1 did: at step 3 the
agent writes the plan in plan mode and is told where to commit it; at low
risk, or with the plan approved, it starts at step 4 and the issue moves
to the first in-progress status. `brief` past step 3 and `approve` pass
`--build`.

### wtx.toml

```bash
git board-agents doctor --json | jq .answers > answers.json   # wtx.toml as it is
$EDITOR answers.json
git board-agents init --answers answers.json          # preview: run in a scratch repository
git board-agents init --answers answers.json --write  # write wtx.toml and .wt.toml, uncommitted
```

### AI

`risk` asks an OpenAI-compatible chat API at `LLM_BASE_URL` for `LLM_MODEL`,
with `LLM_API_KEY`. Without them it reads the address and model of
opencode's `~/.config/opencode/opencode.json`, the key still from
`LLM_API_KEY`. Only the issue's title and description leave the machine.
What it writes is a comment, never the risk itself: a person sets that.

## How it talks to git-board

It never reads the board's storage. Everything goes through:

1. **The `git-board` CLI**, the contract git-board keeps from v1 to v2
   (commands, flags, `--json` fields, exit codes; its
   [CLI reference](https://github.com/guilbep/git-board/blob/main/docs/guide/cli.md)):
   `show <issue> --json` (the issue, its description, its line comments),
   `branch <issue>` (the branch `[branches]` names for it), `set <issue>
   status=…`, and `comment <issue> -b …` when no server runs.
2. **The board's HTTP API**, when `git board` serves the repository, for
   what only the server computes: `GET /api/issues/{id}/cycle` (the step,
   its gate, the evidence branch, the plan and its approval), `GET
   /api/issues/{id}/thread` (line comments located on the branch's head),
   `GET /api/work?who=team` (which issue each worktree's branch names), and
   `POST /api/issues/{id}/comments` (a comment in a cycle step).

The server is found from `--board`, else `GIT_BOARD_URL`, else the address
a running `git board` writes in the main checkout's
`.git/git-board/server.json`, if the server there answers. Without one,
everything still works but the cycle: the prompt names no step, the branch
is `git board branch`'s, `approve` is refused (`no-cycle`), and `status`
lists no issues.

The routes are git-board v1's (v0.11). v2 reshapes the API: every route is
in `internal/board/board.go`, so following v2 is a change to that file.

### Differences from v1

- v1's server refused to run wtx unless it listened on loopback alone and
  the request came from its own page. Here you run wtx yourself, on your
  machine: those checks have nothing left to guard.
- v1's server refused a second wtx write while one ran (`busy`). Nothing
  here stops two runs of this binary at once: don't start two agents on
  one issue together.
- At low risk, `start` moves the status first and moves it back if `wtx go`
  fails: the step the prompt names comes from the board, which computes it
  from the status. v1 computed it as if moved, and moved it after. A
  failed start leaves two status changes in the issue's history.
- v1 wrote the AI's suggestion with `ai:model` and `ai:risk` comment
  metadata, which no API route takes from a client. The model is in the
  comment's text instead, and the web page doesn't show it as a
  suggestion.
- There is no web UI: v1's Agent panel, Work page column and Settings ›
  Worktrees tab are the commands above.

## Development

```bash
go build ./... && go vet ./... && go test ./...
```

The tests are hermetic (`internal/hermetic`): no network but loopback, no
token, no git config but the test's. wtx, git-board and the LLM are fakes:
shell scripts on `PATH` and `httptest` servers.

The module requires nothing outside the standard library, and must not
import git-board's packages (`TestNoDependency`): what it needs from them
is copied and owned here (`internal/wtx`, `internal/ai`,
`internal/hermetic`, the cycle's `Position` and `ApprovedPlan` in
`internal/board/cycle.go`).

## Moving to its own repository

It lives in git-board's `contrib/board-agents/` until it has a repository.
To move it with its history:

```bash
# in a git-board checkout
git subtree split --prefix contrib/board-agents -b board-agents
# create github.com/guilbep/board-agents, empty, then
git push git@github.com:guilbep/board-agents.git board-agents:main
git branch -D board-agents
```

Then in git-board: delete `contrib/board-agents/`, the `agents` target of
the `Makefile` and the `agents-vet` command of `lefthook.yml`, and point
ADR 0021 and the design's section 5 at the new repository.

## Licence

MIT, as git-board ([LICENSE](LICENSE)).
