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
  const res = await fetch(API_BASE + path, init);
  if (!res.ok) {
    const body = await res.text();
    throw new Error(`${init?.method ?? 'GET'} ${path} -> ${res.status}: ${body}`);
  }
  return res.json() as Promise<T>;
}

export function getViews(): Promise<Record<string, ViewConfig>> {
  return request('/api/views');
}

export function getFields(): Promise<Record<string, FieldConfig>> {
  return request('/api/config/fields');
}

export function getIssues(filter: string, sort: string, group: string): Promise<Bucket[]> {
  const params = new URLSearchParams();
  if (filter) params.set('filter', filter);
  if (sort) params.set('sort', sort);
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
