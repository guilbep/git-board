# The protocol: what the time flow stores, and how it syncs

Companion to `README.md`. Key names are a proposal in the core's form
(`docs/spec/data-model.md`): the core gives no key a meaning, git-board
does. Nothing new is stored: no new entity, no new operation kind.

## One rule

A thing a person does in the time flow is one of the core's seven issue
operations, with `meta` keys where git-board needs to read it back. The
forge gets the same thing as text a person can read without git-board.

## The keys

| Key | On | Value | Set by |
| --- | --- | --- | --- |
| `cycle:step` | a `comment` | `"01"` … `"07"` | the composer, from the issue's step at that moment; exists today |
| `thread:anchor` | a `comment` | an anchor, below | the composer, when started from a section's gutter or from a thread |
| `thread:reply-to` | a `comment` | the root comment's id | the composer, when replying in a thread |
| `thread:kind` | a `comment` | `"suggestion"` | the composer, when the body carries a ```suggestion block |
| `thread:resolved` | a root `comment`, by `set-meta` | `"v2"` (the version that resolved it) or a person's id | Apply, or the *Resolve* button |
| `thread:applied` | a root `comment`, by `set-meta` | the id of the `edit-text` that applied the suggestion | Apply |
| `spec:verdict` | a `comment` | `"approve"` or `"request-changes"` | the verdict bar |
| `spec:on` | the same `comment` | the body's version the verdict is on, `"body@v2"` | the verdict bar |

A `comment` with none of these is a plain comment on the issue, as today.
`review:*` keys are untouched.

### The anchor grammar

```text
<text>@v<n>/<section>[#<item>]
body@v1/expected#3        the 3rd item of the list under "Expected", in v1 of the body
body@v2/intro             the prose before the first heading, in v2
plan@a1f0c3e/steps#4      later: the 4th step of the plan file at that commit
```

- `<text>` is `body` (the issue's text, the `create` operation's, target
  `<issue id>:0`). `plan@<commit>` is reserved for the plan file; not in
  the first milestones.
- `v<n>` is the text's version, counted (below).
- `<section>` is the heading's slug: lower case, runs of other characters
  as one `-`, an index appended when the slug repeats (`expected-2`). The
  prose before the first heading is `intro`.
- `#<item>` is the 1-based index of a list item in the section; absent for
  a comment on the section as a whole.

## Versions and ticks

The body's versions are never stored. Reading the issue's operations in
the core's order, v1 is the `create`, and each `edit-text` with target
`<issue id>:0` that changes text is the next version. An `edit-text` that
differs from the previous text only by `- [ ]` ↔ `- [x]` flips is a tick,
listed in History and Changes, not numbered.

"Changes since v*n*-1" is a diff of the two texts, section by section.
The section list of a version is the list of its headings; an anchor is
resolved against the version it names, never against the current one.

## Carried, outdated, resolved

Computed when the page renders, for a thread whose anchor names an older
version than the current one:

| State | Rule | Shown as |
| --- | --- | --- |
| carried | the anchored section's text is the same in the current version | the thread under the section, "carried from v1" |
| outdated | the section's text changed, and the root has no `thread:resolved` | the thread under the section, amber, "written on v1 · §4 changed in v2 · still open" |
| resolved | the root has `thread:resolved` | folded, "resolved by v2" or "resolved by Person B" |

A suggestion applied by *Apply* does two operations in order: the
`edit-text` that is the next version, then `set-meta` on the root with
`thread:resolved` = that version and `thread:applied` = the edit's id.

## The verdict

A verdict is a `comment` with `spec:verdict` and `spec:on`. Its body is
the comment the reviewer wrote (a required one for request-changes). An
approve on the current version moves the status to ready through the same
path as today's *Move to ready*; a later version makes the approval stale
(the bar says "approved v2, v3 awaits") without touching the status (open
point 2 of the README).

## Steps and segments

Segments are computed: the flow is cut at the operations that opened each
step, found as `cycle.ts` finds the step today (the status and the
evidence). A comment's segment is its `cycle:step`; an operation's is the
step current at its time. Nothing new is stored for this.

## What sync does

`docs/spec/sync.md` stays the reference. The time flow adds three rules:

1. **A thread comment on the body is posted** to the forge, unlike a
   `review:*` line comment, which stays local. It is posted as a flat
   comment, in time order with the rest.
2. **The text sync posts** is the comment's body with a header and the
   marker. The header is a quote: `> **On v1 · § Expected · 3** · _the
   anchored line_`, the section's heading as written, the item's text in
   italics; a reply repeats its root's header. A suggestion keeps its
   ```suggestion block, which GitHub shows as code. A verdict's first line
   is bold: `**Approved v2 of the description**`. A published version is
   the issue body, edited (the forge keeps the edit history), plus a
   comment `**Published v2** · § intro, § Expected, § Workaround changed`.
3. **The keys ride in the marker.** Sync already writes
   `<!-- git-board comment <id> -->` on the last line. The thread, spec
   and `cycle:step` keys are appended to it, `key=value` separated by
   spaces: `<!-- git-board comment 9c41…e77a:0 step=01
   anchor=body@v1/expected#3 reply-to=7d2f…0a1c kind=suggestion -->`.
   Pull strips the header and the marker, as it strips them today, and
   restores the keys as `meta`, so a comment that went out and came back
   is the same comment. A marker in a stranger's text is just text.

GitLab: a thread (a root and its replies) is one discussion; resolving the
thread resolves the discussion. The quote header is still written, so the
discussion reads the same way.

An edit made on the forge to a thread comment is the forge's text, as for
any comment; the keys are preserved by the marker unless the person
deleted the marker, in which case the comment becomes a plain comment.

## The example of 6f, in full

The suggested edit of Person B, as the core stores it:

```json
{
  "kind": "comment",
  "author": "b7e41c09d2a85f36e1c4a0d7b9f2e5c83a6d1f40",
  "time": 1792764300,
  "data": {
    "body": "Then say what replaces the CLI text.\n\n```suggestion\nA missing identity sends the user to Settings › identity; the CLI text is never shown.\n```",
    "meta": {
      "cycle:step": "01",
      "thread:anchor": "body@v1/expected#3",
      "thread:reply-to": "7d2f0a1c4e9b36d85a2f1c0e7b4d93a6e8f5c2b1:3",
      "thread:kind": "suggestion"
    }
  }
}
```

As sync posts it to GitHub #37:

```markdown
> **On v1 · § Expected · 3** · _A missing identity sends the user to Settings › identity, not to a CLI command._

Then say what replaces the CLI text.

```suggestion
A missing identity sends the user to Settings › identity; the CLI text is never shown.
```

<!-- git-board comment 9c41e77a2d5f8b3c6e1a0d4f7b9c2e5a8d3f6b1c:7 step=01 anchor=body@v1/expected#3 reply-to=7d2f0a1c4e9b36d85a2f1c0e7b4d93a6e8f5c2b1:3 kind=suggestion -->
```

When Person A applies it: an `edit-text` on `<issue id>:0` with the new
body (v2), then a `set-meta` on the root comment
`7d2f…c2b1:3` with `{"thread:resolved": "v2", "thread:applied": "<the
edit-text's id>"}`. On GitHub: the body edited, a comment `**Published v2**
…`, nothing else.
