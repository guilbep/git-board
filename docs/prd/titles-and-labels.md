# Titles and labels: one truth

A discussion to turn into a plan, and maybe a proposal to the co2 team.
Start a session here with:

```sh
cd ~/works/git/github/git-board-titles && claude
# then: "Read docs/prd/titles-and-labels.md, and let's discuss it."
```

## The problem

co2-calculator's convention (CONTRIBUTING.md, "Issue Convention"):

```
[PREFIX](Scope) Issue description (#2518)
```

- `PREFIX` is BUG, FEAT, PERF, SPECS or TASK, prefilled by the issue
  template, which also sets flat labels (`bug`, `enhancement`, …);
- `Scope` is the module or tool (Equipment, Travel, Results, BackOffice, CI);
- `(#2518)` is appended by a workflow (`append-issue-number.yml`);
- the type label sets the branch prefix (`bug` → `fix/`, `feat` → `feat/`),
  docs/src/contributing/project-board.md#labels.

The title holds three facts that are also data: the kind (a label), the
scope (a module field), the number (the forge link). Two truths drift: the
title can say BUG while the label says feat, and a scope written in a title
can't be filtered reliably.

## The user's position

"It's weird to have two truths. It's only my opinion, but I'll fight it."

- Labels should weigh more than the title convention; `git board lint`
  should say so.
- The forge number in the title was a workaround: git-bug didn't keep
  track of the forge number, so it was really confusing. The PM keeps
  referring to issues by number; it was impossible to work with git-bug
  without it. git-board now shows the forge number (the Forge column,
  `#2518`, `GL #45`).
- A lot of problems and a lot to talk about.

## Facts found

- **Titles go to forges unchanged** (git-bug fork,
  `bridge/core/syncpush.go`, `CreateIssue` and `SetTitle` take
  `snap.Title`). Bodies get their `#refs` translated (`refIndex.translate`)
  but titles don't. So on the GitLab mirror, `(#2518)` would point to
  GitLab's own #2518, a different issue. Being fixed in the main session
  before co2 ever syncs.
- git-board's convention is namespaced labels (`kind:bug`,
  `module:equipment`); co2's are flat (`bug`, `feat`) and feed branch
  prefixes and Project 40's automation (read-only for us).

## Directions to weigh

1. **git-board reads the convention without fighting it**: a declared title
   shape (`[titles] pattern = "[{kind}]({module}) {text} (#{number})"`)
   gives kind and module as fields you can filter and group by; the title
   shows without its prefix and number (chips instead); `git board lint`
   reports a title that disagrees with the labels. Titles are never
   rewritten.
2. **Flat labels as a field's values**: `[fields.kind] labels = ["bug",
   "feat", "perf"]`, so `bug` counts as `kind:bug`.
3. **Change the convention** (the fight): plain titles, labels for kind and
   scope, the number shown by the tools. Needs the team, the PM, the
   template and workflow changes, and Project 40 left untouched.
4. **Issue numbering**, shared with the "issue is the unit of work" session
   (`git-board-work-unit`): a project's own number, independent of forges?

## Constraints

- co2-calculator stays local: no sync, no PRs or issues its automation
  would pick up, the EPFL IP checkbox of its PR template is only for the
  user.
- Live tests only on the scratch forges (guilbep/git-bug-sync-test,
  gitlab.epfl.ch 42400, guilbep/git-board-test, Project #1).
