# `.git-board.toml`

## Overview

The file is optional. Without it, every `ns:value` label is a field and git-board works on any git-bug repository. The file adds what labels can't say: the order of a field's values, single or multiple values, colors, sprint dates, saved views, the GitHub Project to sync with, and where attachments go.

It lives at the root of the repository. Edit it in **Settings → Project configuration**: it is checked as you type and applies as soon as you save. Editing it by hand works too; restart `git-board-server` after.

| Table | What it sets |
| --- | --- |
| `[fields.<name>]` | a field's type, values, order, single or multiple: together, the project's ontology |
| `[fields.<name>.colors]` | the color of each value |
| `[views.<name>]` | a saved view: layout, filter, grouping, sort |
| `[sprints]`, `[[sprints.list]]` | sprint length and dates, for `sprint:@current` |
| `[github-project]` | the GitHub Project to sync fields with |
| `[uploads]` | where attached files go |
| `[branches]` | the branch an issue suggests: its base and its name |
| `[repos]` | the GitHub repositories `git board lint --create-labels` writes to |

## Fields

A field is a label namespace: `status:ready` is the value `ready` of the field `status`. Every namespace found on the issues is already a field; declare one here to give it an order, a cardinality or colors.

```toml
[fields.status]
single = true
options = ["backlog", "ready", "in-progress", "in-review", "validated"]

[fields.est]
single = true
numeric = true
options = ["0.5", "1", "2", "4", "8"]

[fields.assignee]
single = false
```

| Key | Type | Default | What it does |
| --- | --- | --- | --- |
| `options` | list of strings | none | The values, in workflow order: board columns, sort order, and the next status for `git board move <id> --next`; the last one is what `git board close` sets. A value outside the list still works; `git board lint` reports it. |
| `single` | true / false | false | One value at a time: setting a new one replaces the old. Multiple values (`false`) suit assignees or modules. |
| `numeric` | true / false | false | Sorts as numbers. A field named `est` shows its values in hours (`3h`). |
| `project` | inline table | none | The GitHub Project's option name for a value, when it isn't the value: `project = { comite = "Comité Spécialisé" }`. |
| `type` | one of the types below | guessed | What the values are; see Types. |
| `unit` | string | none (`h` for `est`) | Written after a number field's values: `unit = "pt"`. |

## Types

The fields and their types are the project's **ontology**: the kinds of things an issue can say about itself, and how they relate. An assignee is a person, a sprint is a span of dates, a parent is another issue. The type decides how a field sorts, filters, shows and is edited; its values stay labels (`due:2026-10-30`, `parent:#12`), so they travel to GitHub and GitLab like any label.

| Type | For | What it gives | Guessed from |
| --- | --- | --- | --- |
| `select` | status, priority, module | ordered values (`options`), colors, board columns | `options`; any field nothing else fits |
| `iteration` | sprint | the dates of `[[sprints.list]]`, `@current`, sorting by Dates, today's sprint tagged | the names `sprint`, `iteration` |
| `person` | assignee, reviewer | logins, `@me`, several at once | the names `assignee`, `assignees`, `reviewer`, `owner` |
| `number` | estimate, points | numeric sorting, a `unit` after the values | `numeric = true` |
| `date` | due date, deadline | a date picker, red once past, amber within a week, "3 days late" | the names `due`, `deadline`; values that are all `YYYY-MM-DD` |
| `issue` | parent, blocked-by | a link to the other issue, and on it the reverse: Sub-issues, Blocks | the names `parent`, `blocked-by`, `blocks`, `depends-on`, `duplicate-of`; values that are all `#12` or git-bug ids |
| `text` | anything free | a plain text box | only when set |

Set a type when the guess is wrong, or for a field no issue uses yet:

```toml
[fields.due]
type = "date"

[fields.points]
type = "number"
unit = "pt"

[fields.parent]
type = "issue"
```

An issue field names the other issue by its forge number, `#12`, or by its git-bug id, at least its first 7 characters.

## Colors

Every field value already has a color, picked from words in it: done, validated or closed are green; review or qa purple; validation pink; in-progress, doing or wip blue; ready or feat teal; waiting, comite or p2 amber; test, p1 or high orange; blocked, bug, fix or p0 red; backlog, todo, p3 or low grey. Other values take blue, purple, teal and pink in turn. Numbers and assignees stay plain.

Pick a color yourself for any value:

```toml
[fields.status.colors]
"in-review" = "purple"
"tests-metier" = "orange"
validated = "green"
```

The colors are grey, blue, purple, amber (or yellow), orange, green, red, teal and pink. Another name stops the file from loading, and the editor says which line. Quote a value that has spaces or other punctuation than `-` and `_`.

## Views

A view is a saved page: it appears in the header, in alphabetical order, and its address can be bookmarked.

You rarely need to write one by hand. On the board, change the search, the filters, the grouping or the sort, and a bar offers **Save view** (into this view, for everyone) or **Save as new view…**; **Discard** goes back to the saved one. Only the view's table changes in the file, comments included.

```toml
[views.current-sprint]
layout = "board"
filter = "sprint:@current is:open"
group = "status"

[views.backlog]
layout = "table"
filter = "is:open no:sprint"
sort = "prio asc"

[views.roadmap]
layout = "roadmap"
filter = "is:open"
group = "status"
```

| Key | Type | Default | What it does |
| --- | --- | --- | --- |
| `layout` | `"board"`, `"table"` or `"roadmap"` | `"table"` | A board has a column per value of `group`, and cards move between columns by drag and drop. A table has a row per issue. A roadmap puts each issue on a time line: a bar over its sprint's dates, a diamond on its date fields (`due`), today marked; issues with neither are listed below it, unscheduled. |
| `filter` | string | `is:open` | Which issues show; see Filters. |
| `group` | field name | none | The board's columns, or the table's sections: `status`, `sprint`, any field. |
| `sort` | `"<field> asc"` or `"<field> desc"` | none | Order within a column or section: by the field's `options` order, numerically for a numeric field, else alphabetically. |
| `columns` | list of strings | the default ones | A table's columns, in order: `id`, `title`, `is` (open or closed), any field, `links` (the issue on each forge), `other` (labels without a namespace). Without it, the table shows id, forge links, title, state, every field and other, so a new field gets its column. Set from the table's **Columns** button and Save view. |
| `lanes` | list of values | every value | A board's columns: which values of `group` it shows, in this order; `"(none)"` for the issues without one. The other issues are left out, and the board says how many. Set from the board's **Lanes** button. |
| `limits` | inline table | none | At most this many cards in a lane, work in progress: `limits = { in-progress = 5 }`. The lane shows "4 / 5", amber at the limit, red over it. |

### Boards per role

The same issues, a board for each way of working: each view keeps the
lanes its people move cards between.

```toml
[views.dev]
layout = "board"
group = "status"
lanes = ["ready", "in-progress", "in-review", "ready-for-validation"]
limits = { in-progress = 5 }

[views.pm]
layout = "board"
group = "status"
lanes = ["(none)", "backlog", "comite", "ready", "qa"]

[views.qa]
layout = "board"
group = "status"
lanes = ["comite", "ready", "qa", "tests-metier", "validated"]
```

On a board, Cmd- or Ctrl-click picks several cards (Shift-click a range
in a lane, or the box on a card's corner), and dragging one of them moves
them all; a lane's ⋯ menu moves every card it holds.

## Filters

A view's `filter` and the page's search box take the same syntax. Conditions combine with AND, as in GitHub's search; a leading `-` negates any of them.

| Syntax | Matches |
| --- | --- |
| `status:ready` | issues with the label `status:ready` |
| `-status:ready` | issues without it |
| `status:ready,qa` | either value, as in GitHub's search; works with `is:`, `label:` and `author:` too |
| `sprint:@current`, `@next`, `@previous` | the sprint of today, the next or the previous one, from `[[sprints.list]]` |
| `assignee:@me` | you: your git-bug identity's GitHub or GitLab login |
| `is:open`, `is:closed` | open or closed issues |
| `title:*spec*` | a title matching the pattern |
| `author:guilbep`, `author:@me` | who opened the issue |
| `label:bug` | a plain label, or a full one: `label:status:ready` |
| `no:assignee` | no value for that field; `no:label`: no plain label |
| `travel`, `"car sharing"` | words in the title, the description or a comment |

## Sprints

Sprints are a field like any other (`sprint:s12`). The list gives them dates, so that `sprint:@current` finds today's sprint.

```toml
[sprints]
length_days = 14

[[sprints.list]]
slug = "s12"
title = "Sprint 12"
start = 2026-10-05

[[sprints.list]]
slug = "s13"
title = "Sprint 13"
start = 2026-10-19
```

| Key | Type | What it does |
| --- | --- | --- |
| `length_days` | number | The length of the last sprint, whose end has no next start. Also what **+ Sprint** adds to the last start. |
| `start_weekday` | string | Read, not used yet. |
| `slug` | string | The label's value: `sprint:s12`. |
| `title` | string | The name shown. |
| `start` | date, `YYYY-MM-DD`, no quotes | First day. A sprint lasts until the next one starts. |

**+ Sprint** adds the next one after the last: its start plus `length_days`, its number plus one.

The `sprint` field follows these dates: in the filters (sorted by **Dates**, today's sprint tagged *current*), on a board grouped by sprint, in a table's sections and when sorting. In an issue, the sprint is picked from the list. Give `[fields.sprint]` its own `options` to choose another order.

## GitHub Project

The GitHub Project whose fields (status, priority, sprint…) `git board bridge pull` and `sync` turn into labels and back.

```toml
[github-project]
owner = "guilbep"
number = 1
write = true
```

| Key | Type | Default | What it does |
| --- | --- | --- | --- |
| `owner` | string | none | The user or organization owning the Project. |
| `number` | number | none | The Project's number, as in its address. |
| `write` | true / false | false | Lets a sync write git-board's changes to the Project. Off, the Project is only read. |

The token needs the `project` scope: `gh auth refresh -s project`.

## Attachments

Files attached to comments go to an S3 bucket that a CDN serves; the comment gets a link to them. The keys stay out of this file: set `ACCESS_KEY_ID`, `ACCESS_KEY_SECRET` and `BUCKET_NAME` where `git-board-server` runs. Without them, attaching is off.

```toml
[uploads]
prefix = "git-board/co2-calculator"
```

| Key | Default | What it does |
| --- | --- | --- |
| `endpoint` | `https://s3.epfl.ch` | The S3 service. |
| `public_url` | `https://enacit4r-cdn.epfl.ch` | Where the bucket is served. |
| `prefix` | `git-board/<repository>` | The folder in the bucket. |
| `region` | `us-east-1` | The S3 region. |

Files are public to anyone with the link, even for a private repository, and stay cached on the CDN for up to a year after a delete.

## Branches

An issue's **Development** section lists the branches whose name names it, and **Create branch** gives the command creating its branch (`git board branch <id>` prints the same). git-board creates and pushes nothing.

```toml
[branches]
base = "dev"
name = "{kind}/{number}-{slug}"
kinds = { bug = "fix", feat = "feat", perf = "perf", task = "chore" }
```

| Key | Default | What it does |
| --- | --- | --- |
| `base` | the remote's default branch, else `main`, `master` or `trunk` | The branch a new branch starts from (`origin/<base>`, or `<base>` without a remote), and the one "merged" and ahead/behind compare with. |
| `name` | `"{number}-{slug}"` (GitHub's) | The branch's name. `{number}`: the issue's number on its first forge, else its git-bug id's first 7 characters. `{kind}`: its kind, through `kinds`; without one, `{kind}/` is dropped. `{slug}`: its title in lower-case words, without a leading `[BUG](Scope)` or a trailing `(#1234)`, cut at about 50 characters. |
| `kinds` | none | An issue's kind (its `kind:` field, or a plain label listed here) to the `{kind}` of its branch. |

`[BUG](Equipment) Other equipment shows the wrong example (#2518)`, labeled `bug`, gives `fix/2518-other-equipment-shows-the-wrong-example`. What git doesn't take in a branch name becomes `-`.

A branch names an issue when its first number is the issue's number on a forge (`fix/2518-…`, `2518-…`), or when a word in it starts the issue's git-bug id (7 characters or more). Local branches and the remotes' (as last fetched) are listed.

## Repos

```toml
[repos]
list = ["EPFL-ENAC/co2-calculator"]
```

The GitHub repositories where `git board lint --create-labels` creates the labels the fields imply. Nothing else reads it.
