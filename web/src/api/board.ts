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
