import { fieldTypes } from '@/fieldtypes';
import type { Facet } from '@/facets';
// Typed client for the git-board server's API (internal/server). All HTTP
// for the board page goes through here — never fetch() from a component
// directly.

// Same origin in production (frontend is embedded in the Go binary); the
// backend's default dev port otherwise (see git-board serve --addr and
// web/README.md for how to point elsewhere).
export const API_BASE = import.meta.env.DEV ? 'http://localhost:8080' : '';

// clientId names this page in the writes it makes, so the live updates it
// gets back (live.ts) can tell its own changes from someone else's.
export const clientId = Math.random().toString(36).slice(2) + Date.now().toString(36);

export interface Issue {
  id: string;
  title: string;
  shown_title: string; // the title without a trailing (#2518) of its own (gitbugio.Issue.ShownTitle)
  status: string;
  labels: string[];
  inherited?: string[]; // what it takes from the project's [roles], for the roles it doesn't set
  github_url?: string; // absent when the issue wasn't bridged in from GitHub
  forges?: Forge[]; // where it is on forges
  number?: string; // its project number, CO2-3075; absent until it has one
  counted?: boolean; // the project numbers its issues: a bare number is a project number
  updated_at?: string;
  created_at?: string;
}

export interface Bucket {
  group: string;
  issues: Issue[];
}

export interface ViewConfig {
  layout: string;
  filter: string;
  group: string;
  sort: string;
  columns?: string[] | undefined; // a table's columns, in order (tablecols.ts); none: the default
  lanes?: string[] | undefined; // a board's lanes, in order (lanes.ts); none: every value
  limits?: Record<string, number> | undefined; // a lane's limit (work in progress)
}

export interface FieldConfig {
  single: boolean;
  options: string[] | null;
  numeric: boolean;
  current?: string; // an iteration field: today's sprint
  type?: string; // select, iteration, person, number, date, issue, text (config.TypeOf)
  unit?: string; // a number's: h
  inherits?: string; // a role's project value ([roles]), shown when the issue sets none
  people?: string[]; // a person field: who to offer, the identities and who the forges can assign (never options)
}

// ApiError carries the server's status and code: syncing (a sync with
// the forges runs: retry after it), changed (edited meanwhile).
export class ApiError extends Error {
  constructor(message: string, readonly status: number, readonly code: string) {
    super(message);
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  // The server only takes JSON writes (withGuard in internal/server).
  if (init?.method && init.method !== 'GET') {
    init = { ...init, headers: { 'Content-Type': 'application/json', 'X-Git-Board-Client': clientId, ...init.headers }, body: init.body ?? '{}' };
  }
  const res = await fetch(API_BASE + path, init).catch(() => {
    throw new Error('The git-board server is not reachable.');
  });
  if (!res.ok) {
    // The server answers errors as {"error": "...", "code"?: "..."}
    // (writeError, writeCoded in internal/server); surface that
    // message, e.g. "@me" with no --me set.
    const body = await res.text();
    let message = body;
    let code = '';
    try {
      const parsed = JSON.parse(body) as { error?: string; code?: string };
      message = parsed.error ?? body;
      code = parsed.code ?? '';
    } catch {
      // not JSON: keep the raw body
    }
    throw new ApiError(message || `${init?.method ?? 'GET'} ${path} failed (${res.status})`, res.status, code);
  }
  return res.json() as Promise<T>;
}

export function getViews(): Promise<Record<string, ViewConfig>> {
  return request('/api/views');
}

export async function getFields(): Promise<Record<string, FieldConfig>> {
  const fields = await request<Record<string, FieldConfig>>('/api/config/fields');
  fieldTypes.value = Object.fromEntries(Object.entries(fields).map(([k, f]) => [k, f.type ?? 'select']));
  return fields;
}

// Each field value's color name (field → value → grey, blue, …).
export function getColors(): Promise<Record<string, Record<string, string>>> {
  return request('/api/colors');
}

// Unsorted: the page sorts (facets.sortIssues), by option order where the
// server would sort alphabetically. filter: the board's, ⌘K's, a view's —
// one syntax (docs/configuration.md, Filters); "" is every issue.
export function getIssues(filter: string, group = ''): Promise<Bucket[]> {
  const params = new URLSearchParams({ filter });
  if (group) params.set('group', group);
  return request(`/api/issues?${params}`);
}

// The filters panel for a filter, counted on the server (board.Facets).
export async function getFacets(filter: string): Promise<Facet[]> {
  const facets = await request<{ key: string; options: { value: string; count: number }[]; selected: string[]; rest: string }[]>(
    `/api/facets?${new URLSearchParams({ filter })}`,
  );
  return facets.map((f) => ({ ...f, options: f.options.map((o): [string, number] => [o.value, o.count]) }));
}

// Every field's values on every issue, what the filter boxes suggest
// (viewfilter.suggest).
// ponytail: asked once per page load; a new value shows after a reload.
let values: Promise<Record<string, string[]>> | undefined;
export function filterValues(): Promise<Record<string, string[]>> {
  values ??= getFacets('')
    .then((fs) => Object.fromEntries(fs.map((f) => [f.key, f.options.map(([v]) => v)])))
    .catch(() => {
      values = undefined;
      return {};
    });
  return values;
}

export function patchIssue(id: string, changes: Record<string, string>): Promise<Issue> {
  return request(`/api/issues/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(changes),
  });
}

// The GitHub Project sync (internal/server/sync.go). Only check, apply
// and resolve read the Project; getSync returns the last check.
export interface Conflict {
  bug_id: string;
  number: number;
  url: string;
  title: string;
  field: string;
  base: string;
  has_base: boolean;
  local: string; // git-board's value, "" when unset
  project: string; // the Project's
}

export interface SyncReport {
  checked_at: string;
  conflicts: Conflict[];
  label_changes: number;
  project_writes: number;
  warnings: number;
  unmatched: number;
}

export interface SyncStatus {
  project: string; // "owner/number", "" when none is configured
  write: boolean;
  last: SyncReport | null;
}

export function getSync(): Promise<SyncStatus> {
  return request('/api/sync');
}

export function checkSync(): Promise<SyncStatus> {
  return request('/api/sync/check', { method: 'POST' });
}

export function applySync(): Promise<SyncStatus> {
  return request('/api/sync/apply', { method: 'POST' });
}

export function resolveConflict(number: number, field: string, keep: 'git-board' | 'project'): Promise<SyncStatus> {
  return request('/api/sync/resolve', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ number, field, keep }),
  });
}

// Issue management (internal/server/issues.go): every write answers
// with the issue's whole history, re-read from git-bug.
export interface Person {
  id: string;
  name: string;
  login?: string;
  avatar_url?: string;
}

export interface Forge {
  name: string; // GitHub, GitLab, or the host
  url: string;
  number?: string;
  bridge?: string; // in a thread: the sync bridge whose project it is
  gone?: 'unlinked' | 'deleted'; // not synced anymore
}

export interface IssueEvent {
  kind: 'created' | 'comment' | 'labels' | 'status' | 'title' | 'unknown';
  id: string; // created, comment: what editComment takes
  author: Person;
  at: string;
  body?: string; // Markdown
  html?: string; // rendered and cleaned by the server
  edited?: boolean;
  history?: { author: Person; at: string; body: string }[];
  pending?: string[]; // forges the next sync hasn't taken this change to yet
  edit_pending?: string[]; // a comment there already: forges its latest edit hasn't reached
  edit_was?: string; // with edit_pending: the text the forges have
  added?: string[];
  removed?: string[];
  status?: 'open' | 'closed';
  title?: string;
  was?: string;
  meta?: Record<string, string>; // cycle:step, plan:approve, verify:run, verify:criterion, signoff:status…
  review?: Review; // comment: a line comment, a reply or a review
}

// A comment's review:* metadata (internal/gitbugio/review.go), mirrored to
// the pull request by the sync.
export interface Review {
  path?: string;
  line?: number; // at commit
  side?: 'old' | 'new';
  commit?: string;
  pr?: string; // github.com/o/r#26
  reply_to?: string;
  state?: 'APPROVE' | 'REQUEST_CHANGES' | 'COMMENT';
  resolved?: boolean;
  // by the server: the issue's branch whose head has commit, and from it
  branch?: string;
  outdated?: boolean;
  head_line?: number;
  text?: string; // the line at commit
}

export interface Thread extends Issue {
  author: Person;
  created_at: string;
  edited_at: string;
  participants: Person[] | null;
  events: IssueEvent[];
}

const json = (method: string, body: unknown): RequestInit => ({ method, body: JSON.stringify(body) });
const issuePath = (id: string) => `/api/issues/${encodeURIComponent(id)}`;

export function getThread(id: string): Promise<Thread> {
  return request(`${issuePath(id)}/thread`);
}

export function createIssue(title: string, body: string, labels: string[]): Promise<Thread> {
  return request('/api/issues', json('POST', { title, body, labels }));
}

// A comment, and/or a status: "Close with comment".
// step: the cycle step whose discussion the comment is in (0: none, step 1's)
export function addComment(id: string, body: string, status?: 'open' | 'closed', step = 0): Promise<Thread> {
  return request(`${issuePath(id)}/comments`, json('POST', { body, status: status ?? '', step }));
}

// base is the text the edit started from: 409 if the comment changed since.
export function editComment(id: string, commentId: string, body: string, base: string): Promise<Thread> {
  return request(`${issuePath(id)}/comments/${encodeURIComponent(commentId)}`, json('PUT', { body, base }));
}

export function setTitle(id: string, title: string): Promise<Thread> {
  return request(`${issuePath(id)}/title`, json('PUT', { title }));
}

export function setStatus(id: string, status: 'open' | 'closed'): Promise<Thread> {
  return request(`${issuePath(id)}/status`, json('PUT', { status }));
}

export function changeLabels(id: string, add: string[], remove: string[]): Promise<Thread> {
  return request(`${issuePath(id)}/labels`, json('POST', { add, remove }));
}

export interface MeResponse {
  me: Person | null;
  identities: Person[] | null;
}

export function getMe(): Promise<MeResponse> {
  return request('/api/me');
}

export function adoptIdentity(id: string): Promise<MeResponse> {
  return request('/api/me', json('POST', { id }));
}

export function createIdentity(name: string, email: string): Promise<MeResponse> {
  return request('/api/me', json('POST', { name, email }));
}

// The sync with the forges (internal/server/forges.go).
// One way of a sync; a dry run of a pull only knows "up to N changes".
export interface SyncDirection {
  issues: number; // created on the forge / imported from it
  changed: number; // linked issues with changes
  comments: number;
  edits: number;
  changes: number; // title, state, labels
}

export interface BridgeTally {
  to: SyncDirection;
  from: SyncDirection;
  linked: number; // found on the forge by their git-bug marker
  stays_local: number; // texts no token can post
  conflicts: number; // linked issues changed on both sides: a send alone would replace the forge's titles, states, comment texts
  assignable: number; // who the forge says can be assigned, asked after the run
  warnings: number;
  errors: number;
  lines: string[];
}

// A run goes both ways (sync), or one: get (git bug bridge pull), send
// (git bug bridge push). A dry run, the Check, looks both ways.
export type ForgeMode = 'sync' | 'get' | 'send';

export interface ForgeJob {
  dry_run: boolean;
  mode: ForgeMode;
  state: 'running' | 'done' | 'failed';
  started: string;
  ended?: string;
  names: string[];
  bridges: Record<string, BridgeTally>;
  errors: number;
  error?: string;
  step?: SyncStep; // where it is, while it runs
}

// Where a run is: the issue a way of a bridge is on, and how many it has
// come to. A pull has no total: the forge doesn't say how many it holds.
export interface SyncStep {
  bridge: string;
  way: 'pull' | 'push';
  done: number;
  total?: number;
  issue?: string; // "#43 import: users lose their work"
}

export interface ForgeStatus {
  bridges: { name: string; target: string }[];
  job: ForgeJob | null;
}

export function getForges(): Promise<ForgeStatus> {
  return request('/api/forges');
}

// A dry run looks at every forge; a real run at bridges (empty: all).
export function startForgeSync(dryRun: boolean, mode: ForgeMode = 'sync', bridges: string[] = []): Promise<ForgeStatus> {
  return request('/api/forges/sync', json('POST', { dry_run: dryRun, mode, bridges }));
}

// The git remotes (internal/server/remotes.go): issues and people
// go with git, as git bug pull and push move them.
export interface Ahead {
  send: number;
  receive: number;
}

export interface Remote {
  name: string;
  url: string; // without credentials
  push_urls: string[]; // where a push goes, every one (pushurls, else urls)
  tracked: boolean; // its refs are known: fetched or pushed once
  issues: Ahead;
  people: Ahead;
  checked?: string; // this server's last fetch, pull or push of it
}

export function getRemotes(): Promise<{ remotes: Remote[] }> {
  return request('/api/remotes');
}

// fetch is the Check: nothing local changes
export function remoteAction(name: string, action: 'fetch' | 'pull' | 'push'): Promise<{ remotes: Remote[] }> {
  return request(`/api/remotes/${encodeURIComponent(name)}/${action}`, { method: 'POST' });
}

// The project's own numbers (internal/server/numbers.go): CO2-3075,
// a counter on origin. Only init and run push: refs/board/seq, to origin.
export interface Numbers {
  on: boolean;
  prefix?: string;
  start?: number;
  next?: number;
  numbered: number; // with a number: their line, or their GitHub one below start
  waiting: number; // on origin without one: run numbers them
  unpushed: number; // only here: their push numbers them
  highest: number; // the highest forge number of an issue here
  github_numbers?: number[]; // before the switch: each issue on origin by its GitHub number, 0 for none
  origin: Remote | null; // where the counter goes
  name: string; // the repository's, for a prefix
  github?: { issues: string; highest: number; error?: string } | undefined; // asked with github: GitHub's answer
  done: number; // a write: how many issues it numbered
  error?: string; // started, but the issues not numbered yet
}

// github asks GitHub's highest issue or pull request number too, before the switch
export function getNumbers(github = false): Promise<Numbers> {
  return request(`/api/numbers${github ? '?github=1' : ''}`);
}

export function startNumbers(prefix: string, start: number): Promise<Numbers> {
  return request('/api/numbers/init', json('POST', { prefix, start }));
}

export function numberIssues(): Promise<Numbers> {
  return request('/api/numbers/run', json('POST', {}));
}

// The inbox (internal/server/inbox.go): what happened that concerns
// you on the issues of the last 14 days, newest first.
export interface InboxRow {
  issue: Issue;
  reasons: string[]; // why you follow it; none: addressed to you only
  news: string[]; // "2 comments", "closed", "added you as reviewer"…
  who: Person[];
  latest: string;
  first: string; // the oldest of those events: the panel scrolls there
}

export interface InboxResponse {
  me: { id: string; login: string };
  rows: InboxRow[];
  fields: boolean; // also field changes on issues you're in
  views: string[]; // views followed
  views_missing: string[]; // followed, no longer in the config
  following: number; // open issues your inbox follows
  roles: string[]; // roles you hold for the whole project: its new and closed issues
}

export interface FollowState {
  following: boolean;
  reasons: string[];
  set: boolean | null; // what you clicked; null: automatic
}

export function getInbox(): Promise<InboxResponse> {
  return request('/api/inbox');
}

export function markSeen(issues: string[]): Promise<object> {
  return request('/api/inbox/seen', json('POST', { issues }));
}

const followPath = (id: string) => `/api/inbox/follow/${encodeURIComponent(id)}`;

export function getFollow(id: string): Promise<FollowState> {
  return request(followPath(id));
}

export function setFollow(id: string, follow: boolean | null): Promise<FollowState> {
  return request(followPath(id), json('PUT', { follow }));
}

export function setInboxSettings(views: string[], fields: boolean): Promise<object> {
  return request('/api/inbox/settings', json('PUT', { views, fields }));
}

// Where the page loads a server path from (the API's origin in dev).
export const apiURL = (path: string) => API_BASE + path;

// renderMarkdown is the composer's preview, rendered as a comment would be.
export function renderMarkdown(body: string): Promise<{ html: string }> {
  return request('/api/markdown', json('POST', { body }));
}

// Attachments (internal/server/uploads.go): on S3, served by a CDN to
// git-board and the forges alike.
export interface UploadStatus {
  enabled: boolean;
  max_bytes: number;
  public_url?: string;
}

export function getUploadStatus(): Promise<UploadStatus> {
  return request('/api/uploads');
}

// uploadFile sends one file; the answer's markdown shows it (an image) or
// links it. Not JSON: the page's header stands in (withGuard).
export async function uploadFile(file: File): Promise<{ url: string; name: string; markdown: string }> {
  const body = new FormData();
  body.append('file', file);
  const res = await fetch(API_BASE + '/api/uploads', { method: 'POST', body, headers: { 'X-Git-Board-Upload': '1' } }).catch(() => {
    throw new Error('The git-board server is not reachable.');
  });
  const data = (await res.json().catch(() => ({}))) as { error?: string; url: string; name: string; markdown: string };
  if (!res.ok) throw new ApiError(data.error ?? `upload failed (${res.status})`, res.status, '');
  return data;
}

// Settings (internal/server/settings.go).
export interface Bridge {
  name: string;
  target: string;
  where: string;
  sync: boolean;
  can_sync: boolean;
}

export interface Settings {
  bridges: Bridge[];
  uploads: { enabled: boolean; endpoint?: string; public_url?: string; prefix?: string };
  github_project: { owner: string; number: number; write?: boolean };
  config_path: string;
  cycle: CycleStages;
}

export function getSettings(): Promise<Settings> {
  return request('/api/settings');
}

export function setBridgeSync(name: string, sync: boolean): Promise<{ sync: boolean }> {
  return request(`/api/bridges/${encodeURIComponent(name)}/sync`, json('PUT', { sync }));
}

// .git-board.toml, edited from the settings page (configfile.go).
export interface ConfigFile {
  path: string;
  text: string;
  exists: boolean;
  hash: string;
  default?: string; // the default file, when there is none
}

export interface ConfigCheck {
  ok: boolean;
  error?: { line: number; col: number; message: string };
  message?: string;
  warnings: string[];
  summary: { fields: string[] | null; views: string[] | null; sprints: number };
}

export function getConfigFile(): Promise<ConfigFile> {
  return request('/api/config');
}

export function checkConfig(text: string): Promise<ConfigCheck> {
  return request('/api/config/check', json('POST', { text }));
}

export function saveConfig(text: string, base: string): Promise<ConfigCheck> {
  return request('/api/config', json('PUT', { text, base }));
}

// ReleaseSetup is what Enable binary release proposes from the repository.
export interface ReleaseSetup {
  binaries: string[]; // main packages, ./cmd/git-board
  platforms: string[]; // os/arch
  build: string; // a command that fills dist/ instead of the Go builds, "make dist"; ""
  remote: string; // the authority remote
  forge: string; // github; "" when the remote isn't one git-board publishes to
  repo: string; // owner/name on the forge
  note: string; // why it can't enable; "" when it can
}

export function getReleaseSetup(): Promise<ReleaseSetup> {
  return request('/api/releases/setup');
}

export function enableRelease(s: Pick<ReleaseSetup, 'binaries' | 'platforms' | 'build' | 'remote'>): Promise<void> {
  return request('/api/releases/setup', json('POST', s));
}

// docs/configuration.md, a section per ## heading, rendered.
export interface DocSection {
  id: string;
  title: string;
  html: string;
}

export function getConfigDocs(): Promise<DocSection[]> {
  return request('/api/config/docs');
}

// docs/forges.md, in sections: how forges work
export function getForgeDocs(): Promise<DocSection[]> {
  return request('/api/forges/docs');
}

// saveView writes [views.<name>] into .git-board.toml (the board's "Save
// view"); the answer is every view, as GET /api/views.
export function saveView(name: string, v: ViewConfig): Promise<Record<string, ViewConfig>> {
  return request(`/api/views/${encodeURIComponent(name)}`, json('PUT', v));
}

// An issue's links through its issue fields (refs.go): out, the issues it
// names (parent, blocked-by…); in, the issues naming it.
export interface IssueRef {
  value: string;
  id?: string;
  title?: string;
  status?: string;
}
export interface IssueRefs {
  out: Record<string, IssueRef[]>;
  in: Record<string, IssueRef[]>;
}

export function getRefs(id: string): Promise<IssueRefs> {
  return request(`${issuePath(id)}/refs`);
}

// An issue's code (internal/branches): the branch to create, from start
// (origin/<base>), and the branches whose name names the issue.
export interface IssueBranch {
  name: string;
  remote?: string;
  ahead: number;
  behind: number;
  merged: boolean;
  in_base: boolean; // its changes are in the base: merged, or squashed or rebased in
  last_commit: { sha: string; subject: string; author: string; date: string };
}
export interface IssueBranches {
  suggested: string;
  base: string;
  start: string;
  command: string;
  branches: IssueBranch[];
}

export function getBranches(id: string): Promise<IssueBranches> {
  return request(`${issuePath(id)}/branches`);
}

// The PR gate (internal/server/pulls.go, internal/ghpr): each branch
// on origin and its GitHub pull request, found by the branch's name. Open
// and Merge act on GitHub as `login`, with that person's token.
export interface GateCheck {
  name: string;
  state: 'success' | 'failure' | 'pending' | 'expected'; // expected: required, never reported
  required: boolean;
  url?: string;
}
export interface GateReview {
  login: string;
  state: string; // APPROVED, CHANGES_REQUESTED, DISMISSED
  commit: string;
}
export interface GatePR {
  number: number;
  url: string;
  state: 'OPEN' | 'MERGED' | 'CLOSED';
  draft: boolean;
  title: string;
  base: string;
  head: string; // its head commit
  merge_commit?: string; // when merged
  author: string;
  decision: string; // APPROVED, REVIEW_REQUIRED, CHANGES_REQUESTED, '' (no review rule)
  mergeable: string;
  merge_state: string;
  checks: GateCheck[];
  reviews: GateReview[];
  requested: string[];
  can_merge: boolean;
  checking: boolean; // GitHub is still computing mergeability
  waits: string[];
}
export interface IssuePulls {
  repo: string; // '' when origin isn't on GitHub
  base: string;
  login: string;
  pulls: { branch: string; pr: GatePR | null }[];
}

// The issue's step in the development cycle (internal/cycle,
// internal/server/cycle.go), computed from the status and the evidence.
export interface CycleWaits {
  role?: string; // risk, plan, review, assignee, author, signoff
  login?: string; // '' with anyone
  what: string;
  anyone: boolean;
}
export interface CyclePlan {
  path: string;
  ref: string;
  status?: string; // the front matter's: draft, accepted, in-progress, delivered
  issue?: string; // the front matter's issue: line, what found the file
  html?: string; // the file rendered, below its front matter
  by?: string; // who first committed it on the plan branch
}
export interface CycleSignoff {
  status: string;
  by: string;
  commit?: string;
  why?: string;
  at: string;
}
// What the server gathered (internal/cycle Input): the thread's fields and
// comments, the branches, the PRs.
export interface CycleEvidence {
  kind?: string; // [branches]' kind, for step 4's rule
  criteria?: string[];
  criteria_problems?: string[]; // where the description reads differently from how it looks
  risk?: string;
  author?: string;
  assignees?: string[];
  people?: Record<string, string>; // the login holding each gate: risk, plan, review, signoff:<status>
  branch?: string;
  head?: string;
  base?: string;
  plan_branch?: string;
  plan_head?: string;
  plan?: CyclePlan;
  plan_approval?: { by: string; commit?: string; at: string };
  plan_pr?: GatePR | null;
  tests?: { path: string; commit?: string }[];
  pr?: GatePR | null;
  signoffs?: CycleSignoff[];
  runs?: { by: string; commit?: string; at: string }[];
  checks?: { index: number; by: string; commit?: string; at: string }[];
}
export interface Cycle {
  step: number; // 1 to 7
  of: number;
  name: string;
  stage: string;
  done: boolean;
  gate: string;
  risk?: string;
  skipped?: number[];
  note?: string; // evidence that disagrees with the status
  waits: CycleWaits;
  mine: boolean; // the viewer's turn, or anyone's
  actions?: string[]; // set-risk, approve-plan, review, verify, check, signoff, move-ready, start, ready-for-review, to-validation
  evidence: CycleEvidence;
  stages: CycleStages;
  pr_checked_at?: string;
  ai?: boolean; // an AI address is set: step 2 can ask it for the class
}

// the cycle's stages: the project file's [cycle], the rest guessed from the status names
export interface CycleStages {
  before?: string[];
  ready: string;
  in_progress: string[];
  in_review: string[];
  validation: string[];
  closing: string;
  plans?: string;
  criteria: string[]; // the headings the criteria are read under
  kinds: Record<string, string>; // step 4's rule by kind: failing-test, per-criterion, none
  guessed: boolean;
  roles?: Record<string, string>; // the role holding each gate: risk, plan, review, signoff:<status>
}

// The Work page (internal/work, internal/server/work.go): every open
// issue's cycle, grouped into what needs the viewer and what waits on others.
export interface WorkRow {
  issue: Issue;
  step: number;
  of: number;
  name: string;
  risk?: string;
  done: boolean;
  waits: CycleWaits;
  mine: boolean;
  actions?: string[];
  since: string; // when the gate started waiting
  involved: boolean;
  question?: { id: string; body: string; by: Person; at: string };
  branch?: string;
}

export interface WorkAgent {
  path: string;
  branch: string;
  issue?: Issue;
  step?: number;
  name?: string;
  state: string; // "Waits for you", "Waits for bob to review the PR", "No issue"
  at: string; // the last commit
  session?: WtxSession; // wtx status on its branch
}

export interface SendLine {
  issue: Issue;
  what: string; // "a comment", "the status", "the risk", "the title", "closed", "reopened", "a new issue"
  at: string;
}

export interface WorkResponse {
  me: { id: string; login: string };
  needs: WorkRow[];
  waiting: WorkRow[];
  agents: WorkAgent[];
  to_send: SendLine[]; // newest first, at most 20
  pending: number; // every change the forges wait for
  retro: { sprint?: string; since?: string; corrections: number };
  can_start: boolean; // wtx may start agents from this server
  reason?: string; // why not
}

export function getWork(who: 'me' | 'team'): Promise<WorkResponse> {
  return request(`/api/work?who=${who}`);
}

export function getCycle(id: string): Promise<Cycle> {
  return request(`${issuePath(id)}/cycle`);
}

// approve-plan, verify {output, commit}, check {index}, signoff {why}.
export function cycleAction(id: string, action: string, body: Record<string, unknown> = {}): Promise<Cycle> {
  return request(`${issuePath(id)}/cycle/${action}`, json('POST', body));
}

// Step 4's suites and checks (internal/server/checks.go): the base
// branch's [cycle.tests.*] and [cycle.checks.*], each result a signed
// line in git notes (internal/proof, docs/adr/0001).
export interface Suite {
  run: string[];
  files?: string[];
  where?: string; // "worktree" (default) or "ci": the forge's, never run here
  dir?: string;
  env?: Record<string, string>;
}

export interface CheckResult {
  check: string;
  verdict: 'PASSED' | 'FAILED';
  commit: string;
  at: string;
  dur: number; // nanoseconds
  log: string; // sha256:<hex>, kept by the clone that ran it
}

export interface CheckStatus {
  name: string;
  key: string;
  verdict?: 'PASSED' | 'FAILED'; // none: no line verifies at this key
  proven: boolean;
  by?: string; // the writer, a clone
  signer?: string;
  result?: CheckResult;
  unverified?: number;
}

export interface SuiteRow extends Suite {
  name: string;
  touched: string[] | null;
  head?: CheckStatus;
  at_test?: CheckStatus;
}

export interface StepTests {
  rule: string; // failing-test, per-criterion, none
  base: string;
  head?: string;
  test_commit?: string;
  suites: SuiteRow[];
  checks: SuiteRow[];
  gate: { proven: boolean; reason?: string; checks: CheckStatus[] };
  running: string[];
  error?: string; // the last run's: a check that couldn't start
}

export function getStepTests(id: string): Promise<StepTests> {
  return request(`${issuePath(id)}/tests`);
}

// names pick entries of the base's toml: nothing else reaches a command line
export function runSuites(id: string, names: string[], target: 'head' | 'test'): Promise<{ running: string[]; commit: string }> {
  return request(`${issuePath(id)}/tests/run`, json('POST', { names, target }));
}

export async function getRunLog(id: string, log: string): Promise<string> {
  const res = await fetch(`${API_BASE}${issuePath(id)}/tests/log/${encodeURIComponent(log.replace(/^sha256:/, ''))}`);
  if (!res.ok) throw new Error(res.status === 404 ? 'This clone has no log of that run: another clone ran it.' : `The log: HTTP ${res.status}`);
  return res.text();
}

export interface ChecksSetup {
  source: string; // lefthook.yml, Makefile, a package.json; "" for none
  checks: Record<string, Suite>;
  skipped: { name: string; run: string; why: string }[];
  toml: string;
  declared: boolean;
}

export function getChecksSetup(): Promise<ChecksSetup> {
  return request('/api/checks/setup');
}

export function writeChecks(): Promise<ChecksSetup> {
  return request('/api/checks/setup', json('POST', {}));
}

export function getPulls(id: string): Promise<IssuePulls> {
  return request(`${issuePath(id)}/pulls`);
}

export function openPull(id: string, branch: string): Promise<GatePR> {
  return request(`${issuePath(id)}/pulls`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ branch }),
  });
}

export function mergePull(id: string, number: number, sha: string): Promise<void> {
  return request(`${issuePath(id)}/pulls/${number}/merge`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ sha }),
  });
}

// The Changes tab (internal/server/review.go): an issue's branch, from
// where it left the base (base) to its head on origin.
export interface FileStat {
  path: string;
  from?: string; // renamed from
  added: number;
  deleted: number;
  binary?: boolean;
  generated?: boolean; // linguist-generated
  blob: string; // the file at to: what "viewed" remembers
}
export interface Changes {
  branch: string;
  base_name: string;
  base: string;
  head: string;
  from: string;
  to: string;
  commits: { sha: string; subject: string; author: string; date: string }[]; // base..head, newest first
  files: FileStat[]; // from..to
}
export interface ChangeRange {
  branch?: string;
  from?: string;
  to?: string;
}
export interface FileDiff {
  path: string;
  diff: string;
  truncated: boolean;
}

const query = (q: Record<string, string | number | undefined>) =>
  new URLSearchParams(Object.entries(q).filter((e): e is [string, string | number] => !!e[1]).map(([k, v]) => [k, String(v)])).toString();

export function getChanges(id: string, r: ChangeRange): Promise<Changes> {
  return request(`${issuePath(id)}/changes?${query({ ...r })}`);
}

export function getFileDiff(id: string, r: ChangeRange, path: string, fromPath?: string, limit?: number): Promise<FileDiff> {
  return request(`${issuePath(id)}/changes/file?${query({ ...r, path, from_path: fromPath, limit })}`);
}

// A line comment (path, line, side, commit), a reply (reply_to), or a
// review (state, commit); pr once the branch has a pull request.
export interface ReviewRequest {
  body: string;
  path?: string;
  line?: number;
  side?: 'old' | 'new';
  commit?: string;
  pr?: string;
  reply_to?: string;
  state?: Review['state'];
}

export function postReview(id: string, req: ReviewRequest): Promise<Thread> {
  return request(`${issuePath(id)}/review`, json('POST', req));
}

export function resolveReview(id: string, commentId: string, resolved: boolean): Promise<Thread> {
  return request(`${issuePath(id)}/review/${encodeURIComponent(commentId)}/resolve`, json('POST', { resolved }));
}

// Bridges set up from the settings page (settings.go). A credential is
// listed without its token; a pasted token goes to git-bug's store.
export interface Credential {
  id: string;
  target: string;
  host: string;
  login: string;
}

export function getCredentials(): Promise<Credential[]> {
  return request('/api/credentials');
}

export function addBridge(b: { target: string; url: string; name?: string | undefined; credential?: string | undefined; token?: string | undefined; sync: boolean }): Promise<{ name: string }> {
  return request('/api/bridges', json('POST', b));
}

export function removeBridge(name: string): Promise<Record<string, never>> {
  return request(`/api/bridges/${encodeURIComponent(name)}`, json('DELETE', {}));
}

// Insights (internal/server/insights.go): the issues' history
// replayed day by day.
export interface InsightsReport {
  field: string;
  values: string[];
  days: string[];
  all: Record<string, number[]>;
  open: Record<string, number[]>;
  weeks: { start: string; opened: number; closed: number }[];
  cycle: { from: 'started' | 'opened'; start: string; days: number[]; median: number; p85: number };
  burnup?: { sprint: string; title: string; start: string; end: string; days: string[]; scope: number[]; done: number[]; status: Record<string, number> };
  headlines: Record<string, string>;
}

export function getInsights(field: string, days: number, view: string): Promise<InsightsReport> {
  const q = new URLSearchParams({ field, days: String(days) });
  if (view) q.set('view', view);
  return request(`/api/insights?${q}`);
}

export interface SprintWindow {
  slug: string;
  title: string;
  start: string;
  end: string;
}

export function getSprints(): Promise<SprintWindow[]> {
  return request('/api/sprints');
}

// A sprint's report as Markdown (report.go): the current sprint unless one
// is named.
export interface SprintReport {
  sprint: string;
  title: string;
  markdown: string;
}

export function getReport(sprint = '@current'): Promise<SprintReport> {
  return request(`/api/report?sprint=${encodeURIComponent(sprint)}`);
}

// One issue synced with every sync bridge (issuesync.go): a dry run first,
// which a real run needs.
export interface IssueSyncResult {
  dry_run: boolean;
  writes: number; // sent, or would be sent, to the forges
  errors: number;
  bridges: Record<string, BridgeTally>;
}

export function syncIssue(id: string, dryRun: boolean): Promise<IssueSyncResult> {
  return request(`${issuePath(id)}/sync`, json('POST', { dry_run: dryRun }));
}

// One issue on one forge (issuesync.go), nothing written to it: linked to
// an issue already there (its number or URL), unlinked for good, or the
// forge's version taken over what wasn't sent yet.
export function linkIssue(id: string, bridge: string, issue: string): Promise<{ id: string; url: string; number: string }> {
  return request(`${issuePath(id)}/link`, json('POST', { bridge, issue }));
}

export function unlinkIssue(id: string, bridge: string): Promise<Record<string, never>> {
  return request(`${issuePath(id)}/unlink`, json('POST', { bridge }));
}

export function takeIssue(id: string, bridge: string): Promise<IssueSyncResult> {
  return request(`${issuePath(id)}/take`, json('POST', { bridge }));
}

// Releases (internal/server/releases.go, internal/release): what runs where,
// the releases, and the jobs that move them, one at a time in the
// background. Go sends a nil list as null, and {enabled: false} alone
// without [release]: normalized here, so components never see a null list.
// Writes answer 409 releasing while a job runs.
export interface ReleaseVersion {
  major: number;
  minor: number;
  patch: number;
  rc: number; // 0: a final
}

export interface ReleaseTag {
  name: string; // v1.4.20, v1.4.20-rc.2
  commit: string;
  message: string;
  version: ReleaseVersion;
  date: string;
}

// a commit that touched an overlay
export interface OverlayChange {
  commit: string;
  author: string;
  version: string;
  date: string;
}

export interface TargetState {
  name: string; // enack8s; with the registry's host when two targets share one
  repo: string;
  path: string; // the overlay's directory
  deployed: { commit: string; version: string; images: Record<string, { tag: string; digest: string }>; chart: string }; // commit "": not adopted
  since: OverlayChange | null; // the overlay's newest commit
}

export interface EnvState {
  name: string;
  role: 'follow' | 'candidate' | 'last' | ''; // "": before the followed one
  targets: TargetState[];
  commit: string; // the targets' when they agree
  version: string; // the targets' when they agree
  split: boolean; // targets run different commits
  adopted: boolean; // every target has git-board/commit
  behind: string[]; // follow only: subjects of the trunk's merges it doesn't run
}

export interface ReleaseRecord {
  id: string;
  commit: string;
  previous: string; // the previous final's tag
  artifacts: { image: string; registry: string; digest: string }[];
  chart: { version: string; digest: string };
  assets: { name: string; sha256: string }[] | null; // the archives published with the tag, binaries only
  notes: string; // markdown
  comments: { author: string; message: string; date: string }[];
  yanked: string; // the reason, "" when not
  author: string;
  created: string;
}

export interface ReleaseRow {
  tag: ReleaseTag;
  names: string[]; // every release tag on its commit, newest first
  record: ReleaseRecord | null; // null until reconciled
  runsIn: string[]; // environments running its commit
  beforeGitBoard: boolean; // its tag wasn't cut by git-board (no " (git-board)" in its subject)
}

export interface ReleaseStep {
  name: string;
  detail: string;
  state: 'running' | 'done' | 'failed';
}

export interface ReleaseJob {
  kind: 'deploy' | 'cut' | 'promote' | 'rollback';
  subject: string; // dev, v1.0.1-rc.1 to stage, prod to v1.0.0, the rc promoted, a cut's tag
  state: 'running' | 'done' | 'failed';
  error: string;
  code: '' | 'taken' | 'older' | 'not-adopted'; // a failed job's
  tag: string; // the tag a cut or promote pushed or tried, once known; ""
  steps: ReleaseStep[]; // each once, as last reported
  started: string;
  ended: string | null;
}

export interface Releases {
  enabled: boolean;
  remote: string; // the authority remote, [release] remote (origin)
  trunk: string; // the trunk branch (dev)
  environments: EnvState[];
  binaries: string[]; // [release] binaries: a cut builds and publishes their archives
  releases: ReleaseRow[]; // newest first
  job: ReleaseJob | null; // running, or the last one
}

export interface DraftIssue {
  number: string; // CO2-3075, #2518
  title: string;
  kind: string;
  merge: string;
}

export interface Draft {
  from: string; // the followed environment's commit, else the trunk's head
  fromVersion: string;
  undeployed: string[]; // trunk merges after from
  previous: ReleaseTag | null;
  proposed: ReleaseVersion;
  tag: string; // what a cut would push
  issues: DraftIssue[];
  notes: string;
  checks: { ok: boolean; text: string; detail: string }[];
}

export interface ReleaseDetail {
  row: ReleaseRow;
  issues: DraftIssue[];
  candidates: ReleaseRow[]; // the rc rows of its final, but itself
  history: Record<string, (OverlayChange & { target: string })[]>; // environment → its overlays' changes naming this version
}

// the commit an overlay is thought to run, for adopt
export interface Proposal {
  env: string;
  repo: string;
  path: string;
  runs: string;
  commit: string; // "": unknown, to type in
  from: 'tag' | 'label' | '';
}

export interface IssueEnvironment {
  name: string;
  version: string;
  in: boolean;
}

const tagPath = (tag: string) => `/api/releases/${encodeURIComponent(tag)}`;
const envPath = (env: string) => `/api/environments/${encodeURIComponent(env)}`;

const releaseRow = (r: ReleaseRow): ReleaseRow => ({
  ...r,
  names: r.names ?? [],
  runsIn: r.runsIn ?? [],
  record: r.record && { ...r.record, artifacts: r.record.artifacts ?? [], comments: r.record.comments ?? [] },
});
const envState = (e: EnvState): EnvState => ({
  ...e,
  behind: e.behind ?? [],
  targets: (e.targets ?? []).map((t) => ({ ...t, deployed: { ...t.deployed, images: t.deployed.images ?? {} } })),
});
const releaseJob = (j: ReleaseJob | null): ReleaseJob | null => j && { ...j, steps: j.steps ?? [] };

export async function getReleases(): Promise<Releases> {
  const r = await request<Partial<Releases>>('/api/releases');
  return { enabled: !!r.enabled, remote: r.remote ?? '', trunk: r.trunk ?? '', environments: (r.environments ?? []).map(envState), binaries: r.binaries ?? [], releases: (r.releases ?? []).map(releaseRow), job: releaseJob(r.job ?? null) };
}

export async function getDraft(final = false): Promise<Draft> {
  const d = await request<Draft>(`/api/releases/draft${final ? '?final=1' : ''}`);
  return { ...d, undeployed: d.undeployed ?? [], issues: d.issues ?? [], checks: d.checks ?? [] };
}

export async function getReleaseJob(): Promise<ReleaseJob | null> {
  return releaseJob(await request<ReleaseJob | null>('/api/releases/job'));
}

export async function getRelease(tag: string): Promise<ReleaseDetail> {
  const d = await request<ReleaseDetail>(tagPath(tag));
  const history = Object.fromEntries(Object.entries(d.history ?? {}).map(([env, h]) => [env, h ?? []]));
  return { row: releaseRow(d.row), issues: d.issues ?? [], candidates: (d.candidates ?? []).map(releaseRow), history };
}

// version: the final form (1.4.20), "" the draft's; the rc number is the
// server's. commit, notes: "" the draft's
export interface CutRequest {
  version?: string;
  final?: boolean;
  hotfix?: boolean;
  commit?: string;
  notes?: string;
  deploy?: boolean;
}

export async function cutRelease(c: CutRequest): Promise<ReleaseJob> {
  return releaseJob(await request<ReleaseJob>('/api/releases', json('POST', c)))!;
}

export async function promoteRelease(tag: string): Promise<ReleaseJob> {
  return releaseJob(await request<ReleaseJob>(`${tagPath(tag)}/promote`, { method: 'POST' }))!;
}

// tag "": the trunk's head, to the followed environment only
export async function deployEnvironment(env: string, tag = ''): Promise<ReleaseJob> {
  return releaseJob(await request<ReleaseJob>(`${envPath(env)}/deploy`, json('POST', tag ? { tag } : {})))!;
}

export async function rollbackEnvironment(env: string, tag: string, reason: string): Promise<ReleaseJob> {
  return releaseJob(await request<ReleaseJob>(`${envPath(env)}/rollback`, json('POST', { tag, reason })))!;
}

export async function getProposals(): Promise<Proposal[]> {
  return (await request<Proposal[] | null>('/api/releases/adopt')) ?? [];
}

export function adoptOverlays(proposals: Proposal[]): Promise<object> {
  return request('/api/releases/adopt', json('POST', { proposals }));
}

export function setReleaseNotes(tag: string, notes: string): Promise<object> {
  return request(`${tagPath(tag)}/notes`, json('PUT', { notes }));
}

export function commentRelease(tag: string, message: string): Promise<object> {
  return request(`${tagPath(tag)}/comments`, json('POST', { message }));
}

export function yankRelease(tag: string, reason: string): Promise<object> {
  return request(`${tagPath(tag)}/yank`, json('POST', { reason }));
}

export async function getIssueEnvironments(id: string): Promise<IssueEnvironment[]> {
  return (await request<IssueEnvironment[] | null>(`${issuePath(id)}/environments`)) ?? [];
}

// Worktrees (internal/server/worktrees.go): wtx, on the machine where
// git board serve runs.
export interface WtxSession { branch: string; path: string; main: boolean; session: string; live: boolean; state: string; agent: string; llm: string }
export interface WtxCheck { name: string; ok: boolean; detail: string; fix: string; hard: boolean }
// WtxAnswers is wtx init --edit's JSON: what wtx read, and wtx.toml's values.
export interface WtxAnswers {
  repo: { name: string; base_branch: string; protected_branches: string[] };
  ports?: { family?: { name: string; main: number }[] };
  panes?: { pane?: { name: string; role?: string; cmd?: string }[] };
  agent: { tool: string; llm?: string; orchestration?: { enabled: boolean; plan_model: string; build_model: string } };
  permissions: { allow?: string[]; deny: string[] };
  _changes?: unknown[];
  [k: string]: unknown;
}
export interface Worktrees { local: boolean; wtx: boolean; reason?: string; code?: string; doctor?: { ok: boolean; checks: WtxCheck[] }; config: boolean; answers?: WtxAnswers }
export interface WtxFiles { files: { path: string; text: string }[] }
export interface IssueAgent { reason?: string; code?: string; branch: string; worktree?: { path: string; head: string; branch?: string }; session?: WtxSession; output?: string }

export function getWorktrees(): Promise<Worktrees> {
  return request('/api/worktrees');
}

export function previewWtx(answers: WtxAnswers): Promise<WtxFiles> {
  return request('/api/worktrees/preview', json('POST', { answers }));
}

export function writeWtx(answers: WtxAnswers): Promise<WtxFiles> {
  return request('/api/worktrees/init', json('POST', { answers }));
}

export function getIssueAgent(id: string): Promise<IssueAgent> {
  return request(`${issuePath(id)}/agent`);
}

// comment: a line comment's id, its thread quoted in the brief
export function agentAction(id: string, action: 'go' | 'brief' | 'approve', brief: string, comment?: string): Promise<IssueAgent> {
  return request(`${issuePath(id)}/agent/${action}`, json('POST', { brief, ...(comment && { comment }) }));
}

// Settings › Labels (internal/server/labels.go): the project's labels, and
// each GitHub repository's, through gh on the machine where git board serve
// runs. A color is hex without #, as GitHub has it.
export interface GithubLabel { name: string; color: string; description: string }
export interface RepoError { repo: string; message: string }
export interface LabelRow extends GithubLabel {
  field: string;
  value: string;
  own_color: boolean; // the project file sets it, else the board's own
  own_description: boolean;
  count: number; // open issues
  github: { repo: string; state: 'same' | 'color' | 'description' | 'missing'; color: string; description: string }[];
}
export interface Labels { repos: string[]; rows: LabelRow[]; errors: RepoError[] }
export interface LabelChange extends GithubLabel { repo: string; update: boolean; was?: GithubLabel; error?: string }
export interface LintFinding { check: string; issue_id: string; title: string; message: string }
export interface StrayLabel extends GithubLabel { repo: string; count: number }
export interface Lint { findings: LintFinding[]; stray: StrayLabel[]; errors: RepoError[] }

export function getLabels(): Promise<Labels> {
  return request('/api/labels');
}

export function sendLabels(dryRun: boolean): Promise<{ changes: LabelChange[]; errors: RepoError[] }> {
  return request('/api/labels/send', json('POST', { dry_run: dryRun }));
}

// adoptLabel is text, the project file, with GitHub's color and description
// of field:value written in; nothing is saved.
export function adoptLabel(text: string, row: LabelRow, github: { color: string; description: string }): Promise<{ text: string }> {
  return request('/api/labels/adopt', json('POST', { text, field: row.field, value: row.value, ...github }));
}

export function getLint(): Promise<Lint> {
  return request('/api/lint');
}

// deleteLabel deletes a GitHub label outside the project's; dryRun answers
// how many issues and pull requests carry it there.
export function deleteLabel(repo: string, name: string, dryRun: boolean): Promise<{ count?: number }> {
  return request('/api/labels/delete', json('POST', { repo, name, dry_run: dryRun }));
}
