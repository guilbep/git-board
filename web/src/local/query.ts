// A filter answered from the board this page keeps (local/store.ts; ADR
// 0023), for the filters a board's lanes and ticks are made of: is:,
// field:value, label:, no:, each with commas and a minus, and the sorts by
// creation, update and title. The server's rules, as domain/issues/filter.go
// has them (Matches, matchOne); anything else (free text, involves:,
// dates, @me, a sprint) is the server's alone: localIDs answers null. The
// page still asks the server, in the background, and takes its answer
// when they differ. Pure: node runs query.test.ts on it.
import type { Issue } from '../api/types.ts';

interface Term {
  key: string; // is, label, no, or a field
  values: string[];
  neg: boolean;
}

// terms is the filter as conditions, null when one isn't answered here
function parse(filter: string): Term[] | null {
  const out: Term[] = [];
  for (const raw of filter.match(/(?:[^\s"]+|"[^"]*")+/g) ?? []) {
    const token = raw.replaceAll('"', '');
    const neg = token.startsWith('-');
    const body = neg ? token.slice(1) : token;
    const colon = body.indexOf(':');
    if (colon <= 0) return null; // free text, #number: the server's
    const key = body.slice(0, colon);
    const value = body.slice(colon + 1);
    if (['title', 'author', 'involves', 'follows', 'created', 'updated', 'closed', 'comments', 'in'].includes(key)) return null;
    if (value.includes('@') || value === '') return null; // @me, a sprint's alias: the server knows who and when
    out.push({ key, values: key === 'no' ? [value] : value.split(','), neg });
  }
  return out;
}

const labelsOf = (i: Issue) => [...i.labels, ...(i.inherited ?? [])];

function matches(i: Issue, t: Term): boolean {
  const labels = labelsOf(i);
  const one = (v: string): boolean => {
    switch (t.key) {
      case 'is':
        return i.status === v;
      case 'label':
        return labels.some((l) => l.toLowerCase() === v.toLowerCase());
      case 'no':
        return !labels.some((l) => {
          const c = l.indexOf(':');
          return v === 'label' ? c < 0 : c > 0 && l.slice(0, c) === v;
        });
      default:
        return labels.includes(`${t.key}:${v}`);
    }
  };
  const ok = t.values.some(one);
  return t.neg ? !ok : ok;
}

const time = (s: string | undefined) => (s ? Date.parse(s) : 0);

// order is the server's: oldest first (created, then id), then the sort's
// key, stable; null for a sort answered by the server alone (a field's,
// by its options' order)
function order(list: Issue[], sort: string): Issue[] | null {
  const base = [...list].sort((a, b) => time(a.created_at) - time(b.created_at) || (a.id < b.id ? -1 : a.id > b.id ? 1 : 0));
  if (!sort) return base;
  const [key, dir = 'asc'] = sort.trim().split(/\s+/);
  const sign = dir === 'desc' ? -1 : 1;
  const by: Record<string, (a: Issue, b: Issue) => number> = {
    created: (a, b) => time(a.created_at) - time(b.created_at),
    updated: (a, b) => time(a.updated_at) - time(b.updated_at),
  };
  const cmp = by[key ?? ''];
  if (!cmp) return null;
  return base.sort((a, b) => sign * cmp(a, b));
}

// localIDs is a list's page of ids and its total, from the board kept
// here; null when the filter or the sort is the server's alone
export function localIDs(issues: Iterable<Issue>, filter: string, sort: string, limit: number, offset = 0): { ids: string[]; total: number } | null {
  const terms = parse(filter);
  if (!terms) return null;
  const found: Issue[] = [];
  for (const i of issues) if (terms.every((t) => matches(i, t))) found.push(i);
  const sorted = order(found, sort);
  if (!sorted) return null;
  const ids = sorted.map((i) => i.id);
  return { ids: limit > 0 ? ids.slice(offset, offset + limit) : ids.slice(offset), total: ids.length };
}
