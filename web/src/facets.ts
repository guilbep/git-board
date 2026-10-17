// The sidebar's filters and the table's sort. The facets are the server's
// (GET /api/facets): counted against the rest of the filter, the values it
// picks, the filter without them; a tick writes a new filter
// (viewfilter.ts), the page matches no issue itself. Pure logic, no "@/"
// imports: node runs facets.test.ts on it directly.
import type { Issue } from './api/board';
import { displayName, fieldNames, fieldValues, formatValue, type Fields } from './fields.ts';

/** The value of an issue without the field, as board.BucketBy names it. */
export const NONE = '(none)';
/** Facet keys besides label namespaces: git-bug's open/closed, and labels without a namespace. */
export const STATE = 'is';
export const PLAIN = 'label';

/** Facet key -> the values wanted, any of which matches. */
export type Selection = Record<string, string[]>;
export interface Facet {
  key: string;
  options: [value: string, count: number][];
  selected: string[]; // the values the filter lets through; none: no term on it
  rest: string; // the filter without the facet's own terms
}
export interface Sort {
  key: string; // "id", "title", STATE or a label namespace
  desc: boolean;
}

function valuesOf(issue: Issue, key: string): string[] {
  if (key === STATE) return [issue.status];
  const values =
    key === PLAIN ? issue.labels.filter((l) => !l.includes(':')) : fieldValues(issue.labels, key);
  return values.length ? values : [NONE];
}

/**
 * The label namespaces shown as fields: the configured ones, then any other
 * found on the issues (sprint, mod…), so a new "ns:value" label needs no
 * config to get its own column and filter.
 */
export function namespaces(issues: Issue[], fields: Fields): string[] {
  const configured = fieldNames(fields);
  const found = new Set(
    issues.flatMap((i) => i.labels.filter((l) => l.includes(':')).map((l) => l.slice(0, l.indexOf(':')))),
  );
  return [...configured, ...[...found].filter((ns) => !configured.includes(ns)).sort()];
}

export type OptionSort = 'count' | 'alpha' | 'workflow';

/**
 * A section's options by count (most first), alphabetically (a9 before
 * a10), or in the field's configured order; "(none)" last in every order.
 */
export function sortOptions(options: [string, number][], mode: OptionSort, configured: string[]): [string, number][] {
  const alpha = (a: string, b: string) => a.localeCompare(b, undefined, { numeric: true });
  const rank = (v: string) => (configured.includes(v) ? configured.indexOf(v) : configured.length);
  return [...options].sort(([a, na], [b, nb]) => {
    if ((a === NONE) !== (b === NONE)) return a === NONE ? 1 : -1;
    if (mode === 'count') return nb - na || alpha(a, b);
    if (mode === 'workflow') return rank(a) - rank(b) || nb - na || alpha(a, b);
    return alpha(a, b);
  });
}

/**
 * A section's selection after a click on value: a plain (or Ctrl/Cmd)
 * click toggles it; Alt keeps only it, and when it already is the only one,
 * all but it; Shift sets the range from the last clicked value (anchor) to
 * the state the clicked box takes. In the list's order.
 */
export function pick(
  cur: string[],
  value: string,
  order: string[],
  mods: { alt?: boolean; shift?: boolean; anchor?: string | undefined },
): string[] {
  // a selected value no longer listed stays selected, after the others
  const inOrder = (set: Set<string>) => [...order.filter((v) => set.has(v)), ...[...set].filter((v) => !order.includes(v))];
  if (mods.alt) {
    return cur.length === 1 && cur[0] === value ? order.filter((v) => v !== value) : [value];
  }
  const on = !cur.includes(value);
  const next = new Set(cur);
  const from = mods.shift && mods.anchor ? order.indexOf(mods.anchor) : -1;
  const to = order.indexOf(value);
  const range = from >= 0 && to >= 0 ? order.slice(Math.min(from, to), Math.max(from, to) + 1) : [value];
  for (const v of range) {
    if (on) next.add(v);
    else next.delete(v);
  }
  return inOrder(next);
}

/** "-prio" (the URL's form), or "prio desc" / "prio asc" (a view's). */
export function parseSort(spec: string): Sort | null {
  const [key = '', dir] = spec.trim().split(/\s+/);
  if (!key) return null;
  return key.startsWith('-') ? { key: key.slice(1), desc: true } : { key, desc: dir === 'desc' };
}

/**
 * Numeric fields as numbers (est 3.5 between 3 and 4), fields with options
 * in their configured order, the rest alphabetically; issues without the
 * value last either way, as the server's numeric sort does.
 */
export function sortIssues(issues: Issue[], sort: Sort | null, fields: Fields): Issue[] {
  if (!sort) return issues;
  const { key, desc } = sort;
  const field = fields[key];
  // links: the number on the issue's first forge
  const numeric = key === 'links' || field?.numeric || field?.type === 'number';
  const value = (i: Issue) => {
    if (key === 'links') return i.forges?.find((f) => f.number)?.number ?? null;
    const v = key === 'id' ? i.id : key === 'title' ? i.title : valuesOf(i, key)[0]!;
    return v === NONE || ((field?.numeric || field?.type === 'number') && Number.isNaN(Number(v))) ? null : v;
  };
  const options = field?.options ?? [];
  const rank = (v: string) => (options.includes(v) ? options.indexOf(v) : options.length);
  const compare = (a: string, b: string) =>
    numeric
      ? Number(a) - Number(b)
      : rank(a) - rank(b) || a.localeCompare(b, undefined, { numeric: true });

  return [...issues].sort((x, y) => {
    const a = value(x);
    const b = value(y);
    if (a === null || b === null) return a === null ? (b === null ? 0 : 1) : -1;
    return desc ? compare(b, a) : compare(a, b);
  });
}

/** An older link's ticks: one f.<facet> key per facet, one entry per value. */
export function parseSelection(query: Record<string, unknown>): Selection {
  const sel: Selection = {};
  for (const [k, v] of Object.entries(query)) {
    if (!k.startsWith('f.')) continue;
    const values = [v].flat().filter((x): x is string => typeof x === 'string' && x !== '');
    if (values.length) sel[k.slice(2)] = values;
  }
  return sel;
}

export function facetLabel(key: string): string {
  return key === STATE ? 'State' : key === PLAIN ? 'Labels' : displayName(key);
}

export function valueLabel(key: string, value: string, fields: Fields): string {
  return value === NONE ? 'Not set' : formatValue(key, value, fields);
}
