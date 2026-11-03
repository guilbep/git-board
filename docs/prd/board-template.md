# A new board starts as its forge, or as ENAC's template

Status: PRD 2026-11-03, issue p1. Decisions marked (user) are Pierre's.

## Why

A new board starts from git-board's built-in default
(`internal/config/default.git-board.toml`). The team's GitHub repositories
and Project don't match it, so the first thing a new user sees is a Labels
page of conflicts: "Color differs · Adopt" on every label, descriptions that
disagree, Project values spelled otherwise (`P0` / `p0`, `Must` / `musthave`),
fields git-board doesn't have (Sub-issues progress). The goal is **no
conflict between the forge and git-board on the first init** (user).

## Decisions

| Question | Decision |
|---|---|
| Where `init` takes the board from | **the forge first**: when origin's repository has labels or a GitHub Project, `.git-board.toml` is written from them (names, colors, descriptions; the Project's fields, values in order, their descriptions). Nothing to adopt after |
| When the forge has nothing | a **template**, by name: `git board init --template enac` (built in), or a path or URL |
| The ENAC template | the EPFL-ENAC GitHub Project template (user): Status (Backlog · Ready · In progress · In review · Ready for validation · Validated, each with its description), Priority P0–P3 ("Now: production broken…", "This cycle…", "Soon…", "Someday…"), MoSCoW (Must · Should · Could · Won't, with descriptions), Sprint (iteration), Est. hours (number), Sub-issues progress; the labels bug, chore, design, docs, feat, hold:blocked, hold:paused, perf, polish, refactor, style, test with their colors and descriptions |
| A global config | `~/.config/git-board/config.toml` may name the **template `init` uses** (`template = "enac"`), nothing else: it never changes a board that exists, so two teammates always see the same board from the same repository |
| Template changes later | **copied once** at init, then the repository's (user); `git board lint` says where a board has drifted from its template |
| Branch names | **a branch's prefix is its kind's label, always** (user: no divergence): `bug/…` for `bug`. `[branches] kinds` can no longer map a kind to another word (`bug = "fix"` is refused, with the fix in the message); a label description naming another prefix is reported by lint |
| Values' spelling | git-board keeps the forge's: `P0`, `Must`. No renaming layer on a fresh board |

## Acceptance

1. `git board init` in a clone of a repository with the ENAC labels and
   Project: Settings › Labels shows **no** "Color differs", no "Adopt"; the
   Project's fields match, values and order included.
2. A repository with no labels and no Project: `init --template enac` gives
   the same board as 1.
3. `~/.config/git-board/config.toml` with `template = "enac"`: a plain
   `git board init` uses it; an existing board is untouched by it.
4. `[branches] kinds = { bug = "fix" }` is refused with a message naming the
   line; the branch for a `bug` issue starts `bug/`.
5. A board whose labels changed after init: lint lists the drift.

## Out of scope

Seeding template issues; a second template; renaming values on boards that
exist.
