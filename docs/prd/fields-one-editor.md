# Fields: one editor for a field, wherever it lives

Status: implemented 2026-11-02 (go given), not yet committed. Plan: `fields-one-editor-plan.md`.
Mockup: https://claude.ai/artifact/Fpo7pBejtJDvPtce4AjQPY ("Proposal · fields").
Amends `forge-field-mappings.md` (its UI section); the model, the targets,
the defaults and the sync rules there are unchanged.

## Why

Settings has two screens for one thing. **Fields** edits what a field is
(`[fields.<name>]`: type, values in order, one or several per issue).
**Forge fields** edits where it lives on one forge
(`[forges.<forge>.fields.<name>]`: a label, a Project field, sub-issues,
the values' names there). Both write `.git-board.toml`; they are two tables
of the same file, split into two screens by the file's layout. So:

- to change a field you must know which table holds the part you want;
- the field editor already shows "On GitHub: **Label**" and links away to
  change it: the seam shows;
- removing a field leaves its forge mappings behind, written by the other
  screen;
- with two forges, a field's places are on two pages.

The field is the unit people think in. What it is and where it lives on
each forge belong in one editor, with one Save.

## What we know (checked in the code, 2026-11-02)

- Each settings form drafts one section and saves it alone:
  `PUT /api/config/section/{section}` with the file's hash as `base`
  (`internal/server/configsections.go`). The server turns the section's
  body into a function on the file's text (`sectionWriter`), checks the
  result, then writes. Two functions chained on the same text is one write.
- `fields` takes `{set, remove, order}` (`config.SetFieldsText`,
  `MoveFieldText`); `forge-fields` takes `{forge, set, remove}`
  (`config.SetForgeFieldsText`) and first writes the found forges' tables
  when the forges are only found in the remotes (`resolveFound`).
- The field editor (`FieldsSettings.vue`) shows the per-forge line and
  links to `#/settings/forge-fields/<forge>`; the forge table
  (`ForgeFieldsSettings.vue`) has the target dropdown, the Project field,
  the values' renames, the migration preview (`previewFieldMigration`) and
  run (`runFieldMigration`), and the TOML aside.

## Decisions

| Question | Decision |
|---|---|
| Where a field's place on a forge is edited | in the **field editor**: a "Where it lives" block, one row per configured forge: the target (default marked), the details that target needs (the Project field), and "Names on <forge>" for the values' renames |
| The field list | a chip per forge on each row, the field's place there; a non-default place outlined |
| Saving | **one Save** writes both tables, atomically: the `fields` section's body gains `forges: { <forge>: { set, remove } }`, applied in the same write |
| Removing a field | removes its `[forges.*.fields.<name>]` too, in the same write |
| Migration | changing a place shows the counts in the editor before Save; Save writes the file; then a "Move the issues" panel lists each moved field with its counts and a **Run the migration** button. Nothing on a forge changes until it is pressed, as Forge fields did |
| The TOML tab and aside | show the field's table and its forge tables, together |
| Forge fields | **leaves the sidebar**. Sync › <forge> shows a one-line summary ("5 as labels · prio and est in the Project · parent as sub-issues") linking to Fields. Old links (`#/settings/forge-fields/<forge>`) open Fields |
| A newly found forge | nothing to set: every field follows its type until changed; its table is written the first time a place is changed (as today) |

## Behaviour

1. Settings › Fields, a field selected: Type, Values, then **Where it
   lives**: for each forge in `[forges.*]` (or found in the remotes), its
   icon and name, the target dropdown, the details, and a "Names on …"
   disclosure when the target keeps the values' names.
2. Changing a target marks it as an override (blue), shows the migration
   counts under the block ("31 issues carry it, 31 labels come off"), and
   makes the draft dirty.
3. Save: one request. The server applies the fields change, then each
   forge's, checks the file, writes it once, and answers the model. A place
   moved shows "Move the issues to their new place" with the counts and
   Run the migration; Not now leaves it (`git board sync --migrate` later).
4. The field list's chips follow the draft, unsaved changes included.
5. Sync › <forge>: the summary line and "Change it in Fields".

## Out of scope

- New targets or new defaults (`forge-field-mappings.md` owns them).
- Editing several fields' places at once (the old table's strength): the
  list's chips show them; a bulk change is later, if wanted.

## Open questions

1. Keep the per-forge table as a read-only overview under Sync › <forge>
   instead of the one-line summary? This draft says the summary: the
   field list's chips already give the overview.
