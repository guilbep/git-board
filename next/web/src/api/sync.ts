// Where the board is kept, and its sync (docs/rewrite/api.md, routes 27 to
// 38).
import { enc, get, send } from './http.ts';
import type { Copied, Datastore, Numbered, RemoteResult, SyncRun, SyncStatus } from './types.ts';

export function getDatastore(): Promise<Datastore> {
  return get('/api/datastore');
}

// from and to are board.datastore values: '' is git
export function copyDatastore(from: string, to: string): Promise<Copied> {
  return send('POST', '/api/datastore/copy', { body: { from, to } });
}

export function getSync(): Promise<SyncStatus> {
  return get('/api/sync');
}

export interface SyncRequest {
  scope: 'forges' | 'all';
  way?: 'sync' | 'pull' | 'push';
  forges?: string[];
  remote?: string;
  dry_run?: boolean;
  allow_create?: boolean;
}

// a run in the background: its steps come as progress events
export function startSync(r: SyncRequest): Promise<SyncRun> {
  return send('POST', '/api/sync', { body: r });
}

export function remoteAction(name: string, action: 'fetch' | 'pull' | 'push'): Promise<RemoteResult> {
  return send('POST', `/api/remotes/${enc(name)}/${action}`, {});
}

// {prefix, start} starts numbering; {} numbers the issues that have none
export function numbers(start?: { prefix: string; start: number }): Promise<Numbered> {
  return send('POST', '/api/numbers', { body: start ?? {} });
}
