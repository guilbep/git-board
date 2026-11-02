# Avatars Implementation Plan

> **For agentic workers:** steps use checkbox (`- [ ]`) syntax for tracking. One commit per task.

**Goal:** People show a picture instead of initials where one is known: their github.com account's avatar by default, Gravatar if the person switches it on, initials otherwise and whenever a picture fails to load.

**Architecture:** One pure function, `avatar.URL(p people.Person, mode Mode) string`, in a new `internal/avatar` package, with every rule. `internal/store` fills `Person.AvatarURL` with it wherever `personOf` makes a person, reading the mode from git config `board.avatars` (memoised per request, as below). `/api/me` reports the setting and `PUT /api/me/avatars` sets it, mirroring `board.view`. The web gets one `GbAvatar.vue` (picture, or initials when there is none or it fails) used at every place that draws `gb-avatar` today, and one `personByLogin` helper replacing the three copies of `nameOf`.

**Tech Stack:** Go (stdlib `crypto/sha256`, `net/url`), Vue 3 + Quasar + TypeScript, `node --test` for web unit tests, Playwright e2e.

**Spec:** `docs/prd/avatars.md` (read it first).

## Global Constraints

- No new dependency, Go or npm. No CSP change (`img-src https:` already allows both hosts).
- The email never reaches the page: only the URL (which carries its hash under `gravatar`).
- `board.avatars` values: `forge` (and unset), `gravatar`, `none`; anything else is reported as ignored and treated as `forge`, as `board.view` reports its own.
- Picture size asked: 88px, everywhere (`s=88`).
- github.com only: `Account.Server == "github.com"`, its numeric `ID`. Never a URL guessed for GitLab or another host.
- A new Go test calls `t.Parallel()` first, unless it sets git config globally or swaps a package-level variable (CLAUDE.md).
- Unit-tested web modules import types relatively (`./api/board`), as `issuesync.ts` does.
- Commits: `feat(avatars): …` / `docs(avatars): …`, lower case, ending with the Co-Authored-By line. Per task: the package's own tests; `make ci` once, at the end.

## Review Focus

1. **Offline, or an EMU account, or a Gravatar 404**: the initials square shows, same size, no broken-image icon, no layout shift. Pinned in Task 5 (e2e `avatar falls back to initials`).
2. **`none` sends nothing**: no `avatar_url` anywhere in `/api/me` or an issue's thread. Pinned in Task 3 (`TestMeAvatarsNone`).
3. **No email in any response**, whatever the mode. Pinned in Task 3 (`TestMeNeverSendsEmail`).
4. **The email is trimmed and lower-cased before hashing** (`" Ann@EPFL.ch "` and `"ann@epfl.ch"` give one URL). Pinned in Task 1.
5. **A person with several accounts** (a login changed, two forges): the first `github.com` account, by the `Accounts` order (sorted by Server, ID). Pinned in Task 1.

---

## File Structure

| File | Responsibility |
|---|---|
| `internal/avatar/avatar.go` (new) | `Mode`, `ParseMode`, `URL` |
| `internal/avatar/avatar_test.go` (new) | the rules, table-driven |
| `internal/store/thread.go` | `personOf` takes the mode and fills `AvatarURL` |
| `internal/store/store.go` | `AvatarsSetting`, `SetAvatars` (git config `board.avatars`), the per-call memo |
| `internal/store/identities.go` | `Identities`, `NewIdentity` pass the mode |
| `internal/server/issues.go` | `meResponse.Avatars`, `AvatarsFrom`, `AvatarsIgnored`; `handleSetAvatars` |
| `internal/server/server.go` | route `PUT /api/me/avatars` |
| `web/src/api/board.ts` | `MeResponse.avatars…`, `setAvatars` |
| `web/src/people.ts` (new, or the existing people module if one fits) | `personByLogin(identities, login)` |
| `web/src/components/GbAvatar.vue` (new) | picture or initials, `small`, `dark` |
| `AppHeader.vue`, `CommentCard.vue`, `IssueComposer.vue`, `IssueHead.vue`, `FieldEditor.vue`, `FieldPanel.vue`, `PeopleButton.vue`, `InboxPage.vue`, `BranchFace.vue` | use `GbAvatar`; the three `nameOf` use `personByLogin` |
| `web/src/pages/SettingsPage.vue` | Settings › You: "Pictures" |
| `docs/guide/configuration.md`, `docs/implementation-plan.md` | `board.avatars`; a status line |

`ReleasePage.vue`'s release comments carry an author string, not a person: left with initials (no login to look up).

---

### Task 1: `internal/avatar`, the rules

**Files:** create `internal/avatar/avatar.go`, `internal/avatar/avatar_test.go`.

**Interfaces:**
- Produces: `type Mode string` with `Forge`, `Gravatar`, `None`; `ParseMode(s string) (m Mode, ignored string)` (`""` → `Forge`; unknown → `Forge`, ignored = s); `URL(p people.Person, m Mode) string` (`""`: initials).

- [ ] **Step 1: Write the test first** (`t.Parallel()`), cases:
  - `None`, a person with a github.com account and an email → `""`.
  - `Forge`, account `{Server: "github.com", ID: "17847282"}` → `https://avatars.githubusercontent.com/u/17847282?s=88`.
  - `Forge`, only an email → `""` (Gravatar is opt-in).
  - `Gravatar`, only an email `" Ann@EPFL.ch "` → `https://gravatar.com/avatar/<sha256("ann@epfl.ch")>?s=88&d=404`; same URL for `"ann@epfl.ch"`.
  - `Gravatar`, a github.com account and an email → the GitHub URL (the forge comes first).
  - accounts `{gitlab.com, 5}`, `{github.com, 9}` → GitHub's, id 9; two github.com accounts → the first in `Accounts` order.
  - `{Server: "github.example.org"}` (GHE), `{Server: "gitlab.com"}` alone → `""`.
  - a non-numeric GitHub ID (the test store's `ID: login`, `storetest.go:211`) → `""`, never a URL with a login in it.
  - `ParseMode`: `""`, `"forge"`, `" Gravatar "`, `"none"`, `"faces"` (ignored).
- [ ] **Step 2: Implement.** `strings.TrimSpace` + `strings.ToLower` on the email, `sha256.Sum256`, `hex.EncodeToString`; the GitHub id checked with `strconv.ParseUint`. Doc comment says the order and why Gravatar is opt-in (link the PRD).
- [ ] **Step 3:** `go test ./internal/avatar/`. Commit `feat(avatars): the rules, in internal/avatar`.

### Task 2: the setting and `personOf`

**Files:** modify `internal/store/store.go`, `internal/store/thread.go`, `internal/store/identities.go`; tests beside them.

**Interfaces:**
- Produces: `(*Backend) AvatarsSetting() (mode avatar.Mode, scope, ignored string, err error)`; `(*Backend) SetAvatars(m avatar.Mode) error` (`--local`; `""`/`Forge` unsets it there, as `SetView("")`); `personOf(id string, p *people.Person, m avatar.Mode) Person`.
- Consumes: Task 1.

- [ ] **Step 1: Test** `TestAvatarsSetting` (serial: it runs `git config` in a throwaway repo, as the `board.view` tests do; copy their setup): unset → `Forge`, scope `""`; `SetAvatars(Gravatar)` → `Gravatar`, `local`; `SetAvatars(Forge)` → unset again; a hand-written `board.avatars=faces` → `Forge`, ignored `faces`.
- [ ] **Step 2: Test** that a thread's author and `Identities()` carry `AvatarURL` under `Forge` for a person with a numeric github.com account, and none under `None`. The test store's accounts use the login as ID (`storetest.go:211`): add a helper that adds a numeric-id account rather than changing that line (other tests depend on it).
- [ ] **Step 3: Implement.** `AvatarsSetting` is `ViewSetting`'s twin (`git config --show-scope --get board.avatars`). The `personOf` call sites in `thread.go` (the thread's `who`, `Me`) and `identities.go` (`Identities`, `NewIdentity`) get the mode from one `b.avatarMode()`, read once per `Backend` call that builds persons, not once per person; the two in `releases.go` use only `.Name` and pass `avatar.None`. `ponytail:` the setting is read per call (one `git config`); if profiling shows it, cache it on the Backend and drop the cache in `SetAvatars`.
- [ ] **Step 4:** `go test ./internal/store/ -run 'Avatar|Identities|Thread'`. Commit `feat(avatars): board.avatars, and persons carry their picture`.

### Task 3: `/api/me` and `PUT /api/me/avatars`

**Files:** modify `internal/server/issues.go`, `internal/server/server.go`; test in `internal/server/server_test.go` (or the file the `/api/me/view` tests live in).

**Interfaces:**
- Produces: `meResponse` gains `Avatars string json:"avatars"`, `AvatarsScope string json:"avatars_scope,omitempty"`, `AvatarsIgnored string json:"avatars_ignored,omitempty"`; `PUT /api/me/avatars` `{"avatars": "forge"|"gravatar"|"none"|""}` → `writeMe`; `400` for anything else.
- Consumes: Task 2.

- [ ] **Step 1: Tests**: `TestMeAvatars` (the field and its scope); `TestSetAvatars` (each value; `"faces"` → 400, the config untouched); `TestMeAvatarsNone` (no `avatar_url` in `/api/me` nor in `GET /api/issues/{id}`); `TestMeNeverSendsEmail` (create an identity with an email via `POST /api/me`, then assert the raw JSON of `/api/me` and of an issue never contains the address, under all three modes).
- [ ] **Step 2: Implement**, `handleSetView`'s twin.
- [ ] **Step 3:** `go test ./internal/server/ -run 'Avatar|NeverSendsEmail'`. Commit `feat(avatars): /api/me says and sets board.avatars`.

### Task 4: `GbAvatar` and `personByLogin`

**Files:** create `web/src/components/GbAvatar.vue`, `web/src/people.ts` (+ `people.test.ts`, added to `package.json`'s `test` list); modify `web/src/api/board.ts`.

**Interfaces:**
- Produces: `<GbAvatar :name :src? small? dark? />`; `personByLogin(identities: Person[] | null | undefined, login: string): Person | undefined` (case-insensitive, a leading `@` ignored: today's `nameOf` rule); `setAvatars(m)`; `MeResponse.avatars`, `avatars_scope?`, `avatars_ignored?`.

- [ ] **Step 1: Unit test** `personByLogin`: `@Alice` finds `alice`; unknown → `undefined`; `null` identities → `undefined`.
- [ ] **Step 2: `GbAvatar.vue`**: when `src` and not failed, `<img class="gb-avatar …" :src alt="" aria-hidden="true" referrerpolicy="no-referrer" loading="lazy" decoding="async" @error="failed = true">`; else today's `<span class="gb-avatar …">{{ initials(name) }}</span>`. `failed` resets when `src` changes. CSS in `gitboard.css` beside `.gb-avatar`: `img.gb-avatar { object-fit: cover; }` (same box, so no shift). `referrerpolicy="no-referrer"`: the third party isn't told which git-board page asked.
- [ ] **Step 3:** `npm test`, `npm run typecheck`. Commit `feat(avatars): GbAvatar, a picture or the initials`.

### Task 5: every avatar on the page

**Files:** the components in the File Structure table.

- [ ] **Step 1:** Replace each `<span class="gb-avatar…">{{ initials(…) }}</span>` and `CommentCard`'s `<img>`/`<span>` pair with `GbAvatar`, passing `src` from the person (`event.author.avatar_url`, `me.me.avatar_url`, `r.person.avatar_url`, `p.avatar_url`) or, for a login, `personByLogin(me.value?.identities, v)?.avatar_url`. The three `nameOf`s become `personByLogin(…)?.name ?? login`.
- [ ] **Step 2: e2e** in `web/e2e/` (the e2e server has no network): `avatar falls back to initials`: an identity whose `avatar_url` can't load still shows its initials in the header button and the composer, with no `img` left visible. Then a route-intercepted variant (`page.route('https://avatars.githubusercontent.com/**', …)` fulfilling a 1×1 PNG) shows the `img`.
- [ ] **Step 3:** `npm run typecheck`, the e2e file alone. Commit `feat(avatars): pictures in the header, comments, fields and inbox`.

### Task 6: Settings › You, and the docs

**Files:** `web/src/pages/SettingsPage.vue`, `docs/guide/configuration.md`, `docs/implementation-plan.md`.

- [ ] **Step 1:** In the "Appearance" section (`#set-look`, beside the theme), a "Pictures" radio group, built like "Your issue page" (`viewChoice`'s twin): Forge accounts (default) · Forge accounts, then Gravatar · Initials only. Each option's hint says what it sends where (the PRD's table, one line each). Then a note on `board.avatars` and `--global`, as the view's note does. After `setAvatars`, `me` is replaced (the header updates) and the open issue reloads if one is open.
- [ ] **Step 2:** `configuration.md`: `board.avatars`, beside `board.view`, with the three git config lines. `implementation-plan.md`: a dated status line.
- [ ] **Step 3:** `make ci`, once. Commit `feat(avatars): Settings › You chooses the pictures; docs`.

---

## Not in this plan (PRD "Later")

Storing each forge's own `avatar_url` at pull (GitLab, GHE, EMU), and a server-side image proxy with a cache. Each is its own PRD if wanted.
