// The board this page keeps (ADR 0023): every issue, from the snapshot
// (GET /api/snapshot) and the changes since (GET /api/changes), as the
// change events say when; kept in IndexedDB too, so a page opened paints
// at once from the last visit and then catches up. Lists ask the server
// which ids they hold (GET /api/issues/ids) and show the issues kept here.
// A write shows here at once (apply) and goes back if the server refuses.
// ponytail: the whole board in memory: fine to ~50,000 issues (~50 MB);
// the snapshot paged past that.
import { shallowRef, triggerRef } from 'vue';
import { ApiError, getChanges, getSnapshot, type Issue } from '@/api';
import { load, save } from './idb';
import { onLive } from '@/live';

export const issues = shallowRef(new Map<string, Issue>());
let stamp = '';
let cursor = '';
let booted: Promise<void> | null = null;

// ready is the board kept, once: from IndexedDB, then the server's
export function ready(): Promise<void> {
  booted ??= boot();
  return booted;
}

async function boot(): Promise<void> {
  // an issue's change: what changed since; the rest (the file, a person,
  // a sync, a reconnection that may have missed some): the snapshot, a 304
  // when the board didn't move
  onLive((e) => {
    if (e.kind === 'issue') void settle(pull);
    else if (['config', 'person', 'me', 'sync', 'resync'].includes(e.kind)) void settle(refresh);
  }, { own: true });
  const kept = await load();
  if (kept) {
    issues.value = new Map(kept.issues.map((i) => [i.id, i]));
    stamp = kept.stamp;
    cursor = kept.cursor;
  }
  try {
    await (cursor ? pull() : refresh());
  } catch (e) {
    if (!kept) throw e; // a board kept from before still shows, unreachable
  }
}

// refresh takes the snapshot again, unless the board is the one kept
export async function refresh(): Promise<void> {
  const s = await getSnapshot(stamp);
  if (!s) return;
  issues.value = new Map(s.issues.map((i) => [i.id, i]));
  stamp = s.stamp;
  cursor = s.cursor;
  persist();
}

// pull takes the changes since the cursor; too far behind, the snapshot
export async function pull(): Promise<void> {
  if (!cursor) return refresh();
  let c;
  try {
    c = await getChanges(cursor);
  } catch (e) {
    if (e instanceof ApiError && e.status === 410) return refresh();
    throw e;
  }
  cursor = c.cursor;
  if (!c.put.length && !c.dropped.length) return;
  const m = issues.value;
  for (const i of c.put) m.set(i.id, i);
  for (const id of c.dropped) m.delete(id);
  stamp = ''; // the board moved: the next snapshot is asked whole
  triggerRef(issues);
  persist();
}

// one catch-up at a time: a burst of events shares it
let catching: Promise<void> | null = null;
function settle(f: () => Promise<void>): Promise<void> {
  catching ??= f().catch(() => {}).finally(() => (catching = null));
  return catching;
}

// current is the board as it stands once the catch-up running ends: a
// list asked now shows the issues it names as they are
export async function current(): Promise<void> {
  await ready();
  await catching;
}

export const get = (id: string): Issue | undefined => issues.value.get(id);

// apply shows a write at once: the issue patched here; its answer, the
// rollback, for a write the server refused
export function apply(id: string, patch: Partial<Issue>): () => void {
  const before = issues.value.get(id);
  if (!before) return () => {};
  issues.value.set(id, { ...before, ...patch });
  triggerRef(issues);
  return () => {
    issues.value.set(id, before);
    triggerRef(issues);
  };
}

let saving: ReturnType<typeof setTimeout> | undefined;
function persist() {
  clearTimeout(saving);
  saving = setTimeout(() => void save({ stamp, cursor, issues: [...issues.value.values()] }), 500);
}
