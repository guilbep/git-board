# `.git-board.toml`

## Overview

The file is optional. Without it, every `ns:value` label is a field and git-board works on any git-bug repository. The file adds what labels can't say: the order of a field's values, single or multiple values, colors, sprint dates, saved views, the GitHub Project to sync with, and where attachments go.

It lives at the root of the repository. Edit it in **Settings → Project configuration**: it is checked as you type and applies as soon as you save. Editing it by hand works too; restart `git-board-server` after.

| Table | What it sets |
| --- | --- |
| `[fields.<name>]` | a field's values, their order, single or multiple |
| `[fields.<name>.colors]` | the color of each value |
| `[views.<name>]` | a saved view: layout, filter, grouping, sort |
| `[sprints]`, `[[sprints.list]]` | sprint length and dates, for `sprint:@current` |
| `[github-project]` | the GitHub Project to sync fields with |
| `[uploads]` | where attached files go |
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
```

| Key | Type | Default | What it does |
| --- | --- | --- | --- |
| `layout` | `"board"` or `"table"` | `"table"` | A board has a column per value of `group`, and cards move between columns by drag and drop. A table has a row per issue. |
| `filter` | string | `is:open` | Which issues show; see Filters. |
| `group` | field name | none | The board's columns, or the table's sections: `status`, `sprint`, any field. |
| `sort` | `"<field> asc"` or `"<field> desc"` | none | Order within a column or section: by the field's `options` order, numerically for a numeric field, else alphabetically. |

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

## Repos

```toml
[repos]
list = ["EPFL-ENAC/co2-calculator"]
```

The GitHub repositories where `git board lint --create-labels` creates the labels the fields imply. Nothing else reads it.
