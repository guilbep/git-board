# Sync

Sync keeps an issue the same in git-board and on each forge project it is
linked to. It pulls what changed on the forge, and pushes what changed in
git-board. [Forges](forges.md) says what GitHub and GitLab need.

Sync runs with git-board's settings: the forge projects, the posting account
and `publish-private`. They live in git-board's `.git-board.toml`, and
git-board gives them to the core as options. The core doesn't read that
file.

M1 builds one part of pull, in [Pull, the trial's part](#pull-the-trials-part),
and M2 makes its authors people. The rest of this page is planned for M3
(the sync engine) and M4 (the GitHub and GitLab adapters).

## Links

A link joins one issue to one forge issue. It is a `link` operation in the
`sync` entity of that forge's project (see [Sync](data-model.md#sync)), so a
project's entity holds every link to that project.

- An issue has at most one link per project, and a forge issue at most one
  issue. Which link holds, and what happens to a link to an issue not
  present locally, are the sync entity's [state rules](data-model.md#sync-state).
- A pull writes a link when it makes an issue from a forge issue. A push
  writes one when it creates the issue on the forge (M3).
- Sync mirrors: an issue goes to every project sync runs with, except where
  [nothing private published](#nothing-private-published) forbids it. Each
  of those projects' entities then holds a link to it.
- A link is never removed. `unlink` is out of version 1.

## Last seen

For each linked issue and field, the sync entity holds the value the forge
showed when git-board and the forge last agreed on it: after a pull took the
forge's value, or after a push the forge accepted. Each is a `seen`
operation; for a field, the latest in the order counts.

| `field` | `value` |
|---|---|
| `title` | the title, as the forge shows it |
| `body` | the description, as the forge shows it |
| `status` | `open` or `closed` |
| `labels` | the forge's labels, without its [assignee labels](#assignees), one per line |
| `assignees` | the forge's numeric user ids of the issue's assignees, one per line |
| `comment:<forge comment id>` | that comment's text, as the forge shows it |

- **One per line:** the values are unique, sorted by byte order, and joined
  by a line feed (`\n`), with none after the last. An empty set is the
  empty string.
- **Compared as strings,** byte for byte.
- **A field with no `seen`** has the empty string as its last seen value.
- **Which `seen` counts,** and which is ignored, are the sync entity's
  [state rules](data-model.md#sync-state).

Labels `bug` and `ui`, as last seen:

```json
{
  "kind": "seen",
  "author": "4a9d2e7f1b0c6e38d5a2f9b4c7e1d0a36b8f2c95",
  "time": 1792836800,
  "data": {
    "issue": "3f7a9c2e41b6d08a5e1f93c7b2d4a6e8f0c1b3d5",
    "field": "labels",
    "value": "bug\nui"
  }
}
```

From M3, git-board's value is put in the forge's form before it is compared
with last seen, as sync would send it there (see
[Posting for people without a token](#posting-for-people-without-a-token)
and [Mentions and references](#mentions-and-references)). The trial
compares git-board's values as they are.

## Assignees

Assignees are the one field a forge holds as its own. In git-board an
assignee is a label, `assignee:<name>`. On GitHub and GitLab it is one of
the issue's assignees, a user of that forge. Sync never sends an
`assignee:` label to a forge as a label.

- **An assignee label** is the prefix `assignee:`, in any case, followed by
  a name. `Assignee:bob` is one. A bare `assignee:`, with no name, is a
  plain label. The name is kept as it is written.
- **Two fields.** git-board's `labels` value leaves its assignee labels
  out. Its `assignees` value is the forge user ids of the people its
  assignee labels name, found through their accounts on that forge (see
  [People](data-model.md#people)). M3 fixes how a label names a person.
- **A forge's own label that looks like an assignee label,** as an older
  sync left on some issues, is in neither field. Sync leaves it as it is.
- **A name that matches no account on the forge** is refused there (see
  [Refusals](#refusals)).
- **GitLab's free edition takes one assignee per issue.** The others stay
  in git-board, with one warning (see [GitLab](forges.md#gitlab)).

## The four cases

For each field of a linked issue, sync compares git-board's value (G) and
the forge's (F) with last seen (S):

| G = S | F = S | Case | Sync |
|---|---|---|---|
| yes | yes | nothing changed | does nothing |
| yes | no | the forge changed | pulls: git-board takes F, and last seen takes F |
| no | yes | git-board changed | pushes: the forge takes G, and last seen takes G |
| no | no | both changed | the later change wins, pulled or pushed as above |

- **Both changed, to the same value** (G = F): nothing is sent, and last
  seen takes F.
- **Later** compares the `time` of git-board's last operation on that field
  with the time the forge gives its change: its latest event on that field,
  else what [Forges](forges.md) says for that forge. This settles a conflict
  only. The order of a log never reads `time` (see
  [Order](storage.md#order)).
- **The other change stays in the history.** git-board's log keeps its
  operation, and the forge keeps its own history of the issue.
- **Labels and assignees are sets.** Each label and each assignee is
  compared on its own, so changes made on both sides merge. They can't
  conflict: on one label, both changed means both made the same change.
- **A label push is a diff from last seen,** never git-board's whole set.
  So pushing without pulling first keeps whatever labels the forge gained
  since.
- **Comments.** Each comment's text is a field. A comment new on one side
  is added to the other. Adding never conflicts.
- **An issue not on a project yet** is created there from its current
  state: its title and body, then its comments, then its status and labels.
  Issues go oldest first, so the forge's numbers follow git-board's order.
  Its history isn't replayed: that would mean more writes and more
  notifications.
- **A run** reads the issues each forge lists as updated since the last
  run. It can also sync one issue alone, with the same rules.

A pulled operation names its forge project and the event it came from in
its `origin` (see [Origins](data-model.md#origins)).

## Echoes

A forge shows back what sync sent it: the title it set, the comment it
posted. That echo isn't a change made on the forge, and sync doesn't pull
it again.

- **Fields.** After each write the forge accepts, sync appends a `seen` with
  the value sent. The forge then shows last seen, so there is nothing to
  pull.
- **Origins.** An operation pulled from a forge names that forge in its
  `origin`. Sync never sends it back to that forge as a change: the forge
  already has it.
- **Markers.** Each issue and comment sync creates on a forge carries a
  marker, hidden in its text, naming its git-board id. Sync's accounts are
  the posting account and the token owners' it posts as. A forge issue
  or comment that sync's accounts posted with a marker is the git-board
  issue or comment the marker names. Sync links or records it,
  and never pulls it as new. While that issue or comment isn't present
  locally, sync skips it.
- **A marker counts only from sync's accounts.** In anyone else's text it
  is just text, so no stranger can tie a forge issue to a git-board one.
- **A write is never retried blindly:** one the forge made without
  answering would be made twice. A run that stops after a write and before
  its `seen` leaves the marker, and the next run finds it. Sync retries only
  a write the forge turned away for its rate limit (see
  [Forges](forges.md)).

M3 fixes the marker's form. It also records, for each comment sync posts,
which git-board comment it is, so that a comment deleted on the forge isn't
posted again. The data model's `sync` operations don't hold that record
yet.

## Posting for people without a token

- **A token owner acts directly.** A change goes to a forge with its
  author's own token when they have one there, as them.
- **Otherwise the posting account posts it.** A text posted that way starts
  with a header naming its author, the date it was written, and where:
  git-board, or the forge it was pulled from, as a link to the original.
- **The link** tells a mirror from its original, for people and for sync.
- **Titles, statuses, labels and assignees** carry no header. The forge
  shows them as the posting account's changes.
- **Read back,** a text sync posted loses its header and its marker.

[Forges](forges.md#accounts-and-tokens) says where tokens come from.

## Mentions and references

A text names people and issues of the place it was written: the project of
the operation that wrote it, given by its `origin`. A text written in
git-board keeps its mentions and references as they are.

- **Mentions are defused.** A mention, `@login`, in a text sent to a forge
  other than its own, goes as code, `` `@login` ``, so that it notifies no
  one there. Reading the text back restores it. A lone backtick opens no
  code span, so a mention after it is still defused.
- **`#n` becomes the mirrored issue's number.** A reference, `#n`, names
  issue n of the text's own project. Sent to another project, it becomes
  that project's number for the same issue, when the issue is linked there.
  Otherwise it becomes a link to the original issue, when that one is
  linked on its own project. Otherwise it goes as code, `` `#n` ``, so it
  can't point at the wrong issue.
- **Titles are translated too,** as a title may hold a number, like a
  workflow's `(#2518)`. A title shows no link, so a reference sync can't
  translate is `` `#n` ``. A title's mentions stay as written.
- **A title naming its own issue** can be translated only once that issue
  exists on the forge. Sync creates the issue, then sets its title again.

## Refusals

A value a forge refuses stays in git-board, with one warning naming the
value and why.

- **Refused** means for good: the forge answers that the account may not
  do it, or leaves the value out, as GitHub does with an assignee who has
  no access.
- **The rest goes through.** Other values of the same issue are still sent.
- **Last seen keeps the forge's value.** The refused value is still
  git-board's change.
- **Once per value.** Sync records the refusal, warns once, and doesn't
  send that value again. A later change of that field is tried. M3 adds
  this record to the sync entity.
- **A rate limit isn't a refusal:** sync waits, and tries again (see
  [Forges](forges.md)).

## Deletions

Deletions are never propagated, either way.

- **An issue or a comment deleted on a forge** stays in git-board. A linked
  issue gone from the forge stays linked. Sync writes nothing to it, and
  doesn't create it there again.
- **git-board deletes nothing on a forge.** It has no operation that
  deletes.
- **"Not found" is checked twice.** Sync takes a forge issue as deleted only
  when the posting account's token gets "not found" too. A forge gives
  the same answer when a token lacks access to a private project.

## Nothing private published

- **Confidential issues are skipped.** Sync doesn't list them, pull them or
  push to them. Reading one on its own is an error. An issue that turns
  confidential once mirrored keeps its mirror, which then no longer syncs.
- **A public project gets nothing pulled from one that isn't,** unless
  `publish-private` is set. An operation whose `origin` names a project
  that isn't public is never sent to a public one. An issue whose `create`
  came from such a project isn't created on a public one, so nothing else
  of it goes either.
- **Each run reads the visibility** of every project it syncs with. A
  project whose visibility it can't read, and a project it doesn't sync
  with, count as not public.

## Pull, the trial's part

M1's trial builds this part of pull, and nothing else of sync. It reads a
forge's issues as they are now, without their events, and writes into
git-board only. It covers the title, the body, the status, the labels and
the comments. The trial's forge has no assignees.

### What the forge gives

For each issue: its id, number and URL, its author (a numeric id and a
login), its creation time, its title, body and status, its labels, and its
comments. A comment has an id, an author, a time and a text.

- The status is `open` or `closed`. Any other value is an error: the pull
  stops there, before writing anything for that issue.
- The labels are the issue's, from which the pull takes its `labels` value:
  without [assignee labels](#assignees), unique and sorted.

### Steps

1. Load the forge project's sync entity, whose id is the SHA-1 of the
   forge's key. If there is none, its state is empty.
2. Take the forge's issues in the order the forge gives them. For each one:
    - not linked, as no effective link has its id as `forge_id`, by the
      [state rules](data-model.md#sync-state): [a new issue](#a-new-issue).
      An ignored link doesn't count. The first link for a `forge_id` always
      stays effective, so a pulled issue stays linked;
    - linked to an issue present locally: [a linked issue](#a-linked-issue);
    - linked to an issue not present locally: skip it.
3. Return the counts: `Created`, the new issues; `Updated`, the linked
   issues for which the pull wrote anything; `Unchanged`, every other one,
   skipped ones included.

For each forge issue, the pull writes the persons it needs first (see
[Authors](#authors)), then the issue's operations, in one pack, then the
sync entity's, in one pack. It writes no empty pack. So with nothing to
write, it creates no sync entity.

### A new issue

The issue is created with these operations, in this order:

1. `create`, with the forge's title and body;
2. `set-status` `closed`, if the forge issue is closed;
3. `change-labels`, adding the `labels` value, if it isn't empty;
4. a `comment` for each forge comment, in the forge's order, with its text.

Then the sync entity gets, creating it with its id if it doesn't exist:

1. a `link`: the new issue's id, and the forge issue's id, number and URL;
2. a `seen` of the forge's value for `title`, `body`, `status` and
   `labels`, in that order;
3. a `seen` of `comment:<forge comment id>` for each comment, in the
   forge's order, with its text.

### A linked issue

The pull loads the issue, and compares each field's forge value (F) and
git-board's (G) with last seen (S):

| | Writes |
|---|---|
| F = S | nothing |
| F ≠ S, G = S | the issue takes F; then last seen takes F |
| F ≠ S, G = F | last seen takes F |
| F ≠ S, G ≠ S, G ≠ F | nothing |

Fields are compared in this order, and their operations written in it:

- **title:** `set-title`;
- **body:** `edit-text`, whose target is the `create` operation,
  `<issue id>:0`;
- **status:** `set-status`;
- **labels:** G is git-board's `labels` value, its labels without its
  assignee labels. Each label is compared on its own, so one
  `change-labels` holds them all. It adds the labels in F and not in S
  that G hasn't, and removes the labels in S and not in F that G has. It is
  written if either list isn't empty. Last seen then takes F, whenever
  F ≠ S;
- **each forge comment,** in the forge's order. Its copy in git-board is the
  issue's first comment whose `origin` is this forge's key and
  `comment:<forge comment id>`.
    - No copy: the comment is new. The pull appends a `comment` with its
      text, and a `seen` of it.
    - A copy: its text is compared like the fields above, and written with
      an `edit-text` whose target is the copy's id.

A field the issue changed and the forge didn't (G ≠ S, F = S) is left for
M3 to push. A field both changed to different values is left as it is,
last seen included, for M3's later-change rule to settle. Labels are the
exception: they merge per label, and so can't conflict.

The title, last seen as `Search ignores closed issues`:

| git-board's (G) | the forge's (F) | Writes |
|---|---|---|
| `Search ignores closed issues` | `Search ignores closed issues` | nothing |
| `Search ignores closed issues` | `Search skips closed issues` | `set-title`, then a `seen` |
| `Search hides closed issues` | `Search ignores closed issues` | nothing |
| `Search hides closed issues` | `Search skips closed issues` | nothing |
| `Search skips closed issues` | `Search skips closed issues` | a `seen` |

Labels last seen as `bug`, with `ui` added in git-board and `docs` on the
forge: the pull adds `docs`, and last seen becomes `bug` and `docs`. `ui`
waits for M3's push.

### The operations' fields

| Operation | `author` | `time` | `origin` `event` |
|---|---|---|---|
| `create` | the issue's author | the issue's creation | `issue:<forge issue id>` |
| the issue's other operations | the issue's author | the pull's | `issue:<forge issue id>` |
| `comment` | the comment's author | the comment's | `comment:<forge comment id>` |
| `edit-text` of a comment | the comment's author | the pull's | `comment:<forge comment id>` |
| `link` and `seen` | the issue's author | the pull's | none |

- An author is a person's id: the person holding the forge account, as
  [Authors](#authors) says.
- An `origin`'s `forge` is the forge's key.
- The trial reads no events, so it can't tell who changed a field on the
  forge, or when. So its operations carry the pull's time, and the issue's
  author.
- **Why `link` and `seen` take the issue's author.** They record what the
  pull found, so only their time matters, and it is the pull's. They still
  need an author. The pull doesn't read `board.me`, which CI doesn't have,
  so they take the pulled issue's person.

### Authors

The forge gives an issue's or a comment's author as a numeric id and a
login. With the forge's server, like `github.com`, they name an account:
its `id` is the numeric id, as a string (see [People](data-model.md#people)).
The pull loads every person once, then, for each author:

- **A person holds the account:** the author is that person, or, when two
  hold it, its [owner](data-model.md#account-owner).
- **Its login changed on the forge:** the pull appends an `add-account`
  with the new login to that person, authored by the person itself. The
  person stays the same.
- **No person holds it:** the pull creates one, in two writes. First a
  `create` with the login as the name, an empty email and an empty author.
  Then an `add-account` of the account, authored by the new person, as only
  its first pack gives its id. Later authors in the same pull find it.

### What follows

- **Pulling twice in a row writes nothing.** After a pull, every field has
  F = S, or both changed and the pull writes nothing.
- **A change made in git-board is kept.** Where G ≠ S, the pull writes
  nothing to the issue.
- **A login renamed on the forge** keeps its person, whose login changes.
- **A pull that stops between a new person's two writes** leaves a person
  with no account. The next pull creates another one, and the first stays
  unused.
- **A pull that stops between an issue's write and its sync entity's**
  leaves a linked issue's field at G = F ≠ S. The next pull then records
  the `seen`. For a new issue, it leaves the issue without its link, and
  the next pull creates the issue again. M3 finds such an issue by its
  `create` operation's `origin`, and links it instead.
