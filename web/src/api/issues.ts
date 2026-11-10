// Issues (docs/rewrite/api.md, routes 3 to 14). Every write sends the
// version it was read at (If-Match): a 412 stale says someone wrote in
// between, and the page offers to reload or to overwrite (the same write
// without a version: the last writer wins, field by field).
import { call, enc, get, send } from './http.ts';
import type { FollowState, Issue, LogEntry, Page, Thread, Undone } from './types.ts';

const path = (id: string) => `/api/issues/${enc(id)}`;

export interface ListQuery {
  filter?: string | undefined; // the CLI's grammar; absent: is:open; '': every issue
  sort?: string | undefined; // created, updated, title or a field, then asc or desc
  limit?: number | undefined; // 100 by default, 1,000 at most, 0 for all
  cursor?: string | undefined;
  at?: string | undefined;
}

export function listIssues(q: ListQuery, signal?: AbortSignal): Promise<Page<Issue>> {
  return get('/api/issues', { query: { ...q }, signal });
}

// many lists in one request (GET /api/issues/batch): pages, a first page
// each filter (a board's lanes), and totals, each filter counted alone,
// in the order asked
export interface Batch {
  pages: Page<Issue>[];
  totals: number[];
}

// a batch holds at most 64 lists (the server's maxBatch): more go in
// several, at once
const MAX_BATCH = 64;

export async function batchIssues(q: { pages?: string[]; counts?: string[]; sort?: string | undefined; limit?: number }, signal?: AbortSignal): Promise<Batch> {
  const pages = q.pages ?? [];
  const counts = q.counts ?? [];
  if (pages.length + counts.length === 0) return { pages: [], totals: [] };
  const pa = pages.slice(0, MAX_BATCH);
  const ca = counts.slice(0, MAX_BATCH - pa.length);
  const first = get<Batch>('/api/issues/batch', { query: { f: pa, c: ca, sort: q.sort, limit: q.limit }, signal });
  if (pa.length === pages.length && ca.length === counts.length) return first;
  const [a, b] = await Promise.all([first, batchIssues({ ...q, pages: pages.slice(pa.length), counts: counts.slice(ca.length) }, signal)]);
  return { pages: [...a.pages, ...b.pages], totals: [...a.totals, ...b.totals] };
}

export interface NewIssue {
  title: string;
  body?: string;
  labels?: string[];
  set?: string[]; // ["status=ready", "assignee=@me"]
  force?: boolean;
}

export function createIssue(n: NewIssue): Promise<Thread> {
  return send('POST', '/api/issues', { body: n });
}

export function getThread(id: string, at?: string): Promise<Thread> {
  return get(path(id), { query: { at } });
}

// IssueChange is one PATCH: each field optional, applied in this order.
export interface IssueChange {
  title?: string;
  body?: string;
  base?: string; // the description the edit started from: changed if it moved
  labels?: { add?: string[]; remove?: string[] };
  set?: string[];
  force?: boolean;
  status?: 'open' | 'closed';
  reason?: 'completed' | 'not-planned' | 'duplicate';
  duplicate_of?: string;
  comment?: string; // with status: "Close with comment"
}

// version: the thread's, or '' to write over whatever it is now
export function patchIssue(id: string, version: string, change: IssueChange): Promise<Thread> {
  return send('PATCH', path(id), { body: change, ifMatch: version });
}

export async function deleteIssue(id: string, version: string): Promise<void> {
  await call('DELETE', path(id), { ifMatch: version });
}

export async function issueLog(id: string, q: { field?: string; since?: string } = {}): Promise<LogEntry[]> {
  return (await get<{ items: LogEntry[] }>(`${path(id)}/log`, { query: q })).items;
}

export function addComment(id: string, version: string, body: string): Promise<Thread> {
  return send('POST', `${path(id)}/comments`, { body: { body }, ifMatch: version });
}

// base: the text the edit started from (409 changed if it moved since)
export function editComment(id: string, version: string, comment: string, body: string, base: string): Promise<Thread> {
  return send('PATCH', `${path(id)}/comments/${enc(comment)}`, { body: { body, base }, ifMatch: version });
}

export function deleteComment(id: string, version: string, comment: string): Promise<Thread> {
  return send('DELETE', `${path(id)}/comments/${enc(comment)}`, { ifMatch: version });
}

export function undo(id: string, force = false): Promise<Undone> {
  return send('POST', `${path(id)}/undo`, { body: { force } });
}

export function getFollow(id: string): Promise<FollowState> {
  return get(`${path(id)}/follow`);
}

// null: back to following automatically
export function setFollow(id: string, follow: boolean | null): Promise<FollowState> {
  return send('PUT', `${path(id)}/follow`, { body: { follow } });
}
