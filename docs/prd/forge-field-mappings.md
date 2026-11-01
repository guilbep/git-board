# Where fields live on each forge: PRD

Nov 1, 2026 · @Pierre Guilbert

Each git-board field gets a **per-forge mapping** that says where it is written and read on GitHub or GitLab: a label, a scoped label, a GitHub Project field, native assignees, a milestone, the issue's state. The field's type picks a sensible default, so a team configures only what differs.

Design mockup: `Settings › Forges › gh › Fields` — a per-forge Fields table (Field · Lives in · Details · Values · State), a migration preview section, and a "Written to .git-board.toml" panel.

## Problem

Inside git-board, every field value is a namespaced label (`status:ready`, `due:2026-10-30`). How those values reach a forge is decided in code, so a team can't choose that `status` is a GitHub Project column, or that `assignee` uses GitHub's own assignees.

The Labels page of `guilbep/git-board` shows what this costs:

| What the page shows | What it means |
| --- | --- |
| `assignee:guilbep` as a label on 21 issues | A person field written as a label instead of to GitHub's assignees |
| `sprint:13`, `roles:1a`, `risk:normal`, `risk:structural` listed as outside the project | Namespaces on issues that aren't treated as fields, although the reference says every namespace found is one |
| `moscow:could` next to declared `moscow:couldhave` | An old value with no way to fold it into the new one |
| `status:ready-for-validation` outside the project | A retired status still on an issue |
| "Color differs, No description" on almost every declared value | Values exist on GitHub but were never brought in line |

## Goals

1. Where a field lives on a forge is set in `.git-board.toml`, never in code.
2. A field's type gives a working default per forge, so a new project syncs with no mapping written.
3. The same field can live in different places on GitHub and GitLab.
4. Each field has exactly one place git-board reads back from on each forge.
5. Old values (`moscow:could`) can be folded into current ones on adoption.
6. Every namespace on the issues is a field, everywhere in the UI, including Settings › Labels.

## Non-goals

- Changing how git-bug stores values: they stay namespaced labels inside git-board.
- Credentials, forge URLs and tokens: per-deployment, environment variables; the mapping is project schema, committed with the code.
- Reading a field from the issue title (fragile); a write-only title prefix may come later.
- Forges other than GitHub and GitLab in v1; the design must not prevent Gitea/Forgejo.

## The model

git-board keeps one set of values per issue, as namespaced labels in git-bug. Each forge has its own mapping that projects those values onto whatever that forge offers. Only the overrides are written down. The same `status:ready` becomes a Project column on GitHub and a scoped label on GitLab; `assignee` needs no config on either.

Three kinds of label, and no fourth:

| Kind | Example | Lives in the config | On the forge |
| --- | --- | --- | --- |
| Field value | `status:ready`, `sprint:13` | `[fields.<name>]`, or found on the issues | Wherever that forge's mapping says |
| Plain label | `needs-design`, `regression` | `[labels] list` | Always a plain label |
| Outside the project | `good first issue`, `wontfix` | nowhere | Left alone; Settings › Labels offers Delete |

A forge's native slot (assignees, milestone, Project field) is not a kind of label; it's one of the places a field can live.

## Defaults by type

Apply when a forge has no mapping for a field. Every default uses a free feature; GitLab cells marked \* need a paid tier and are offered as overrides only.

| Field type | GitHub default | GitLab default | Possible overrides |
| --- | --- | --- | --- |
| select, `single = true` | label `status:ready` | label `status:ready` | GitHub Project single select; GitLab scoped label `status::ready`\*; plain label `ready` |
| select, several values | label `module:api` | label `module:api` | plain label `api` |
| person | native assignees | native assignees | label `reviewer:guilbep` (a second person field) |
| iteration | label `sprint:13` | milestone | GitHub Project iteration; GitHub milestone; GitLab iteration\* |
| number | label `est:2` | label `est:2` | GitHub Project number field; GitLab weight\* |
| date | label `due:2026-10-30` | native due date | GitHub Project date field |
| issue (`parent`) | native sub-issues | label `parent:#12` | GitLab epic\* or linked issue |
| text | GitHub Project text field, else not synced | not synced | label |

Two rules on top:

- **Closing.** Whatever target the status field uses, setting its last option (what `git board close` sets) also closes the issue on the forge; reopening sets the first option. The issue state is a side effect, never the place the status is read from.
- **One value per issue.** When a `single = true` field maps to a target allowing several values (plain labels), git-board removes the old value when it writes a new one. GitLab scoped labels do this themselves.

## Configuration

Fields stay forge-agnostic in `[fields.*]`. Each forge in `[forges.<name>]` gets an optional `[forges.<name>.fields]` table holding only the overrides. `aliases` on a field folds old values into current ones. See the Decisions section for the exact key semantics locked against the code.

### Validation

- A target that doesn't fit the type is refused with the line: `weight` on a select field, `assignees` on a number field.
- A target the forge doesn't offer is refused: `scoped-label` on GitHub, `sub-issues` on GitLab.
- `as = "plain"` is refused when two fields share a value name (the plain label can't be traced back to one field).
- The existing `project` key keeps working, read as `[forges.gh.fields.<name>.values]`; lint suggests moving it.

## Sync rules

**Writing.** A value change writes to each forge's `to` target, then to any `also` targets. For a single-value field the previous value is removed from every target written.

**Reading back.** On each forge, a field is read only from its `to` target. Values found elsewhere (an old label, an `also` mirror) are ignored when reading and reported by lint as stale. Until a migration runs, the old target is read once as a fallback so nothing looks empty.

**Conflicts.** Later change wins, the other kept in the issue's history (the git-bug bridge's existing rule; see Decisions).

**Unknown values.** A value on the forge that isn't in `options` and has no alias still syncs in, as today; lint reports it. A value with an alias is stored under its current name.

**Changing a mapping.** Moving `status` from labels to a Project column moves nothing by itself. Settings › Forges shows a migration preview (how many issues get the new value set, how many old labels removed); the user runs it explicitly (`git board sync --migrate <field>` too). Until then, the old target is read once as a fallback.

## UI

### Settings › Forges › a forge › Fields

One row per field, in field order: Field (name, type) · Lives in (dropdown of the targets this forge offers; the default marked) · Details (only what the target needs) · Values ("3 renamed" / "1 alias") · State (In sync / N issues to migrate / Target missing on the forge). Changing Lives in opens the migration preview before anything is written.

### Field editor (Settings › Fields)

One line under the summary: **On GitHub: Project › Status · On GitLab: scoped label**, each part linking to that forge's row. The per-value details table loses its "GitHub Project name" column (renames live in the forge's mapping); an Aliases row shows "also read as: could → couldhave".

### Settings › Labels

- Every namespaced label on the issues is grouped under its field, declared or not (`sprint:13`, `risk:structural`, `roles:1a` leave "outside the project"), with a "Declare this field" action.
- "Outside the project" keeps only labels with no namespace that aren't in `[labels]`.
- A value with an alias shows "→ couldhave"; Adopt rewrites it on the issues.
- Labels for fields that no longer map to labels ("left over from an old mapping") get a migrate action.

## Acceptance criteria

- [ ] A project with no `[forges.*.fields]` tables syncs exactly as today, apart from the type defaults that change behaviour (assignee, parent) — listed in the release notes.
- [ ] `status = { to = "project", field = "Status" }` on `gh`: a status change in git-board appears in the GitHub Project column; a Project change comes back.
- [ ] With that mapping, an old `status:ready` label on GitHub is ignored when reading, reported by lint.
- [ ] `assignee` with no mapping writes GitHub assignees; after migration, no `assignee:*` labels remain.
- [ ] `aliases = { could = "couldhave" }` makes an issue carrying `moscow:could` show `couldhave`; Adopt rewrites the label on GitHub.
- [ ] `sprint:13`, `roles:1a`, `risk:*` appear under their fields on Settings › Labels, not under "outside the project".
- [ ] Setting the last status option closes the issue on every forge; reopening on the forge sets the first option.
- [ ] A mismatched mapping (`weight` on a select, `scoped-label` on GitHub) stops the file from loading; the editor names the line.
- [ ] Changing a field's target shows the migration preview with counts before anything is written.
- [ ] The existing `project = { … }` key still works; lint suggests the new form.

## Decisions, checked against the code (Nov 1 2026)

- **Why undeclared namespaces show as "outside the project" today** — answered: `ghlabels.Desired` (internal/ghlabels/ghlabels.go) emits only declared fields' options and `[[sprints.list]]` slugs; `ghlabels.Stray` buckets everything else on the repo, including `risk:structural`. The reference's "every namespace found is a field" rule was never implemented. M1 fixes it in `handleLabelList` (internal/server/labels.go): existing labels with a namespace are grouped under their field, declared or not; Stray keeps only namespace-less labels.
- **Where the GitHub Project is configured** — stays in `[github-project]` (internal/config/config.go, `GithubProject`); the Settings "GitHub Project" section stays. A gh field mapping's `project` target uses that Project. The old `[fields.X] project` map is consumed only by `ghbridge/sync.go` (`toLabel`/`toProject`); the new `[forges.gh.fields.<name>.values]` replaces it, and the old key keeps working as its fallback.
- **Project fields are readable for any field** — answered: `gh project item-list --format json` returns every Project field keyed by its name (the `Item` struct already decodes `"est. hours"`); a catch-all decodes the rest, so any field can be a `project` target, not just today's hardcoded seven (`synced` in internal/ghbridge/sync.go).
- **Conflicts** — the ghbridge `Decide` (project/local/base) and the syncer's changed-at comparison already implement "later change wins, the other kept in history". Not new work; the mapping layer only decides *where* each side is read.
- **git-bug stores labels** — confirmed: values are namespaced labels in git-bug; the forge adapters' `ChangeLabels`/`SetAssignees` (core/forge/forge.go) are the write surface; `syncer.Options.Held` (internal/store/forges.go) is where "this label has another home on this forge" already lives — the mapping layer drives it.
- **GitLab tier on gitlab.epfl.ch** — not verifiable from the code; the paid-tier targets (`scoped-label`, `weight`, `iteration`, epic) parse and validate but the UI marks them "needs a paid tier". Defaults stay free. To confirm with EPFL IT before M5 ships in a release.
- **GitHub issue types and sub-issues on a personal repo** — issue types are organization-scoped; sub-issues are repo-level and free. `parent`'s default becomes sub-issues on GitHub; `type` targets wait. `config.Held` already holds issue-naming fields off every forge.
- **Changing default behaviour (assignee → native assignees, parent → sub-issues)** — ships as the type default; the old labels keep working as a read fallback until migration runs (Sync rules), and M4's preview makes the move explicit. Release notes list it.
- **`also` mirrors and title prefix** — deferred until someone asks. The config refuses unknown keys today, so they'd be a forward-compatible addition later; `also` is parsed but unused is *not* done — it's simply not accepted yet.
- **The status/close rule** — the syncer already pushes `SetStatus` from the status field's value (core/syncer/plan.go `status` text field); closing follows the last option wherever the field lives, because the value is the same namespaced label regardless of target.

## Milestones

Each ships on its own.

1. **Labels page grouping** — every namespace under its field, declared or not. No config change; answers the stray question. *(done)*
2. **Mapping config and labels only** — `[forges.<name>.fields]`, `to = "label"` with `as`, `values`, field `aliases`, validation; the old `project` key read as the new form. *(done)*
3. **Native targets on GitHub** — the keystone: the forges' sync holds a label off a forge when its field's home there is not a label (config.Held reads the mappings), and the GitHub Project sync takes its fields from the config — any field can be a `project` target, by the mapping's column and value names (Item keeps the raw values gh lists by column name). Assignees already flow through the syncer's assignee sync once the labels are held; the closing rule holds wherever the status field lives, since `git board close` sets the last option and the issue state together. **Deferred**: `milestone` and `sub-issues` as write targets in the syncer (the forge interface has no SetMilestone/SetSubIssue yet) — the labels stay held off the forge meanwhile, nothing is lost, and the PRD's acceptance criteria don't depend on them. *(core done)*
4. **Migration preview** and `git board sync --migrate`; switch the person and issue defaults. *(done — internal/migrate, GET|POST /api/fields/migrate, sync --migrate; the defaults changed with M3, the old labels read as a fallback until the migration runs)*
5. **GitLab targets** — scoped labels, milestone, due date, and the paid-tier ones the EPFL instance turns out to have. *(config, validation and defaults done in M2/M3; the adapters' write targets deferred like M3's — the config and the UI accept and offer them, marked "needs a paid tier")*
6. **Settings › Forges Fields table** and the line in the field editor. *(done — the forge-fields tab, one row per field with the target dropdown, the details, the renames, the migration preview before save, the TOML aside; the field editor's one line per forge; the per-value table's GitHub Project column gave way to the Aliases column; the Labels page lists a mapping's leftovers with a migrate action)*
