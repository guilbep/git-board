# The exploration: rows 8 to 19 of the canvas

The canvas "git-board · one issue, one plane?" from row 8 on, as its notes
say it. Row 7 is `docs/design/project-cycle/`. The boards are not copied
here: the earlier rows draw #2771 as it is, with its real people. The
screens of the decisions are in `../screens/`, with made-up people. Read for
the reasoning, not as a spec.

| Row | Boards | What it tried |
| --- | --- | --- |
| 8 | 8a step back; 8b A, fields under the title; 8c B, panes; 8d C, one column weighted | three organizations of the side column |
| 9 | 9a D fold; 9b E sentence; 9c F waits; 9d G dock; 9e H lenses; 9f I keys | six more directions, mixed or new |
| 10 | 10a I1 margin; 10b I2 outline; 10c I3 column; 10d I4 point; 10e I5 motion | I iterated: a reading width, one quiet line, one editor |
| 11 | 11a protocol; 11b to 11d the sheets; 11e scoreboard | A/B round 1: sixteen designs, three issues, three widths |
| 12 | 12a merge; 12b rounds 2 to 4; 12c J-line 2 | the merge and the one that held |
| 13 | 13a re-test; 13b dropdowns; 13c PRs; 13d counts | J-line 2's corners, part 1 |
| 14 | 14a threads; 14b life; 14c phone; 14d rules | J-line 2's corners, part 2 |
| 15 | 15a where we are; 15b the EPFL pass | the working document; the design system to the letter |
| 16 | 16a to 16g | the seven sections, each where it matters |
| 17 | 17a to 17d | assembled, before the bar |
| 18 | 18 | the blind check of three open choices |
| 19 | 19a to 19f | the bar, the arrow, the branch proposal |

## The notes on the canvas, top to bottom

### 8 · The side column: a step back, then three organizations

Raised 2026-10-29: the side column holds too much, all at one weight. 8a
takes it item by item, who needs what and when, and sets six rules. Then
three organizations of #2771 in the drawer's 64rem. A: the fields as a row
of dropdowns under the title, the work in one line, sync, people and
following on the meta line; the conversation gets the whole width. B: the
column becomes four panes, Fields, Code, Forges, People, one at a time,
badges for what waits. C: one column, read first, status on top,
Development and Forges as one-line summaries. The weights go in `[issue]` in
`.git-board.toml`. Development is drawn with branches named by the issue's
own number: today GitLab #1 links every branch whose first number is 1.

### 9 · Six more: mixes and new ideas, D to I

Raised 2026-10-29, while A, B and C are weighed: more directions, mixed or
new. D folds A into C as the drawer narrows, one component. E writes the
issue as a sentence of words you click. F puts what waits, and on whom,
before the fields. G pins the fields, the work and the forges in a dark dock
at the bottom edge. H gives each person a lens, Plan, Build or All, from
their role. I keeps one quiet line and moves every change into the command
palette, with the board's filter syntax. They combine: F's waits over A's
row, I's keys under any of them. Participants show only when someone
besides the author and the assignees took part.

### 10 · I, iterated: a reading width, one quiet line, one editor

Raised 2026-10-29 by the designer: A is too busy; in B and C the side column
keeps the text's lines short, which reads better. I is the direction kept:
one quiet line and the palette. Measured in Arial 15px, A's lines run to
about 130 characters, B's and C's about 85; every I below keeps a 640px
card, about 85. I1 brings the column back as plain marginalia. I2 keeps the
line under the title and gives the margin to the description's outline. I3
drops the margin: title, line, tabs and text share one centred 640px
column. I4 opens the palette under the value you click, already typed with
that field, so the mouse and the keys share one editor. I5 puts the line in
motion: a slim bar once it scrolls away; at 26rem two rows and a sheet from
the bottom.

### 11 · A/B, round 1: every proposition rebuilt and tested on the same issues

Raised 2026-10-29: test every proposition of rows 9 and 10, A/B, without
waiting for people. Each was rebuilt as a prototype on the same three issues
at three widths, measured in the browser, timed with the Keystroke-Level
Model, and ranked by three blind judges, simulated by a model: the
title-only PM, the PO, the developer. What held: the text in a 640px column
(A's lines run to 129 characters, B's and C's to 83); the status as one
boxed control, with the next status one click away (H's Plan lens: 2.65 s
instead of 5.30); empty fields you can reach with the mouse (I2 and I3 had
no way); and the waits said in full (I4's bare "waits" read as cryptic to
two judges). I1 was the only design in the top three of all three finals.

### 12 · Rounds 2 to 4: the merge, its challengers, the one that held

Rounds 2 to 4 tested the merges with fresh judges. Given the same parts, the
margin (J) won for the developer and the one block over a centred column
(J-line) won for the PM and the PO. J-line 2 tightened the head, putting the
text at 473px instead of 540; it won round 3 (mean rank 1.67) and round 4
against K, 2 to 1. The judges are a model: before building, spend twenty
minutes with a PM, a designer and a developer on the five tasks.

### 13 · J-line 2, every corner: a correction, the dropdowns, the counts

Raised 2026-10-29: explore every corner of the recommendation. First a
correction: rounds 2 to 4 drew J-line without the crowded issue's labels,
extra fields and draft PR. Drawn complete and re-judged, it still wins 2 to
1 (the PM clearly, the developer slightly); the PO now prefers the margin.
For the PRs, a strip, lines and a single line were tested blind; the
synthesis (V4) won both pairs clearly: a line only for a PR that needs
someone, failing first in red, the waiting one naming who; drafts, merged
and closed as a quiet tail. Counts come from git-board's own 40 issues
(median 1 comment, at most 10) and #2771; co2-calculator's weren't
available, so 4, 9 and 12 are stress levels.

### 14 · J-line 2, every corner: long threads, an issue's life, the phone, the rules

Every part has a cap and says what it leaves out: people 4 faces and 2
names, then +N; one chip for the labels, one for the other fields; at most 4
lines of what waits, yours first, then what blocks, then the others, the
rest in the tail; the whole thread up to 30 entries, then the first 2, a
fold, the last 10. The next status reads "Close as validated" at the last
step, as git-board's cycle closes the issue there.

### 15 · Where we are, and the EPFL design skill to the letter

Raised 2026-10-30: keep the canvas as the working document, follow the EPFL
design skill to the letter, and redo seven sections: fields, labels,
development, PRs and create branch, the forges, participants, the inbox.
15a says where we are; 15b how the skill applies. The adherence linter: 0
problems on everything new; 517 on git-board's web today, in 56 of its 75
style files (415 px, 100 hex, 2 fonts).

### 16 · The seven sections, one by one: each where it matters

The waits block is gone: each wait sits on its source. Inbox: a bell.
Forges: an icon each by the title, a count with an up arrow when changes
wait. PRs: one chip per branch, colored by whose move. Create branch: a
popover on `{kind}/{number}-{slug}`. Labels: none unless the project file
lists them. Participants: at the end of the tab row.

### 17 · Assembled: the head in three lines

The drawer at 1024, 704 and 416, and six corners of an issue's life: just
opened, closed, not on a forge yet, a forge out of reach, ready to merge,
unfollowed. Superseded by row 19's bar.

### 18 · Blind check: three choices the notes left open

Three judges, cold. A word on the PR chips: kept, 3 to 0 (the PM, cold on
the number alone: "something is wrong, but I can't tell what"). The forges
by the title: kept, 3 to 0; none could tell the bare count's direction,
hence the arrow. Participants at the top, with the words of the composer's
version, which all three asked for. The arrow before a chip's face and the
arrow in the forge count came after: untested.

### 19 · The bar holds the issue's identity; the title gets the column

Raised 2026-10-30: the forges, the bell and the number go up into the bar so
the title takes one line where it can; *Move to* becomes an arrow; no dead
blank. 19f answers the naming question: a folder per issue,
`test/2771/1-slug`, and three steps of a branch, branch, review, merge,
with the forge's PR as the gate only. To decide.
