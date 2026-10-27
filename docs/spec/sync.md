# Sync

Sync keeps an issue the same in git-board and on each forge project it is
linked to. It pulls what changed on the forge, and pushes what changed in
git-board. [Forges](forges.md) says what GitHub and GitLab need.

Sync runs with git-board's settings: the forge projects, the posting account
and `publish-private`. They live in git-board's `.git-board.toml`, and
git-board gives them to the core as options. The core doesn't read that
file. With no forge table there, git-board finds the projects in the git
remotes; a GitLab one's numeric id is read before the run, so its key is
the one a table naming that id gives.

M1 built one part of pull, in [Pull, the trial's part](#pull-the-trials-part),
and M2 made its authors people. M3 builds the rest on a fake forge: the
push, the conflict rules, the records, and [a run](#a-run). M4 builds the
GitHub and GitLab adapters.

## Links

A link joins one issue to one forge issue. It is a `link` operation in the
`sync` entity of that forge's project (see [Sync](data-model.md#sync)), so a
project's entity holds every link to that project.

- An issue has at most one link per project, and a forge issue at most one
  issue. Which link holds, and what happens to a link to an issue not
  present locally, are the sync entity's [state rules](data-model.md#sync-state).
- A pull writes a link when it makes an issue from a forge issue. A push
  writes one when it creates the issue on the forge.
- Sync mirrors: an issue goes to every project sync runs with, except where
  [nothing private published](#nothing-private-published) forbids it. Each
  of those projects' entities then holds a link to it.
- A link is never removed. `unlink` is out of version 1.
- `Links` lists every sync entity's links, by issue, as its state rules
  hold them: git-board shows where an issue is with them. `Records` adds
  each link's last seen values, texts without header and marker, and the
  entity's `posted` comments: git-board shows what waits for a forge with
  them. A link's forge key isn't in it, as an entity's id is a hash of the
  key; its `url` names the forge, and a record names its entity.

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

git-board's value is put in [the forge's form](#the-forges-form) before it
is compared with last seen and with the forge's, as sync would send it
there. The trial compared git-board's values as they are.

### The forge's form

A text goes to a project in that project's form:

1. its mentions and references, translated for that project (see
   [Mentions and references](#mentions-and-references));
2. a header first, when sync posted it there for someone without a token
   (see [Posting for people without a token](#posting-for-people-without-a-token));
3. a quote after the header, for a comment the application gives one
   (`Options.Quote`; git-board's says where a thread comment is anchored):
   one Markdown quote line, `> …`;
4. a marker last, when sync created it there (see [Markers](#markers)).

The parts are joined by a blank line (`\n\n`). The header, the quote and
the marker a text has on a project stay the ones it was created with: sync takes them
from its last seen value there. So the form changes only when the text
does, or a comment's keys (see [Markers](#markers)). A title, a status, labels and assignees have no header and no
marker. A title is translated too.

### What git-board took from a forge

When the last operation that changed a field of an issue came from the
forge it is compared with, its `origin` naming that forge's project, the
field's value is what git-board took from that forge. Sync compares with
that value instead of the `seen`: git-board didn't change it.

- **It is never sent back** to that forge as a change: the forge already
  has it (see [Echoes](#echoes)).
- **It settles two pulls of one comment.** Two clones that pull the same
  forge comment, the forge editing it in between, each write a copy and a
  `seen`. Once they merge, the issue keeps the first copy's text, in the
  issue's order, and `seen` the last value, in the sync entity's order. They
  can disagree. The copy came from the forge, so it counts as last seen: a
  forge edit is pulled, and the stale copy is never pushed over it.
- **Per label and per assignee,** for sets: the operation that last added or
  removed that label.

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
  [People](data-model.md#people)).
- **A label names a person** by a login: the person owning an account on
  the forge's server with that login, compared without case. Failing
  that, by a name: the person of that name with an account there. The id
  is that account's.
- **Pulled,** a forge assignee becomes `assignee:<login>`, with the login
  of their account on that forge. A removed one takes away every assignee
  label naming them.
- **A forge's own label that looks like an assignee label,** as an older
  sync left on some issues, is in neither field. Sync leaves it as it is.
- **A name that matches no account on the forge** is refused there (see
  [Refusals](#refusals)).
- **A forge takes the whole set.** Sync sends the forge's set, plus what
  git-board added and less what it removed, and reads back what the forge
  kept. An assignee it didn't keep is refused.
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
  seen takes F. So does any field where G = F and last seen differs.
- **Later** compares the `time` of git-board's last operation on that field
  with the time the forge gives its change: its latest change of that
  field, else what [Forges](forges.md) says for that forge. At the same
  second, the forge's wins: it is already public, and pulling sends no
  notification. This settles a conflict only. The order of a log never
  reads `time` (see [Order](storage.md#order)).
- **The other change stays in the history.** git-board's log keeps its
  operation, and the forge keeps its own history of the issue.
- **Labels and assignees are sets.** Each label and each assignee is
  compared on its own, so changes made on both sides merge. They can't
  conflict: on one label, both changed means both made the same change.
- **A label push is a diff from last seen,** never git-board's whole set.
  So pushing without pulling first keeps whatever labels the forge gained
  since. Last seen then takes S, plus the labels added and less the labels
  removed, not the forge's set: a label the forge gained is left for the
  next pull to bring in.
- **A held label stays in git-board** on that project (`Options.Held`):
  never sent there, and the project's own copy is neither pulled nor
  removed. git-board holds, on GitHub with a Project, the fields the
  Project holds, and on every forge the fields naming issues (`parent:`,
  `blocked-by:`…) and `milestone:`: each fact has one home there.
- **Comments.** Each comment's text is a field. A comment new on one side
  is added to the other. Adding never conflicts.
- **An issue not on a project yet** is created there from its current
  state: its title and body, then its comments, then its status, labels
  and assignees. Issues go oldest first, so the forge's numbers follow
  git-board's order. Its history isn't replayed: that would mean more
  writes and more notifications.
- **A run** reads the issues each forge lists as updated since the last
  run. It can also sync one issue alone, with the same rules (see
  [A run](#a-run)).

A pulled operation names its forge project and the event it came from in
its `origin` (see [Origins](data-model.md#origins)).

## Conflicts

Users meet two rules, one per kind of conflict:

- **Between clones, by the order.** Two clones that change one field
  offline both keep their operation. Once they merge, the operation later
  in the [order](storage.md#order) wins, whatever their times say. Every
  clone agrees, as the order is the same everywhere.
- **Between git-board and a forge, by time.** When both changed a field
  since they last agreed, the later change wins: the times of git-board's
  operation and of the forge's change, as [The four cases](#the-four-cases)
  says. The clones' winner is the operation compared.

## Push

The push sends git-board's changes to each project. For a project, it takes
the issues present locally, oldest first, by creation time and then id.

1. **Create** each issue that may go to the project and isn't linked there:
   [A new forge issue](#a-new-forge-issue).
2. **Then, for each linked issue** whose git-board value differs from last
   seen in some field, in the forge's form, read the forge issue, compare
   each field, and write: [A linked forge issue](#a-linked-forge-issue).

Creating every issue first lets the texts of the others translate their
references to them in the same run.

Each write goes with its author's token when they have one there, else
with the posting account (see
[Posting for people without a token](#posting-for-people-without-a-token)).
Its author is the author of git-board's operation that last changed the
field. For labels and assignees, sent in one write, it is the author of the
latest of their operations.

After each write the forge accepts, sync records it at once: the link, the
`posted` or the `seen` (see [Records](#records)). A run that stops then
loses nothing.

### A new forge issue

An issue may go to a project unless:

- it is linked there, by any link, effective or not;
- its `create` came from that project: it was pulled from there, and a
  forge issue gone from there isn't created again;
- it is [a duplicate](data-model.md#sync-state);
- [nothing private published](#nothing-private-published) forbids it;
- the project [refused](#refusals) to create it.

Sync creates it with its title and body in the forge's form, the body with
the issue's marker. Then it records the link and the `seen` of what a new
forge issue shows: `title` and `body` as sent, `status` `open`, and empty
`labels` and `assignees`. The issue is then linked, and the rest of it is
sent as for a linked issue: comments, then status, labels and assignees.

### A linked forge issue

The push reads the forge issue from the run's listing, else alone. A forge
issue that is gone, as [Deletions](#deletions) says, or confidential, gets
nothing. Then, in this order:

- **title,** **body:** set, when git-board changed it;
- **comments,** in the issue's order:
    - a comment the forge doesn't have, and that sync never posted there,
      is posted with its marker, then recorded with a `posted` and a
      `seen`. A comment whose `origin` names that project is the forge's,
      and isn't posted back. A comment git-board keeps local, its line
      comments (`review:*` metadata), is never posted: the run's `Local`
      option says which, from a comment's metadata;
    - a comment the forge has, whose text git-board changed, is edited;
- **status:** set, when git-board changed it;
- **labels:** one write adds the labels git-board added since last seen and
  the forge hasn't, and removes the ones it removed that the forge still
  has;
- **assignees:** one write of the whole set, as [Assignees](#assignees)
  says.

A field both sides changed is pushed when git-board's change is the later.

## Records

Besides links and last seen values, the sync entity records what a run
needs to remember (see [Sync](data-model.md#sync)):

- **`posted`**, for each comment sync posted: which git-board comment the
  forge comment is. A comment recorded on a project is never posted there
  again, even once deleted on the forge.
- **`refused`**, for each value a forge refused: the field, the value, and
  the forge's reason (see [Refusals](#refusals)).
- **`run`**, at the end of a run that wrote anything: the time the run
  started. The next run lists the issues updated since the latest `run`
  of the project. A run that wrote nothing records nothing, so a quiet
  repository's sync entity doesn't grow.

## Echoes

A forge shows back what sync sent it: the title it set, the comment it
posted. That echo isn't a change made on the forge, and sync doesn't pull
it again.

- **Fields.** After each write the forge accepts, sync appends a `seen` with
  the value sent. The forge then shows last seen, so there is nothing to
  pull.
- **Origins.** An operation pulled from a forge names that forge in its
  `origin`. Sync never sends it back to that forge as a change: the forge
  already has it (see [What git-board took from a forge](#what-git-board-took-from-a-forge)).
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
  its record leaves the marker, and the next run finds it (see
  [Crash recovery](#crash-recovery)). Sync retries only a write the forge
  turned away for its rate limit (see [Forges](forges.md)).

### Markers

A marker is an HTML comment, which forges don't show, on the text's last
line:

```text
<!-- git-board issue 3f7a9c2e41b6d08a5e1f93c7b2d4a6e8f0c1b3d5 -->
<!-- git-board comment 8c04e5b1d97f2a63c18e0b4d5f6a7928e3b1c0d4:0 -->
```

The first names an issue, in its body. The second names a comment, by its
id. Pulled back, a text sync's accounts posted loses its header, its quote
and its marker.

A comment's marker also carries keys of its metadata, after its id, as
`key=value` pairs separated by spaces; which keys, under which names, is
the application's (`Options.Carry`; git-board's are in
`docs/design/time-flow/protocol.md`):

```text
<!-- git-board comment 9c41e5b1d97f2a63c18e0b4d5f6a7928e3b1c0d4:7 step=1 anchor=body@v1/expected#3 -->
```

A comment posted under a quote has the word `quoted` after its id, before
its keys: pulled back, its text loses the quote line only then, so a text
of its own starting with a quote keeps it.

The keys are rebuilt from the comment's metadata each run, so a metadata
change alone edits the forge comment. A forge edit of a comment sync
posted sets the metadata its marker's keys say (`Options.Restore`); with
the marker deleted, the comment is plain text from then on, and Restore
says which keys that clears. A comment never edited on the forge comes
back by its record, its metadata untouched.

## Posting for people without a token

- **A token owner acts directly.** A change goes to a forge with its
  author's own token when they have one there, as them. Their texts carry
  the marker, and no header.
- **Otherwise the posting account posts it.** A text posted that way starts
  with a header naming its author, the date it was written, and where:
  git-board, or the forge it was pulled from, as a link to the original.
- **The link** tells a mirror from its original, for people and for sync.
- **Titles, statuses, labels and assignees** carry no header. The forge
  shows them as the posting account's changes.
- **Read back,** a text sync posted loses its header and its marker.

The header is one line in italics. Its author is the person's name, its
date the text's creation day in UTC, and the link the original issue on
the forge the text came from:

```text
*From ann, 2026-10-24, in git-board.*
*From ann, 2026-10-24, on [github.com/guilbep/git-board-test#12](https://github.com/guilbep/git-board-test/issues/12).*
```

The header is built from the text's operation, never from the time of the
run, so the same text always gets the same header.

[Forges](forges.md#accounts-and-tokens) says where tokens come from.

## Mentions and references

A text names people and issues of the place it was written: the project of
the operation that wrote it, given by its `origin`. A text written in
git-board keeps its mentions and references as they are, and so does a text
sent back to its own project.

- **Mentions are defused.** A mention, `@login`, in a text sent to a forge
  other than its own, goes as code, `` `@login` ``, so that it notifies no
  one there. A lone backtick opens no code span, so a mention after it is
  still defused.
- **`#n` becomes the mirrored issue's number.** A reference, `#n`, names
  issue n of the text's own project. Sent to another project, it becomes
  that project's number for the same issue, when the issue is linked there.
  Otherwise it becomes a link to the original issue, when that one is
  linked on its own project. Otherwise it goes as code, `` `#n` ``, so it
  can't point at the wrong issue.
- **So a reference is never mirrored as it is.** No `#n` reaches another
  project unchanged, where it would name another issue.
- **Titles are translated too,** as a title may hold a number, like a
  workflow's `(#2518)`. A title shows no link, so a reference sync can't
  translate is `` `#n` ``. A title's mentions stay as written.
- **A title naming its own issue** can be translated only once that issue
  exists on the forge. Sync creates the issue, then sets its title again.
- **An edit made on the forge** is that forge's text: git-board takes it as
  the forge shows it, without its header and marker. Sync never pulls back
  what it sent (see [Echoes](#echoes)), so nothing is translated back.

A mention is `@` and a login, of letters, digits, `-`, `_` and `.`, less
any `.` it ends with, not after a letter, a digit or `_`. After a lone
backtick, its code span is sent after a space, so that the two don't make
one run of backticks. A reference is `#` and digits, not after a letter, a
digit, `_`, `&` or `/`, nor followed by a letter, a digit or `_`. Neither
counts inside a code span: a run of backticks and what follows it, up to
the next run as long. A run that no run as long follows is a lone one.

## Refusals

A value a forge refuses stays in git-board, with one warning naming the
value and why.

- **Refused** means for good: the forge answers that the account may not
  do it, or leaves the value out, as GitHub does with an assignee who has
  no access.
- **The rest goes through.** Other values of the same issue are still sent,
  and so are the other labels of the same write: an adapter names the
  labels it refused (`forge.Refused`), sends the others, and sync records
  only those refused.
- **Last seen keeps the forge's value.** The refused value is still
  git-board's change.
- **Once per value.** Sync records the refusal, warns once, and doesn't
  send that value again. A later change of that field is tried.
- **The value** recorded is the one sent, in the forge's form. For labels
  and assignees, each is its own value: `+bug` for adding `bug`, `-bug`
  for removing it, and `+bob` for assigning the name `bob`. For a comment
  not posted yet, the field is `comment:<git-board comment id>`. For an
  issue the project won't create, the field is `issue` and the value
  empty.
- **A rate limit isn't a refusal:** sync waits, and tries again (see
  [Forges](forges.md)).

## Deletions

Deletions are never propagated, either way.

- **An issue or a comment deleted on a forge** stays in git-board. A linked
  issue gone from the forge stays linked. Sync writes nothing to it, and
  doesn't create it there again.
- **A comment sync posted** and the forge deleted isn't posted again: its
  `posted` record stays.
- **git-board deletes nothing on a forge.** It has no operation that
  deletes.
- **"Not found" is checked twice.** Sync takes a forge issue as deleted only
  when the posting account's token gets "not found" too. A forge gives
  the same answer when a token lacks access to a private project. When
  that account reads the issue, the write is refused; when its read fails
  otherwise, the run stops with that error, and records nothing.

## Nothing private published

- **Confidential issues are skipped.** Sync doesn't list them, pull them or
  push to them. Reading one on its own is an error. An issue that turns
  confidential once mirrored keeps its mirror, which then no longer syncs.
- **A public project gets nothing pulled from one that isn't,** unless
  `publish-private` is set. An operation whose `origin` names a project
  that isn't public is never sent to a public one. An issue whose `create`
  came from such a project isn't created on a public one, so nothing else
  of it goes either. Nor is one whose title or body such a project last
  changed: the create sends them, and nothing creates it with the values
  from before that change. For a field, the operation is the one that last
  changed it; for a comment, its `comment` and its last edit.
- **Each run reads the visibility** of every project it syncs with. One
  it can't read is left out of the run, as a destination read as not
  public would take what is private: the others sync, and the run returns
  an error naming it. A project the run doesn't sync with counts as not
  public.
- **An internal project** (GitLab's, or a GitHub Enterprise's), which
  every user of its forge may read, counts as public as a destination and
  as not public as a source. That fails closed both ways: nothing private
  goes to it, and nothing of it goes to a public project, unless
  `publish-private` is set. Between two internal projects, nothing goes
  either. The cost, if that's too strict: private to internal needs
  `publish-private`.

## Crash recovery

A run can stop anywhere: a crash, a network error, a write the forge made
without answering. The next run picks up from what is in git and on the
forge, and never makes a thing twice.

- **An issue pulled but not linked.** The pull wrote the issue, then
  stopped before its link. The next pull finds the forge issue unlinked,
  and an issue whose `create` has that forge issue as `origin`: it links
  that one, the first by id, instead of creating another. Its fields are
  then [what git-board took from the forge](#what-git-board-took-from-a-forge).
- **An issue posted but not linked.** The push created the forge issue,
  then stopped before its link. The next pull, or push alone, finds the
  forge issue with a marker from sync's accounts, and links the issue the
  marker names, with the `seen` a new forge issue gets: the title and body
  as the create sent them, header and marker included, so that a forge
  edit made since is the forge's change. The push then sends the rest.
- **A comment posted but not recorded.** The next pull finds the forge
  comment's marker, and records it with a `posted` and a `seen`. It isn't
  posted again.
- **A field written but not seen.** The forge then shows git-board's value:
  G = F, and last seen takes F.

## A run

A run syncs git-board with every project it is given:

1. It takes the run's lock, `board/sync.lock` in git's common directory,
   created only if it doesn't exist. Another run holding it is an error
   naming the file. The run removes it at the end. A run killed before then
   leaves it behind, to remove by hand, like git's own lock files.
2. It reads each project's visibility, and its posting account. A project
   whose visibility it can't read is left out, and the run returns that
   error at its end; a posting account it can't read stops it.
3. It pulls from each project: the issues listed as updated since that
   project's last `run`, or every issue when there is none.
4. It pushes to each project. As every project is pulled first, an issue
   pulled from one goes to the others in the same run.
5. It records a `run` in each project's sync entity it wrote to.

It reports how many forge issues it created, updated or left unchanged in
git-board, and how many issues it created or wrote to on the forges. Its
`Did` option hears each of them as it is counted, with its project, plus
each forge issue found by its marker or its origin and linked: git-board
counts them per forge, and shows a run's progress.

- **One issue alone.** Given an issue, a run reads its forge issue on each
  project where it is linked, or looks in each listing for its marker or
  its origin where it isn't, and syncs that issue only. It records no
  `run`, as it didn't read every change.
- **Pull alone and push alone.** Each is its step of a run, with the same
  rules. Neither records a `run`. A push alone still reads each forge issue
  it writes to, and never takes a value from it. Before it creates
  anything, it lists each project's issues as a pull would, and links
  those found by their marker (see [Crash recovery](#crash-recovery)), so
  that it never creates an issue twice.

## Switching from the fork

A repository switches from the git-bug fork (v0.11.1-sync.10) by pulling
its issues from its forges, with git-board's `switch`. The fork mirrored
each issue and comment to every project, so a forge holds originals and
the fork's copies of the others'. The pull makes one issue of each
mirrored group, never a second.

- **The fork's form,** as the wiki's GitLab and the scratch forges show
  it. A text the fork posted starts with its marker alone on the first
  line, `<!-- git-bug:<64 hex digits> -->`, then the text. A text it posted
  for someone else has a word after the id,
  `<!-- git-bug:<id> attributed -->`, then a header in italics and a blank
  line: `*Originally posted by <name> on <YYYY-MM-DD>, [view on
  <forge>](<url>)*`. The URL is the original issue's, or the original
  comment's: the issue's with `#issuecomment-<id>` (GitHub) or
  `#note_<id>` (GitLab). A header without the marker is text.
- **Pulled,** a text loses the fork's marker and header, whoever wrote it.
  Last seen keeps the forge's text, and a text sent back to that project
  gets them again, as [the forge's form](#the-forges-form) keeps sync's
  own header. The fork's copies end without spaces or blank lines, so a
  text sent in its form ends so too, and a text whose last seen is in its
  form compares without its start and without the space at its end: a
  forge edit that leaves a line end there, as a browser does, is pulled
  once, and the next pull writes nothing. No pull writes a value the
  field already has from that forge.
- **A mirror** is a forge issue whose body is in the fork's form. It
  waits until every project's other issues are pulled, whoever posted it,
  so that the originals come first. Then it joins the issue its group
  already has: from sync's accounts only, the one linked on another
  project at its header's URL (a comment's fragment aside), or pulled from
  a copy with the same marker id. With none, it is a new issue.
- **The fork's data.** git-board gives the run the fork's links, the
  `Mirrors` option: groups of forge issues, by project key (in any case)
  and number, that the fork made one issue. A forge issue in a group joins
  the issue another of the group was pulled into. That covers a copy with
  the marker only, and an original.
- **Joining** links the forge issue to the issue, with last seen as the
  forge has the title, body and status: the issue's values, the
  original's, win. Labels and assignees get no last seen, so the two
  sides' sets merge. A joined issue is never created on that project again.
- **A comment joins** one of the issue's comments that isn't from that
  project nor recorded there. The forge's comment is a copy when in the
  fork's form from sync's accounts; the issue's is one when its text was
  in it on the forge it came from. A copy's header names its original
  exactly: the comment pulled from the forge comment its URL's fragment
  names, on the host the URL names. Failing that, it joins the first with
  the same text, which compares without the fork's start, backticks,
  references (`#n`, or a link to one) and runs of spaces, as either side
  may have translated them. A `posted` records the join. Last seen is the
  forge's text when the forge has the copy, else git-board's, so that the
  original's text is pulled.
- **What a join leaves.** A text the fork translated otherwise than sync
  does shows as an edit waiting for that forge, never as a new issue or
  comment.

## Pull, the trial's part

M1's trial built this part of pull, and nothing else of sync. It reads a
forge's issues as they are now, without their events, and writes into
git-board only. It covers the title, the body, the status, the labels and
the comments.

M3's pull follows these steps, with these changes:

- it lists the issues updated since the project's last `run` (see
  [A run](#a-run));
- before making a new issue, it looks for one to link: by the forge issue's
  marker, or by an issue's origin (see [Crash recovery](#crash-recovery));
- it compares git-board's values in [the forge's form](#the-forges-form),
  and with [what git-board took from the forge](#what-git-board-took-from-a-forge);
- a field both sides changed goes to [the later change](#the-four-cases);
- it pulls assignees, and records the comments it finds by marker;
- a text sync's accounts posted loses its header and marker.

### What the forge gives

For each issue: its id, number and URL, its author (a numeric id and a
login), its creation time, its title, body and status, its labels, and its
comments. A comment has an id, an author, a time and a text.

- The status is `open` or `closed`. Any other value is an error: the pull
  stops there, before writing anything for that issue.
- The labels are the issue's, from which the pull takes its `labels` value:
  without [assignee labels](#assignees), unique and sorted.
- From M3, it also gives the issue's assignees, as accounts, its last
  update, and when its title, body and status last changed, and a comment's
  last edit. Those times serve only [the later change](#the-four-cases).

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
the push. A field both changed to different values goes to the later
change: the pull takes F when the forge's change is the later, and leaves
it for the push otherwise, last seen included. Labels are the exception:
they merge per label, and so can't conflict.

The title, last seen as `Search ignores closed issues`:

| git-board's (G) | the forge's (F) | Writes |
|---|---|---|
| `Search ignores closed issues` | `Search ignores closed issues` | nothing |
| `Search ignores closed issues` | `Search skips closed issues` | `set-title`, then a `seen` |
| `Search hides closed issues` | `Search ignores closed issues` | nothing |
| `Search hides closed issues` | `Search skips closed issues` | the later change: `set-title` and a `seen` when it is the forge's, else nothing |
| `Search skips closed issues` | `Search skips closed issues` | a `seen` |

Labels last seen as `bug`, with `ui` added in git-board and `docs` on the
forge: the pull adds `docs`, and last seen becomes `bug` and `docs`. `ui`
waits for the push.

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
- The pull reads no events, so it can't tell who changed a field on the
  forge. So its operations carry the pull's time, and the issue's author.
- **Why `link` and `seen` take the issue's author.** They record what the
  pull found, so only their time matters, and it is the pull's. They still
  need an author. The pull doesn't read `board.me`, which CI doesn't have,
  so they take the pulled issue's person.

### Authors

The forge gives an issue's or a comment's author as a numeric id and a
login. With the forge's server, like `github.com`, they name an account:
its `id` is the numeric id, as a string (see [People](data-model.md#people)).
The pull loads every person once. A person that fails to load stops the
pull with an error, before it writes anything for the issue at hand:
ownership can't be decided without every person. Issues already pulled
stay pulled, as with any failed write. Then, for each author:

- **A person holds the account:** the author is that person, or, when two
  hold it, its [owner](data-model.md#account-owner).
- **Its login changed on the forge:** the pull appends an `add-account`
  with the new login to that person, authored by the person itself. The
  person stays the same.
- **No person holds it:** the pull creates one, in one write. Its first
  pack holds a `create`, with the login as the name and an empty email,
  then an `add-account` of the account. Both have an empty author, as the
  person's id comes from that pack. Later authors in the same pull find it.

### What follows

- **Pulling twice in a row writes nothing.** After a pull, every field has
  F = S, or both changed and the pull writes nothing.
- **A change made in git-board is kept.** Where G ≠ S, the pull writes
  nothing to the issue, unless the forge changed it later.
- **A login renamed on the forge** keeps its person, whose login changes.
- **A pull that stops between an issue's write and its sync entity's**
  leaves a linked issue's field at G = F ≠ S. The next pull then records
  the `seen`. For a new issue, it leaves the issue without its link, and
  the next pull finds it by its `create` operation's `origin`, and links it
  (see [Crash recovery](#crash-recovery)).
