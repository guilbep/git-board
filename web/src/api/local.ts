// The board a page keeps (docs/rewrite/api.md, routes 64 to 66; ADR
// 0023): every issue, then what changed since, and which ids a page's
// lists hold.
import { call, get } from './http.ts';
import type { Issue } from './types.ts';

export interface Snapshot {
  stamp: string;
  cursor: string;
  issues: Issue[];
}

// getSnapshot is every issue; null when the page holds stamp already (304)
export async function getSnapshot(stamp: string): Promise<Snapshot | null> {
  const r = await call<Snapshot>('GET', '/api/snapshot', { ifNoneMatch: stamp || undefined });
  return r.status === 304 ? null : r.data;
}

export interface Changes {
  cursor: string;
  put: Issue[];
  dropped: string[];
}

// getChanges is what changed since cursor; an ApiError 410 (gone) when the
// server holds no more that far back
export const getChanges = (since: string) => get<Changes>('/api/changes', { query: { since } });

export interface IDLists {
  stamp: string;
  lists: string[][];
  totals: number[]; // how many each filter matches
}

// issueIDs is the ids each filter matches, sorted, from offset, limit of
// them (0: all), and how many it matches, in the order asked
export const issueIDs = (q: { f: string[]; sort?: string | undefined; limit?: number; offset?: number }, signal?: AbortSignal) =>
  get<IDLists>('/api/issues/ids', { query: { f: q.f, sort: q.sort, limit: q.limit, offset: q.offset }, signal });
