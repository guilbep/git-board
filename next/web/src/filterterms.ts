// The filter's terms, read in the page: v1's server answered the filters
// panel whole (GET /api/facets); v2 answers counts (GET /api/labels) for a
// filter, and the page says which terms are a facet's, what it picks, and
// what the rest of the filter is. The grammar is the CLI's
// (docs/guide/configuration.md, Filters). Pure: node runs
// filterterms.test.ts on it.

export const NONE = '(none)';
export const STATE = 'is';
export const PLAIN = 'label';

export interface Term {
  text: string; // as typed
  neg: boolean;
  key: string; // '' for a word
  value: string; // unquoted
}

// terms splits a filter at its spaces, a quoted value kept whole
export function terms(filter: string): Term[] {
  const out: Term[] = [];
  for (const m of filter.matchAll(/(?:[^\s"]+|"[^"]*")+/g)) {
    const text = m[0];
    const neg = text.startsWith('-') && text.length > 1;
    const body = neg ? text.slice(1) : text;
    const colon = body.indexOf(':');
    const key = colon > 0 && !body.startsWith('"') ? body.slice(0, colon) : '';
    const value = (key ? body.slice(colon + 1) : body).replace(/"/g, '');
    out.push({ text, neg, key, value });
  }
  return out;
}

// owns: the terms that are a facet's, which a tick rewrites
export function owns(facet: string, t: Term): boolean {
  if (!t.key) return false;
  if (t.key === 'is') return facet === STATE;
  if (t.key === 'no') return t.value === facet;
  if (t.key === 'label') return facet === PLAIN && !t.value.includes(':');
  return t.key === facet;
}

// rest is the filter without a facet's terms
export function rest(filter: string, facet: string): string {
  return terms(filter).filter((t) => !owns(facet, t)).map((t) => t.text).join(' ');
}

// picked are the values a facet's terms let through, out of options (and
// NONE): none when the filter has no term on it
export function picked(filter: string, facet: string, options: string[]): string[] {
  const own = terms(filter).filter((t) => owns(facet, t));
  if (!own.length) return [];
  const all = [...options.filter((o) => o !== NONE), NONE];
  return all.filter((v) =>
    own.every((t) => {
      const values = t.key === 'no' ? [NONE] : t.value.split(',');
      const has = t.key === 'no' ? v === NONE : values.includes(v);
      return t.neg ? !has : has;
    }),
  ).concat(own.flatMap((t) => (t.key === 'no' || t.neg ? [] : t.value.split(','))).filter((v) => v && !all.includes(v)));
}

// a filter term naming value of field, for a lane's own list
const quote = (v: string) => (/[\s",]/.test(v) ? `"${v.replace(/"/g, '')}"` : v);
export function laneFilter(filter: string, field: string, value: string): string {
  const term = value === NONE ? `no:${field}` : field === STATE ? `is:${value}` : `${field}:${quote(value)}`;
  return filter.trim() ? `${filter.trim()} ${term}` : term;
}

// the facets' keys: is, the fields, the other namespaces in use, the plain
// labels
export function facetKeys(fields: string[], inUse: string[]): string[] {
  return [STATE, ...fields, ...inUse.filter((k) => k !== PLAIN && !fields.includes(k)).sort(), PLAIN];
}
