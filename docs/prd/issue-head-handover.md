# Handover: the issue head v2, 2026-10-30

For the session that picks this work up. Read this first, then the two
documents it rests on:

- `docs/design/issue-head/README.md`: the design, its *Decisions* (dated
  rows) and its *Open points*.
- `docs/prd/issue-head-plan.md`: the milestones (W0 to W3, L, E, D) and
  their *Progress*, pass by pass.

Pierre (guilbep) settles the open points and says when anything is
committed. He wants claims checked before they're made, and "I couldn't
find it" said plainly rather than guessed around.

## Where it stands

Nothing is committed, pushed or tagged. The work sits uncommitted in the
main checkout, on `main`: 38 edited files and 49 new ones, this file
included (listed under *What changed*).

Checked on 2026-10-30, after the last change:

- Web: `npm run typecheck` clean; `npm test` 299 of 299.
- The EPFL linter: no problem in a new file. The edited files keep only
  older problems, none on a line this work touched: `gitboard.css`'s 145,
  `app.css`'s hex fallback (line 13), `LabelsSettings.vue`'s `1px` (line
  235).
- Go: `go build ./...`, `go vet ./...` and `gofmt -l` clean; the tests of
  `internal/branches`, `config`, `ghlabels`, `lint` and `cycle` pass. The
  whole `go test ./...` passed after the labels pass in a scratch copy
  with git 2.43 and an `origin` remote (`TestInferGitBoard` needs one).
  The sandbox's git 2.34 lacked `notes --no-separator`, `check-attr
  --source` and `merge-tree --write-tree`, and refused this repo's
  `branch.autoSetupMerge = simple`; on the Mac's git none of that applies.
- Never run: `make ci`, `make web-build`, `make docs`, the app on a real
  board. The screens were checked in a Vite harness with fixtures, outside
  the repo, wide and narrow.

## First, before any commit

1. `git status`, to see the state for yourself.
2. Delete `internal/cycle/cycle_test.go.4667479599407640364`: a stale
   temp file a gofmt left (the sandbox couldn't delete). `cycle_test.go`
   beside it is the right one. Never commit it.
3. Delete `web/node_modules/.cache/gb-src.tgz` and `gb-src2.tgz`: copies
   the sandbox made to move files (ignored by git).
4. `.git-board.toml` has one line this work didn't add, as far as I can
   tell: `columns = [...]` under `[views.mine]`, probably from the board's
   column chooser. Ask Pierre whether it goes in.
5. Put the work on a branch, never on `main` (`CLAUDE.md`: never push
   `main`, never tag; Pierre runs `wtx land <branch>`). His call, two ways:
   - `git stash push -u` in the main checkout, then `git stash pop` in the
     worktree `wtx go <branch>` makes (worktrees share the stash). This
     file is untracked: it moves with the rest.
   - Or `git switch -c <branch>` in the main checkout, commit there, switch
     it back to `main`, then open the branch's worktree.
6. Commit only when Pierre asks. Lefthook's pre-commit runs gofmt, go vet
   and the web typecheck on each commit. Once the work is done, run `make
   ci`, and `make docs`: it's strict, and `docs/design/issue-head/` is on
   the site (only `prd/` and `implementation-plan.md` are left out).
7. `CONTRIBUTING.md` asks for a dated *Status update* in
   `docs/implementation-plan.md` for what lands (D-1's second box). Not
   written yet.

## What changed

Go, branch names and *Create* (W3-2, W3-3; decisions 3 and 4):

- `internal/branches/branches.go`: `{n}`, the branch's place in the issue's
  folder, counted from the branches and the base's last 500 first-parent
  merge subjects, never stored; `Folder`, `Next`, `Valid`, `NameAt`. The
  default name is `{number}/{n}-{slug}`; `default.git-board.toml` proposes
  `{kind}/{number}/{n}-{slug}`.
- `internal/store/code.go`: `MergeSubjects`, `HasBranch`, `CreateBranch`
  (`git branch --no-track`), `PushBranch` (`push -u origin`,
  `GIT_TERMINAL_PROMPT=0`).
- `internal/server/refs.go`: `POST /api/issues/{id}/branches`
  (`handleCreateBranch`) behind `localWrite` (loopback, and the board's own
  page or the dev server's origin). It never switches the checkout; the
  name must be ref-safe, name the issue and be new; a refused push leaves
  the branch local and says so. `server.go` holds the route.
- Tests: `internal/branches/branches_test.go`,
  `internal/server/branches_test.go`, `cmd/git-board/branch_test.go`.

Go, the cycle branched out (W3-5; decision 6): `internal/cycle/cycle.go`
(`Lane`, `PickLane`: the issue's step is its least advanced open branch),
`gather.go` (a branch only in this clone with nothing in it is no lane),
their tests.

The labels list (L-1 to L-3; decision 7): `internal/config/config.go`,
`check.go`, `setview.go` (`[labels] list`, `colors`, `descriptions`);
`internal/ghlabels/ghlabels.go`; `internal/server/labels.go` (`GET
/api/config/labels`); `internal/lint/lint.go` (`label-outside`); their
tests and `internal/server/configfile_test.go`. Web: `labellist.ts`,
`LabelPicker.vue`, `LabelsSettings.vue`, `configsnippets.ts`.

Web, the head itself (W0, W1, W2 and the looks after):

- Pure modules, each with a test listed in `package.json`: `nextstep.ts`,
  `branchstate.ts`, `forgestate.ts`, `people.ts`, `headfields.ts`. Also
  `headswitch.ts` (the three switches below), `fieldwrite.ts`.
- Components: `IssueHead.vue` (the title, the fields line, the branch
  line, Spec and Linked), `StatusStepper.vue`, `BranchLine.vue`,
  `BranchFace.vue`, `BranchDetail.vue`, `ForgeIcons.vue`, `InboxBell.vue`,
  `PeopleButton.vue`, `FieldEditor.vue`, `FieldPanel.vue`, `GbPopover.vue`.
- `css/head.css`, all tokens, scoped by `.is-head-v2`: v1 keeps
  `gitboard.css` as it was.
- Edited: `IssuePanel.vue` (the bar, More, Fit and Fill, Comfortable and
  Compact), `IssueSide.vue` (`only`), `IssueStep.vue` (the lanes),
  `GbIcon.vue` (Feather shapes), `api/board.ts`, `inboxcheck.ts`,
  `css/gitboard.css` (task lists as text; two browser margins removed; a
  loose checklist's boxes), `css/app.css` (imports `head.css`).

Docs: `docs/design/issue-head/` and `docs/prd/issue-head-plan.md` (both
new), `docs/guide/configuration.md` (`{n}`, *Create*, *Labels*), this file.

## The switches, kept per browser

| localStorage key | Value | In the page |
| --- | --- | --- |
| `gb-head` | `v1` | More › *The previous head*; v1's meta line has *The new head* |
| `gb-head-width` | `fill` | The bar's *Fit* and *Fill*, hidden under 30rem |
| `gb-head-density` | `compact` | The bar's *Comfortable* and *Compact*; under 30rem, More › *Compact spacing* |

The key absent means v2, Fit, Comfortable.

## Checking it

```bash
cd web && npm run typecheck && npm test
# from the repo root: the EPFL linter (web/README.md), on what you touch
npx --package=stylelint@16 --package=postcss-html -- stylelint \
  web/src/css/head.css web/src/components/IssueHead.vue \
  --config /Users/guilbert/works/git/github/epfl-design-skill/adherence/.stylelintrc.json
go test ./internal/branches/ ./internal/cycle/ ./internal/config/ \
  ./internal/ghlabels/ ./internal/lint/ ./internal/server/ ./cmd/git-board/
make ci     # once, at the end
make docs   # strict: a broken link fails it
```

In the app: `go run ./cmd/git-board serve` from the repo root (`:8080`)
and `npm run dev` in `web/` (`:9000`); one board process per repo at a
time (git-bug's lock, `CLAUDE.md`). *Create* pushes to `origin`: try it
on a scratch repo, such as the live tests' `guilbep/git-board-test`.

## What's next

From the plan, as its *Progress* stands:

- Not started: W0-2 (the value colors into a token file), W0-8 (✓ ✗ ● ×
  as icons), W0-9 (`make web-lint`), E-* (one file's EPFL fixes each), W2-2
  (the forge states), W3-6 ("New on the forge", open point 8).
- In part:
  - W0-10: `TestNamesInFolders` covers the folder form; still to write,
    `test/2771b-x` not linked and `feat/co2-421/1-x` linked.
  - W2-1: the forge popover hosts today's `IssueSync`;
    `useissuesync.ts` isn't extracted.
  - W2-7: the held flag shows, and the field's editor names the Project's
    value and links to Conflicts; no tooltip with the value, no *Resolve*
    in the popover.
  - W3-1: the side column is gone in v2; retiring v1 and its switch waits
    for Pierre.
  - W3-4: the head uses branch, review, merge and "The gate: GitHub #N";
    the cycle's own texts don't yet.
  - D-1: the guide is written; the status update isn't.
- Open points still open in the README: 2 (the waits block), 8 ("New on
  the forge"), 9 (the arrow without words: a check with the PM), 10 (dark
  mode), 11 (the position in the bar).
- Comfortable or Compact as the default, once Pierre has used both; the
  other may then go.
- co2-calculator: if its `.git-board.toml` sets `[branches] name`, it needs
  `{n}` for the folder numbering. `../co2-calculator` is read-only here.
- Small ones: `gitboard.css`'s `.gb-md ul.gb-tasks` rules have no producer
  (dead). The CSS-only fixes (task lists, the browser margins, a loose
  checklist) have no regression test, which `CONTRIBUTING.md` asks of a
  bug fix: the web tests have no DOM. Say so, or add one.

## Things that bit, so they don't again

- No input takes the focus when a popover opens: Safari's and 1Password's
  autofill jumped in. `GbPopover` focuses its panel unless given `focus`;
  inputs carry `autocomplete="off"`, `data-1p-ignore`,
  `data-lpignore="true"`, `data-form-type="other"`.
- A `<p>` gets the browser's 1em margins wherever nothing resets them: the
  bar's crumb made the bar 62px, not 51. `.gb-md` spaces its blocks with
  `> * + *` and drops the margin under them.
- `--gb-prose` (Fill's prose width) is Fit's width less the comment body's
  padding: change `.gb-comment-body`'s padding and it with it.
- `IssueSide.vue`'s styles are scoped: `head.css` overrides them with one
  class more (`.is-compact .gb-head-extra .gb-ref`).
- A narrow drawer is a container query, not a media query:
  `@container (max-width: 30rem)` on `.is-head-v2.gb-panel`;
  `.gb-wide-only` and `.gb-narrow-only` follow it.
- `node --test` runs only the files listed in `package.json`'s `test`
  script, and a test imports with the `.ts` extension.
- EPFL to the letter (the README's row): tokens, no px or hex; Feather
  icons; borders, a shadow on popovers only; 2px radii; pills only on
  counts; a 2px focus outline 2px off; disabled at opacity 1; fades only;
  sentence case, no emoji.
- No real person's name in mocks or fixtures (`cycle-protocol.md`):
  Person A, Person B.
- git-board runs only commands its code or the project file names, as
  argv; a name from the page is checked (ref-safe, names the issue, new)
  before it reaches one.
