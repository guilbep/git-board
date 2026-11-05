// .git-board.toml (docs/rewrite/api.md, routes 23 to 25): the file's text,
// its version (a hash of the text), and its keys. A write sends the
// version it read (If-Match): 412 stale when the file moved since.
import { get, send } from './http.ts';
import type { ConfigFile } from './types.ts';

export function getConfig(): Promise<ConfigFile> {
  return get('/api/config');
}

// the whole text; dryRun checks it and writes nothing
export function putConfig(text: string, version: string, dryRun = false): Promise<ConfigFile> {
  return send('PUT', '/api/config', { body: { text }, ifMatch: version, query: { dry_run: dryRun ? 1 : undefined } });
}

// dotted keys, each a value or a whole table (an object), null removing
// it: what a settings form saves
export function patchConfig(set: Record<string, unknown>, version: string, dryRun = false): Promise<ConfigFile> {
  return send('PATCH', '/api/config', { body: { set }, ifMatch: version, query: { dry_run: dryRun ? 1 : undefined } });
}
