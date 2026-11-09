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

// a git remote, added from the page: the name (origin most of the time)
// and its URL. A name the clone has already is refused.
export function addRemote(name: string, url: string): Promise<void> {
  return send('POST', `/api/remotes/${enc(name)}`, { body: { url } });
}

// {prefix, start} starts numbering; {} numbers the issues that have none
export function numbers(start?: { prefix: string; start: number }): Promise<Numbered> {
  return send('POST', '/api/numbers', { body: start ?? {} });
}

// a forge's [forges.<name>] table, its token to git's credential helper:
// the name written (gh, gh2… when none is given)
export interface NewForge {
  kind: string;
  url: string;
  name?: string;
  login?: string;
  token?: string;
  sync?: boolean;
  way?: ForgeWay; // '': both
}

export function addForge(f: NewForge): Promise<{ name: string }> {
  return send('POST', '/api/forges', { body: f });
}

// The forges' tokens as forge status reads them (routes 59, 55): each
// forge, and for each of its accounts whether a token is found, and
// where. check asks each forge whose each token is (the network). Never
// a token's value.
export interface ForgeToken {
  login: string;
  role: string; // posting (the forge's table's login) or you
  found: boolean;
  source?: string; // an environment variable, or git's credential helper
  owner?: string; // checked: the login the forge says the token is
  error?: string; // checked: what the forge answered instead
}

export interface ForgeAuth {
  name: string;
  kind: string;
  host: string;
  project: string;
  sync: boolean;
  way: string; // push (a backup), pull (read-only), or both
  discovered?: boolean; // found in origin's URLs, no [forges] table
  device?: boolean; // logs in in the browser (the OAuth device flow, routes 62 and 63)
  tokens: ForgeToken[];
}

export function getForgeAuth(check = false): Promise<{ items: ForgeAuth[] }> {
  return get(`/api/forges/auth${check ? '?check=1' : ''}`);
}

// a token stored in git's credential helper (forge login, ADR 0010): on
// the request's body alone, never shown again
export interface NewToken {
  forge: string;
  login?: string; // '' : yours on the host, else the forge's posting account
  token: string;
}

export function storeForgeToken(t: NewToken): Promise<{ forge: string; host: string; login: string }> {
  return send('POST', '/api/forges/auth', { body: t });
}

// a login in the browser (the OAuth device flow): the code to type at
// verification_uri, then the outcome. The token never reaches the page.
export interface DeviceLogin {
  forge?: string;
  state: 'none' | 'waiting' | 'done' | 'failed';
  user_code?: string;
  verification_uri?: string;
  expires_at?: string;
  login?: string; // done: whose token was stored
  error?: string; // failed: why
}

export function startDeviceLogin(forge: string): Promise<DeviceLogin> {
  return send('POST', '/api/forges/auth/device', { body: { forge } });
}

export function getDeviceLogin(): Promise<DeviceLogin> {
  return get('/api/forges/auth/device');
}

// the datastore this clone keeps its board in, as board.datastore in its
// own git config: '' or 'git' for git, else the value (a postgres:// URL,
// markdown:<dir>). The value is opened first, so a name no datastore
// understands is refused and the board still opens. No restart.
export function setDatastore(value: string): Promise<{ name: string; where: string; replica: boolean; counter: boolean }> {
  return send('PUT', '/api/datastore', { body: { value } });
}

// the default .git-board.toml, written as `git board init` does: the
// template ('' the default), seeded with origin's labels and its
// Project's fields when it can read them. Never overwrites one.
export function initConfig(template = ''): Promise<{ path: string; lines: string[] }> {
  return send('POST', '/api/config/init', { body: template ? { template } : {} });
}

// a forge's way, its direction: push (a backup: the issues go there, and
// nothing comes back), pull (read-only), or both
export type ForgeWay = 'push' | 'pull' | 'both';

export function setForgeWay(name: string, way: ForgeWay): Promise<{ name: string; sync: boolean; way: string }> {
  return send('PATCH', `/api/forges/${enc(name)}`, { body: { way } });
}
