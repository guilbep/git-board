# A long sync you can follow, and cut

PRD, 2026-10-29. The user asked for it and skipped the questions ("ok do
it!"), so every decision here marked **(call)** is this document's, open
to change. What the user said is marked **(user)**.

## Why

The first import after a start-over pulls every issue a forge holds:
1,000 and more for co2-calculator, several minutes. **(user)** Three
things go wrong on such a run:

- **The progress says nothing useful.** Since `2172ebc` (the sync moved
  onto the core) the run never says how many issues it will go through,
  so the bar has no end, and it moves only when an issue changes. The
  minutes go before the bar even starts: `Issues()` reads every issue's
  comments (GitHub: one request per issue with comments; GitLab: its
  notes and state events) before the syncer sees the first one. All the
  page shows meanwhile is "Syncing the forges…".
- **A cut run blocks the next.** The run's lock, `board/sync.lock`, is a
  file made at the start and removed at the end. A shutdown, a crash or
  a `kill -9` leaves it, and every sync after fails with "another sync
  runs" until someone deletes it by hand (docs/spec/sync.md, "A run").
- **A cut run starts over.** What it pulled stays pulled, each issue
  written as it went, and nothing is made twice. But it records where it
  got to nowhere, so the next run reads every issue's comments again:
  the same minutes. A read gives up after three tries in about seven
  seconds, so a Wi-Fi drop or a laptop going to sleep cuts the run.

The user wants a warning that it takes minutes, a progress bar, and a run
that survives a lost connection or a shutdown, continuing gracefully.
**(user)**

## Decisions

| Topic | Taken |
| --- | --- |
| The lock | An OS lock on `board/sync.lock`, held for the run: `flock` on Unix, `LockFileEx` on Windows. The OS drops it when the process dies, however it dies, so a file left behind blocks nothing. A run that finds it held is told another sync runs, with that run's process id. The file is never removed, so two runs can never lock two different files of that name. **(call)** |
| The listing | `Issues()` reads what comes in pages: the issues, and GitHub's change times, a hundred per query. A listed issue may be **partial**: its comments, and on GitLab its notes and state events, are read by a new `Complete()`, which the pull calls on each issue as it reaches it. Every marker, origin and mirror match reads only an issue's body, so it works on the partial issue. The push's reads already come from pulled issues, or from `Issue()`. **(call)** |
| Progress | The syncer reports each issue its pull reaches, done of total, the total known as soon as the listing is in, seconds into the run. Its push reports each issue it walks the same way. The bar has an end, and the line names the issue, as it does today. **(call)** |
| Time left | The page estimates it from the rate so far, once 20 issues are done: "about 6 min left". **(call)** |
| The warning | When the total is in and 200 issues or more are left to read, or the estimate passes a minute, the progress says: about N issues to read, this takes several minutes; the page can close, the sync goes on in the server; a cut sync goes on next time from where it stopped. The command line prints the same once. **(call)** |
| What a cut run leaves | The lock file holds the run's journal. Its first line is the run: process id and start time. Then it gets one line for each forge issue the pull has pulled, or found unchanged: the forge, the issue's id, and the start of the run that read it. A run that ends without recording its `run` leaves its journal: cut, failed, stopped, or a pull alone. A run that records its `run` empties the file. **(call)** |
| Going on | A run reads the journal it finds and writes back what it keeps. A listed issue is skipped, with no `Complete()` and no pull, when all three hold:<ul><li>the journal has it;</li><li>it is linked;</li><li>the forge says it was last updated more than 5 minutes before the start of the run that read it.</li></ul>So the issue is as git-board last read it: this is `since`'s rule, applied to what the cut run reached. 5 minutes is more than the adapters' own margins (GitHub 1, GitLab 2) for the two clocks. A skipped issue stays out of the push's listing, so a push that writes to it reads it fresh. A run of one issue alone neither reads nor writes the journal, and neither does a dry run, on its own copy. **(call)** |
| Not reached | Nothing goes in the journal for an issue the pull didn't finish:<ul><li>one whose write failed;</li><li>one skipped because its issue isn't here;</li><li>a mirror waiting for the end of the walk, until it is pulled.</li></ul> **(call)** |
| Retries | A read retries a broken connection, a timeout or a 5xx until its waits add up to 5 minutes, waits doubling to at most 30 s (each try has its own minute's timeout on top). The progress line says so: "GitHub doesn't answer: trying again in 30 s". A rate limit's wait, already waited out, is said the same way. A write is never retried, as before: the forge may have made it. **(call)** |
| After a restart | The run is gone from the server's memory, but its journal is on disk. Settings › Sync says the last sync stopped at k of n issues from that forge, and when, and that Sync goes on from there. The command line says the same when it starts a sync over a cut journal. Nothing resumes by itself: a sync writes to the forges, and the person starts it. **(call)** |
| The bar's numbers after a cut | The next run counts its total over the whole listing. Issues the journal lets it skip count as done at once, so the bar starts where the cut run stopped. **(call)** |

## Not in this PRD

- **Pulling in parallel.** One issue at a time, as today; GitHub's
  secondary rate limits punish bursts.
- **A sync that starts by itself** after a restart.
- **Faster writes.** A push that creates 1,000 issues stays paced by the
  forge's write limits (GitHub's 500 an hour). Its bar and estimate show
  it, and it goes on after a cut as before: what it created is linked, or
  found by its marker.

## Left to the builder

- The journal's exact line format, and the names of the new options.
- How the server carries a step's total and a wait to the page.
- The wording, in the page's voice.

## Breaking

- **The lock file's meaning.** An older git-board, still running a sync
  without the OS lock while this one starts another, wouldn't stop it.
  That only happens during the upgrade itself. **(call)**
- **The forge interface** gains `Complete()`: every adapter, and the dry
  run's wrapper, implements it.
- **An older git-board refuses to sync in a clone where this one has run.**
  It makes the lock file only if it doesn't exist, and this version never
  removes it. Every git-board that syncs one clone, a cron job's included,
  needs this version; until then, deleting `board/sync.lock` lets the old
  one run once. **(call)**
