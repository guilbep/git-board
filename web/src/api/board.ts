import { fieldTypes } from '@/fieldtypes';
// Typed client for the git-board-server API. All HTTP for the board page
// goes through here — never fetch() from a component directly.

// Same origin in production (frontend is embedded in the Go binary); the
// backend's default dev port otherwise (see cmd/git-board-server --addr and
// web/README.md for how to point elsewhere).
export const API_BASE = import.meta.env.DEV ? 'http://localhost:8080' : '';

// clientId names this page in the writes it makes, so the live updates it
// gets back (live.ts) can tell its own changes from someone else's.
export const clientId = Math.random().toString(36).slice(2) + Date.now().toString(36);

export interface Issue {
  id: string;
  title: string;
  status: string;
  labels: string[];
  github_url?: string; // absent when the issue wasn't bridged in from GitHub
  forges?: Forge[]; // where it is on forges
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
}

// ApiError carries the server's status and code: syncing (a sync with
// the forges runs: retry after it), changed (edited meanwhile).
export class ApiError extends Error {
  constructor(message: string, readonly status: number, readonly code: string) {
    super(message);
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  // The server only takes JSON writes (withGuard in cmd/git-board-server).
  if (init?.method && init.method !== 'GET') {
    init = { ...init, headers: { 'Content-Type': 'application/json', 'X-Git-Board-Client': clientId, ...init.headers }, body: init.body ?? '{}' };
  }
  const res = await fetch(API_BASE + path, init).catch(() => {
    throw new Error('The git-board server is not reachable.');
  });
  if (!res.ok) {
    // The server answers errors as {"error": "...", "code"?: "..."}
    // (writeError, writeCoded in cmd/git-board-server); surface that
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
// server would sort alphabetically.
// q: the search box, GitHub's syntax (free text in titles, descriptions
// and comments; assignee:x, label:x, author:x, no:x, is:closed…).
export function getIssues(filter: string, group: string, q = ''): Promise<Bucket[]> {
  const params = new URLSearchParams();
  if (filter) params.set('filter', filter);
  if (q.trim()) params.set('q', q);
  if (group) params.set('group', group);
  return request(`/api/issues?${params}`);
}

export function patchIssue(id: string, changes: Record<string, string>): Promise<Issue> {
  return request(`/api/issues/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(changes),
  });
}

// The GitHub Project sync (cmd/git-board-server/sync.go). Only check, apply
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

// Issue management (cmd/git-board-server/issues.go): every write answers
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
export function addComment(id: string, body: string, status?: 'open' | 'closed'): Promise<Thread> {
  return request(`${issuePath(id)}/comments`, json('POST', { body, status: status ?? '' }));
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

// The sync with the forges (cmd/git-board-server/forges.go).
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
  warnings: number;
  errors: number;
  lines: string[];
}

export interface ForgeJob {
  dry_run: boolean;
  push: boolean;
  state: 'running' | 'done' | 'failed';
  started: string;
  ended?: string;
  names: string[];
  bridges: Record<string, BridgeTally>;
  errors: number;
  error?: string;
  pushed?: string;
}

export interface ForgeStatus {
  bridges: { name: string; target: string }[];
  job: ForgeJob | null;
  origin: string[]; // where pushing the git-bug data after a run goes
}

export function getForges(): Promise<ForgeStatus> {
  return request('/api/forges');
}

// A dry run looks at every forge; a real run at bridges (empty: all).
export function startForgeSync(dryRun: boolean, push: boolean, bridges: string[] = []): Promise<ForgeStatus> {
  return request('/api/forges/sync', json('POST', { dry_run: dryRun, push, bridges }));
}

// Where the page loads a server path from (the API's origin in dev).
export const apiURL = (path: string) => API_BASE + path;

// renderMarkdown is the composer's preview, rendered as a comment would be.
export function renderMarkdown(body: string): Promise<{ html: string }> {
  return request('/api/markdown', json('POST', { body }));
}

// Attachments (cmd/git-board-server/uploads.go): on S3, served by a CDN to
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

// Settings (cmd/git-board-server/settings.go).
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

// docs/configuration.md, a section per ## heading, rendered.
export interface DocSection {
  id: string;
  title: string;
  html: string;
}

export function getConfigDocs(): Promise<DocSection[]> {
  return request('/api/config/docs');
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

export function addBridge(b: { target: string; url: string; name?: string | undefined; credential?: string | undefined; token?: string | undefined }): Promise<{ name: string }> {
  return request('/api/bridges', json('POST', b));
}

export function removeBridge(name: string): Promise<Record<string, never>> {
  return request(`/api/bridges/${encodeURIComponent(name)}`, json('DELETE', {}));
}

// Insights (cmd/git-board-server/insights.go): the issues' history
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
