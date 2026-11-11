// The board kept between visits, in IndexedDB: one record a server (its
// origin). Every call is tried: a private window, a full disk or a
// browser without it leave the page as it was without one.
import type { Issue } from '@/api';

export interface Kept {
  stamp: string;
  cursor: string;
  issues: Issue[];
}

const DB = 'git-board';
const STORE = 'board';
const KEY = typeof location === 'undefined' ? 'board' : location.origin;

function open(): Promise<IDBDatabase | null> {
  return new Promise((ok) => {
    try {
      const r = indexedDB.open(DB, 1);
      r.onupgradeneeded = () => r.result.createObjectStore(STORE);
      r.onsuccess = () => ok(r.result);
      r.onerror = () => ok(null);
      r.onblocked = () => ok(null);
    } catch {
      ok(null);
    }
  });
}

export async function load(): Promise<Kept | null> {
  const db = await open();
  if (!db) return null;
  return new Promise((ok) => {
    try {
      const r = db.transaction(STORE).objectStore(STORE).get(KEY);
      r.onsuccess = () => ok((r.result as Kept | undefined) ?? null);
      r.onerror = () => ok(null);
    } catch {
      ok(null);
    }
  });
}

export async function save(k: Kept): Promise<void> {
  const db = await open();
  if (!db) return;
  try {
    db.transaction(STORE, 'readwrite').objectStore(STORE).put(k, KEY);
  } catch {
    // not kept: the next visit takes the snapshot
  }
}
