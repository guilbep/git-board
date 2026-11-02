// Tips, one shortcut at a time (docs/prd/header-help.md, 4): chosen among
// the shortcuts that work where you are, never one you already use (3
// uses, keys.ts), nor one you dismissed; none at all once tips are off.
// Per browser, in localStorage. Pure but for the shared refs.
import { ref } from 'vue';
import type { Shortcut } from './keys.ts';
import { uses } from './keys.ts';

export const RETIRE_AFTER = 3;
export const EVERY_MS = 12_000;

interface Store {
  getItem(k: string): string | null;
  setItem(k: string, v: string): void;
  removeItem(k: string): void;
}
const local = (): Store | null => {
  try {
    return typeof localStorage === 'undefined' ? null : localStorage;
  } catch {
    return null;
  }
};
const get = (s: Store | null, k: string) => {
  try {
    return s?.getItem(k) ?? null;
  } catch {
    return null;
  }
};
const set = (s: Store | null, k: string, v: string | null) => {
  try {
    if (v === null) s?.removeItem(k);
    else s?.setItem(k, v);
  } catch {
    // private window: lasts until reload
  }
};

export function dismissed(store: Store | null = local()): string[] {
  try {
    return JSON.parse(get(store, 'gb-tips-dismissed') ?? '[]') as string[];
  } catch {
    return [];
  }
}

// the shortcuts no tip shows again: used enough, or dismissed
export function retired(store: Store | null = local()): Set<string> {
  const u = uses(store);
  return new Set([...Object.keys(u).filter((id) => (u[id] ?? 0) >= RETIRE_AFTER), ...dismissed(store)]);
}

// the next tip after last, in the pool's order, skipping the retired and
// the shortcuts without a tip; null when none is left
export function pickTip(pool: Shortcut[], gone: Set<string>, last?: string): Shortcut | null {
  const left = pool.filter((s) => s.tip && !gone.has(s.id));
  if (!left.length) return null;
  const at = left.findIndex((s) => s.id === last);
  return left[(at + 1) % left.length]!;
}

export const tipsOff = ref(get(local(), 'gb-tips-off') === '1');
// a tip dismissed, or all reset: the tip line recomputes
export const tipsChanged = ref(0);

export function setTipsOff(off: boolean, store: Store | null = local()) {
  tipsOff.value = off;
  set(store, 'gb-tips-off', off ? '1' : null);
}

export function dismissTip(id: string, store: Store | null = local()) {
  set(store, 'gb-tips-dismissed', JSON.stringify([...new Set([...dismissed(store), id])]));
  tipsChanged.value++;
}

// every tip again: uses forgotten, nothing dismissed, tips on
export function resetTips(store: Store | null = local()) {
  set(store, 'gb-tips-dismissed', null);
  set(store, 'gb-keys-used', null);
  setTipsOff(false, store);
  tipsChanged.value++;
}
