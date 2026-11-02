// Every keyboard shortcut, once (docs/prd/header-help.md, 1): the list
// behind ? (KeyboardHelp), the palette's keys, the key line at the foot
// and the tips all read it. The handlers stay where they act and call
// noteUse, so a shortcut you use stops being a tip.
// Pure but for the shared refs: node --test imports it.
import { ref } from 'vue';

// where you are: the most specific last ('board', then 'panel' when an
// issue is open, then 'filters' while you're in the filters panel)
export type Context = 'any' | 'board' | 'panel' | 'filters' | 'settings-file';

export interface Shortcut {
  id: string;
  keys: string[]; // 'Mod' is ⌘ on a Mac, Ctrl elsewhere; 'Alt' is ⌥ on a Mac
  what: string; // as the list says it
  group: 'Board' | 'Issue panel' | 'Filters' | 'Settings › The file';
  contexts: Context[];
  tip?: string; // the tip's words after the keys; none: never a tip
  short?: string; // the key line's words
}

export const shortcuts: Shortcut[] = [
  { id: 'palette', keys: ['Mod', 'K'], what: 'Filter the issues, jump to one, or run a command', group: 'Board', contexts: ['any'], tip: 'filter, jump to an issue, or run a command', short: 'search' },
  { id: 'complete', keys: ['Tab'], what: "In the search, complete a field's value (status:r → status:ready)", group: 'Board', contexts: ['board'], tip: 'in the search, completes status:r → status:ready' },
  { id: 'new', keys: ['c'], what: 'New issue', group: 'Board', contexts: ['board'], tip: 'a new issue', short: 'new issue' },
  { id: 'help', keys: ['?'], what: 'This list', group: 'Board', contexts: ['any'], short: 'every shortcut' },
  { id: 'next', keys: ['j'], what: 'Next issue', group: 'Issue panel', contexts: ['panel'], tip: 'the next issue on the board', short: 'next' },
  { id: 'prev', keys: ['k'], what: 'Previous issue', group: 'Issue panel', contexts: ['panel'], tip: 'the previous issue', short: 'previous' },
  { id: 'wide', keys: ['f'], what: 'Wide over the board, or back to the side', group: 'Issue panel', contexts: ['panel'], tip: 'the issue wide over the board, or back to the side', short: 'wide' },
  { id: 'esc', keys: ['Esc'], what: 'Leave the field you type in; again: close the panel', group: 'Issue panel', contexts: ['panel'], short: 'close' },
  { id: 'send', keys: ['Mod', 'Enter'], what: 'Send the comment', group: 'Issue panel', contexts: ['panel'], tip: 'sends the comment you are writing' },
  { id: 'click', keys: ['Click'], what: 'Add or remove a value', group: 'Filters', contexts: ['filters'] },
  { id: 'only', keys: ['Alt', 'Click'], what: 'Only this value; again: every value but it', group: 'Filters', contexts: ['filters'], tip: 'only this value; again: every value but it', short: 'only this' },
  { id: 'range', keys: ['Shift', 'Click'], what: 'A range, from the last value clicked', group: 'Filters', contexts: ['filters'], tip: 'a range of values, from the last one clicked', short: 'a range' },
  { id: 'toggle', keys: ['Space'], what: 'Toggle the focused value', group: 'Filters', contexts: ['filters'], short: 'toggle' },
  { id: 'save', keys: ['Mod', 'S'], what: 'Save, after a look at the changes', group: 'Settings › The file', contexts: ['settings-file'], tip: 'saves the file, after a look at the changes', short: 'save' },
];

export const groups = ['Board', 'Issue panel', 'Filters', 'Settings › The file'] as const;

const mac = typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent);
// a key as this keyboard writes it
export function keyLabel(k: string, isMac = mac): string {
  if (k === 'Mod') return isMac ? '⌘' : 'Ctrl';
  if (k === 'Alt') return isMac ? '⌥' : 'Alt';
  return k;
}

export const byId = (id: string) => shortcuts.find((s) => s.id === id);

// the current context, set by the pages; [] is anywhere
export const keyContext = ref<Context[]>([]);

// the shortcuts that work in ctx: 'any' ones everywhere
export function inContext(ctx: Context[]): Shortcut[] {
  return shortcuts.filter((s) => s.contexts.some((c) => c === 'any' || ctx.includes(c)));
}

// the key line: the most specific context's keys first, then anywhere's,
// ? always last; at most five
export function lineFor(ctx: Context[]): Shortcut[] {
  const own = ctx.length ? shortcuts.filter((s) => s.short && s.id !== 'help' && s.contexts.includes(ctx[ctx.length - 1]!)) : [];
  const any = shortcuts.filter((s) => s.short && s.id !== 'help' && s.contexts.includes('any') && !own.includes(s));
  return [...own, ...any].slice(0, 4).concat(byId('help')!);
}

// uses, per shortcut, in this browser (gb-keys-used): a tip retires at 3
interface Store {
  getItem(k: string): string | null;
  setItem(k: string, v: string): void;
}
const local = (): Store | null => {
  try {
    return typeof localStorage === 'undefined' ? null : localStorage;
  } catch {
    return null;
  }
};
export function uses(store: Store | null = local()): Record<string, number> {
  try {
    return JSON.parse(store?.getItem('gb-keys-used') ?? '{}') as Record<string, number>;
  } catch {
    return {};
  }
}
export function noteUse(id: string, store: Store | null = local()) {
  const u = uses(store);
  u[id] = (u[id] ?? 0) + 1;
  try {
    store?.setItem('gb-keys-used', JSON.stringify(u));
  } catch {
    // private window: lasts until reload
  }
  usesChanged.value++;
}
// a use noted: the tips recompute their pool
export const usesChanged = ref(0);

// a bar holds the bottom of the window (the board's picked cards): the key
// line steps aside, it never covers one
export const bottomBarShown = ref(false);
