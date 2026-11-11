// The HTTP of v2's API (docs/rewrite/api.md, The rules every route
// follows): JSON in and out, one error shape, a version on every entity
// (its ETag) sent back in If-Match by every write. Every request of the
// page goes through call: never fetch() from a component.

// Same origin once embedded in the binary; the server's default port from
// the dev server (README.md).
export const API_BASE = import.meta.env.DEV ? 'http://localhost:8080' : '';

// clientId names this page in its writes (X-Git-Board-Client): the change
// events carry it as `by`, so the page can tell its own changes.
export const clientId = Math.random().toString(36).slice(2) + Date.now().toString(36);

import { ApiError, errorOf } from './error.ts';
import { active, begin, finished, onSlow, routeName, started, traceparent } from '../interaction.ts';

export { ApiError, errorOf, isNotFound, isStale, message, type ErrorCode } from './error.ts';

export interface Call {
  query?: Record<string, string | number | boolean | string[] | undefined> | undefined; // an array: the key once a value
  body?: unknown;
  ifMatch?: string | undefined; // a version, unquoted
  ifNoneMatch?: string | undefined;
  signal?: AbortSignal | undefined;
  // keep: a GET's answer serves the same GET for this many ms, unless a
  // write or a change event came between (forgetPending): what a page
  // read ahead (a card hovered) is what it opens
  keep?: number | undefined;
  // background: no act waits for it (a check of what the page answered
  // itself, the filters panel's counts): it isn't counted in an act's time
  background?: boolean | undefined;
}

export interface Answer<T> {
  data: T;
  status: number;
  version: string; // the ETag, unquoted; "" when none
  location: string;
}

export function url(path: string, query?: Call['query']): string {
  const q = new URLSearchParams();
  for (const [k, v] of Object.entries(query ?? {})) {
    if (Array.isArray(v)) for (const x of v) q.append(k, x);
    else if (v !== undefined) q.set(k, String(v));
  }
  const s = q.toString();
  return API_BASE + path + (s ? `?${s}` : '');
}

const unquote = (etag: string | null) => (etag ?? '').replace(/^W\//, '').replace(/^"|"$/g, '');

// A response, read: the body as text, so each caller parses its own copy.
interface Raw {
  status: number;
  ok: boolean;
  version: string;
  location: string;
  text: string;
}

async function fetchRaw(method: string, path: string, c: Call): Promise<Raw> {
  const headers: Record<string, string> = {};
  if (method !== 'GET') {
    headers['Content-Type'] = 'application/json';
    headers['X-Git-Board-Client'] = clientId;
  }
  if (c.ifMatch) headers['If-Match'] = `"${c.ifMatch}"`;
  if (c.ifNoneMatch) headers['If-None-Match'] = `"${c.ifNoneMatch}"`;
  headers['traceparent'] = traceparent(); // the interaction's trace: the server's spans line up under it
  const init: RequestInit = { method, headers };
  if (method !== 'GET' && method !== 'DELETE') init.body = JSON.stringify(c.body ?? {});
  if (c.signal) init.signal = c.signal;
  if (!c.background) started();
  try {
    const res = await fetch(url(path, c.query), init).catch((e: unknown) => {
      if (e instanceof DOMException && e.name === 'AbortError') throw e;
      throw new ApiError('The git-board server is not reachable.', 0, 'unreachable');
    });
    const text = res.status === 204 || res.status === 304 ? '' : await res.text();
    return { status: res.status, ok: res.ok, version: unquote(res.headers.get('ETag')), location: res.headers.get('Location') ?? '', text };
  } finally {
    if (!c.background) finished();
  }
}

// a slow interaction, told to the server (POST /api/slow): outside every
// interaction, and never one itself
onSlow((i, ms) => {
  void fetch(url('/api/slow'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'X-Git-Board-Client': clientId },
    body: JSON.stringify({ name: i.name, ms: Math.round(ms), trace: i.trace, span: i.span }),
    keepalive: true,
  }).catch(() => {});
});

// The GETs on their way, by URL: the parts of a page that ask the same
// thing at once (a change event heard by five of them) share one request.
// A write, or a change event (forgetPending), ends the sharing: what is
// asked after it is asked again, never answered from before it. A GET
// with a signal goes alone: another caller's abort isn't its own.
const pending = new Map<string, Promise<Raw>>();

export function forgetPending(): void {
  pending.clear();
  kept.clear();
  changes++;
}

// changes counts the writes and change events heard: an answer read at
// the same count is the board's as it still is, as far as the page knows
let changes = 0;
export const changesHeard = () => changes;

// the answers kept a moment (Call.keep), by URL
const kept = new Map<string, { at: number; raw: Raw }>();

function sharedGet(path: string, c: Call): Promise<Raw> {
  const key = url(path, c.query) + '\n' + (c.ifNoneMatch ?? '');
  let p = pending.get(key);
  if (!p) {
    p = fetchRaw('GET', path, c);
    pending.set(key, p);
    const done = () => pending.get(key) === p && pending.delete(key);
    p.then(done, done);
  }
  return p;
}

// the last answer of each GET that came with an ETag, by URL: asked again,
// it goes with If-None-Match, and a 304 answers what was kept (a page that
// reloads on a change event pays the change, not the list). Kept across
// events: the server says whether it moved.
// ponytail: the last 100 URLs; an LRU by size if answers grow
const tagged = new Map<string, Raw>();
function remember(key: string, r: Raw) {
  tagged.delete(key);
  tagged.set(key, r);
  if (tagged.size > 100) tagged.delete(tagged.keys().next().value!);
}

export async function call<T>(method: string, path: string, c: Call = {}): Promise<Answer<T>> {
  if (method !== 'GET') {
    forgetPending();
    if (!active()) begin(`${method} ${routeName(path)}`); // a write is an interaction, unless a named one goes on
  }
  const key = c.keep && method === 'GET' && !c.ifNoneMatch ? url(path, c.query) : '';
  const held = key ? kept.get(key) : undefined;
  // a GET the page asked before, with its ETag: asked again If-None-Match
  const tag = method === 'GET' && !c.ifNoneMatch ? url(path, c.query) : '';
  const known = tag ? tagged.get(tag) : undefined;
  const ask = known ? { ...c, ifNoneMatch: known.version } : c;
  let r = held && performance.now() - held.at < c.keep! ? held.raw : await (method === 'GET' && !c.signal ? sharedGet(path, ask) : fetchRaw(method, path, ask));
  if (known && r.status === 304) r = known; // as it was
  else if (tag && r.ok && r.version) remember(tag, r);
  if (key && r !== held?.raw && r.ok) kept.set(key, { at: performance.now(), raw: r });
  const answer = { status: r.status, version: r.version, location: r.location };
  if (r.status === 204 || r.status === 304) return { ...answer, data: undefined as T };
  if (!r.ok) {
    const e = errorOf(r.text);
    throw new ApiError(e.message || `${method} ${path} failed (${r.status})`, r.status, e.code, e.field);
  }
  return { ...answer, data: (r.text ? JSON.parse(r.text) : undefined) as T };
}

export const get = async <T>(path: string, c?: Call) => (await call<T>('GET', path, c)).data;
export const send = async <T>(method: 'POST' | 'PUT' | 'PATCH' | 'DELETE', path: string, c?: Call) => (await call<T>(method, path, c)).data;
export const enc = encodeURIComponent;
