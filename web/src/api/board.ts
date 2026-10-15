// Typed client for the git-board-server API. All HTTP for the board page
// goes through here — never fetch() from a component directly.

// Same origin in production (frontend is embedded in the Go binary); the
// backend's default dev port otherwise (see cmd/git-board-server --addr and
// web/README.md for how to point elsewhere).
const API_BASE = import.meta.env.DEV ? 'http://localhost:8080' : '';

export interface Issue {
  id: string;
  title: string;
  status: string;
  labels: string[];
  github_url?: string; // absent when the issue wasn't bridged in from GitHub
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
}

export interface FieldConfig {
  single: boolean;
  options: string[] | null;
  numeric: boolean;
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  // The server only takes JSON writes (withGuard in cmd/git-board-server).
  if (init?.method && init.method !== 'GET') {
    init = { ...init, headers: { 'Content-Type': 'application/json', ...init.headers }, body: init.body ?? '{}' };
  }
  const res = await fetch(API_BASE + path, init).catch(() => {
    throw new Error('The git-board server is not reachable.');
  });
  if (!res.ok) {
    // The server answers errors as {"error": "..."} (writeError in
    // cmd/git-board-server); surface that message, e.g. "@me" with no
    // --me set.
    const body = await res.text();
    let message = body;
    try {
      message = (JSON.parse(body) as { error?: string }).error ?? body;
    } catch {
      // not JSON: keep the raw body
    }
    throw new Error(message || `${init?.method ?? 'GET'} ${path} failed (${res.status})`);
  }
  return res.json() as Promise<T>;
}

export function getViews(): Promise<Record<string, ViewConfig>> {
  return request('/api/views');
}

export function getFields(): Promise<Record<string, FieldConfig>> {
  return request('/api/config/fields');
}

// Unsorted: the page sorts (facets.sortIssues), by option order where the
// server would sort alphabetically.
export function getIssues(filter: string, group: string): Promise<Bucket[]> {
  const params = new URLSearchParams();
  if (filter) params.set('filter', filter);
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
  pending?: string[]; // forges the next sync hasn't posted this text to yet
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
  forges: Forge[] | null;
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

export function editComment(id: string, commentId: string, body: string): Promise<Thread> {
  return request(`${issuePath(id)}/comments/${encodeURIComponent(commentId)}`, json('PUT', { body }));
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

// The sync with GitHub/GitLab (cmd/git-board-server/forges.go).
export interface BridgeTally {
  imported: number;
  pulled: number;
  created: number;
  pushed: number;
  would_import: number;
  would_pull: number;
  would_create: number;
  would_push: number;
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
  bridges: Record<string, BridgeTally>;
  errors: number;
  error?: string;
  pushed?: string;
}

export interface ForgeStatus {
  bridges: { name: string; target: string }[];
  job: ForgeJob | null;
}

export function getForges(): Promise<ForgeStatus> {
  return request('/api/forges');
}

export function startForgeSync(dryRun: boolean, push: boolean): Promise<ForgeStatus> {
  return request('/api/forges/sync', json('POST', { dry_run: dryRun, push }));
}

// Where the page loads a server path from (the API's origin in dev).
export const apiURL = (path: string) => API_BASE + path;

// renderMarkdown is the composer's preview, rendered as a comment would be.
export function renderMarkdown(body: string): Promise<{ html: string }> {
  return request('/api/markdown', json('POST', { body }));
}
