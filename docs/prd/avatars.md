# Avatars: a face where there are initials

Status: implemented 2026-11-02 (go given), not yet committed. Plan: `avatars-plan.md`.
Context: the header redesign exploration (user menu, filter help, the ?
key, tips), https://claude.ai/artifact/Fpo7pBejtJDvPtce4AjQPY. Whichever
user menu wins there (A1–A3), its button shows this avatar.

## Why

Every person in git-board is two letters in a square: the header's "GU",
comment authors, assignees in the issue head and the field editor, the
inbox's "who", the composer. On a board with a dozen people, initials
collide (two "PG"s) and nobody recognises a square at a glance. The page
is half ready: `store.Person` has an `avatar_url` and `CommentCard.vue`
shows it, but no server code ever fills it.

## What we know (checked in the code, 2026-11-02)

- A person is `core/people.Person`: `ID`, `Name`, `Email`, and
  `Accounts` (`Server`, `ID`, `Login`). The page sees `store.Person`
  (`id`, `name`, `login`, `avatar_url`), made by `personOf` in
  `internal/store/thread.go`.
- **People pulled from a forge have no email**: the syncer creates them
  with `people.NewCreate("", login, "")` (`core/syncer/pull.go`). An
  email exists mainly for identities made in git-board (Settings › You,
  `POST /api/me` with name and email): you, usually.
- **A GitHub account's `ID` is GitHub's numeric user id**
  (`core/forge/github`, `user.account()`), and its `Server` is the
  forge's host (`github.com`). GitHub serves a public user's avatar at
  `https://avatars.githubusercontent.com/u/<id>`, no token, no API
  call; enterprise-managed users (EMU) are the exception (private
  avatars: that URL fails for them).
- Gravatar takes the SHA-256 of the trimmed, lower-cased email:
  `https://gravatar.com/avatar/<hash>?s=<px>&d=404`; `d=404` answers
  404 when the person has no Gravatar instead of a generated image.
- The page's CSP already allows `img-src https:` (`web/index.html`):
  nothing to change.
- Person fields (`assignee:alice`) hold **logins**; the page names them
  through `/api/me`'s `identities` (the same `nameOf` is written three
  times: `IssueHead.vue`, `FieldEditor.vue`, `FieldPanel.vue`).
- The precedent for a personal display setting is `board.view`: git
  config, read on every `/api/me`, set from Settings › You (`PUT
  /api/me/view`), `--global` for every repository.

## Decisions

| Question | Decision |
|---|---|
| Where the picture comes from | 1. the person's **github.com account** (by numeric id); 2. **Gravatar**, by email, only if switched on; 3. **initials**, as today |
| GitLab, GitHub Enterprise | no picture yet: their avatar URLs need an API call (GitLab) or differ by host (GHE). Initials. Later: store the forge's `avatar_url` at pull (see Later) |
| Who computes the URL | the **server**, into `store.Person.avatar_url`, everywhere `personOf` makes a person. The email never goes to the page; the page only renders what it gets |
| The setting | git config **`board.avatars`**: `forge` (unset means this), `gravatar` (forge, then Gravatar), `none` (initials only). Per person, like `board.view`; `--global` for every repository |
| Default | `forge`. **Gravatar is opt-in**: it sends a hash of every listed person's email to a third party, and a SHA-256 of a guessable address (an EPFL directory) is reversible by guessing |
| Fallback | an image that fails (offline, EMU, 404 from `d=404`) shows the **initials square**, same size, same place, no broken-image icon |
| Shape | the existing square `gb-avatar` (EPFL: tight 2px radius), the photo `object-fit: cover` in it. Never Gravatar's own initials (coloured circles: off-brand) |
| Size asked | twice the drawn size, for retina: 88px for the header's 44px button, 40px for the 20px small avatars; one URL per person at 88px is simpler and cached once: **88px everywhere** |
| Accessibility | the image is decoration: `alt=""`, `aria-hidden`; the name stays where it is said today (the button's `aria-label`, the author's name beside a comment, `title` on field values) |
| Settings | Settings › You, under Appearance, gains "Pictures": Forge accounts / Forge accounts, then Gravatar / Initials only, with one line on what each sends where |

## What each choice sends where

| `board.avatars` | The browser asks | What that tells them |
|---|---|---|
| `none` | nobody | nothing |
| `forge` (default) | `avatars.githubusercontent.com` | which GitHub user ids appear on the page you opened: people already on that GitHub repository |
| `gravatar` | the above, and `gravatar.com` | a SHA-256 of the email of each person who has one, mostly you and local identities |

## Behaviour

1. `/api/me`'s `me` and `identities`, and every person in an issue's
   thread (authors, participants, the inbox's `who`), carry `avatar_url`
   when the setting and the person allow one; otherwise no field.
2. The header's avatar button shows the picture in its 28px square;
   comments, the composer, the issue head's person fields, the field
   editor, the people button, the inbox and the branch face show it in
   their small square. Person fields find it by login through
   `identities`, as they find the name.
3. A picture that fails to load falls back to initials for that page
   load; no retry loop.
4. Changing the setting in Settings › You reloads `me` and the open
   issue: pictures appear or go without a page reload. Changing it from a
   terminal shows at the next reload (as `board.view`).

## Out of scope

- Uploading a picture to git-board, or storing pictures in the repo.
- Proxying or caching images through the server (would hide the person
  ids from GitHub, and work offline after a first load): a later step if
  the privacy line above is not enough.
- Libravatar / self-hosted avatar services.
- The user-menu redesign itself (A1–A3 on the canvas): separate decision.

## Later

- At pull, keep the forge's own `avatar_url` per account (GitLab, GHE,
  EMU) as an account field, so every forge gets pictures without a
  guessed URL.
- A server-side image proxy with a disk cache (offline, no third-party
  requests from the browser).

## Open questions for review

1. Default `forge` (pictures on, GitHub only) or `none` (nothing changes
   until someone opts in)? This draft says `forge`: the ids are the
   repository's own GitHub people, so the browser makes a new request to
   GitHub but tells it nothing it doesn't already hold.
2. Gravatar's `d=404` makes the browser log a 404 for each person
   without a Gravatar. Accept the console noise, or have the server check
   once and remember (a network call from the server)? This draft
   accepts it.
