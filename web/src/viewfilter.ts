// The filter a facet's ticks write: its term after the rest of the filter
// (Facet.rest, the server's: the page parses nothing). Several values: a,b,
// either; "(none)": no:field; "(none)" with values ("all but validated"):
// the values left out, negated (-status:validated), when options lists
// the facet's values; without them it can't be said, and composeFilter
// names the facet in skipped.
import { NONE, PLAIN, STATE, type Facet, type Selection } from './facets.ts';

const quote = (v: string) => (/[\s"]/.test(v) ? `"${v.replace(/"/g, '')}"` : v);

function term(key: string, values: string[], options?: string[]): string | null {
  if (!values.length) return '';
  const none = values.includes(NONE);
  const name = key === STATE ? 'is' : key;
  if (none && values.length > 1) {
    if (!options) return null;
    const left = options.filter((v) => v !== NONE && !values.includes(v));
    return left.length ? `-${name}:${left.map(quote).join(',')}` : ''; // every value and none: no condition
  }
  if (none) return `no:${key === PLAIN ? 'label' : key}`;
  return `${name}:${values.map(quote).join(',')}`;
}

export function withFacet(f: Facet, values: string[]): string {
  return [f.rest, term(f.key, values, f.options.map(([v]) => v)) ?? ''].filter(Boolean).join(' ');
}

// composeFilter is an older link's filter (?q=…&f.status=ready): the view's
// filter, each ticked facet as one condition, then its search words.
export function composeFilter(
  base: string,
  sel: Selection,
  q: string,
  options: Record<string, string[]> = {},
): { filter: string; skipped: string[] } {
  const terms = base.trim() ? [base.trim()] : [];
  const has = (t: string) => ` ${terms.join(' ')} `.includes(` ${t} `);
  const skipped: string[] = [];
  for (const [key, values] of Object.entries(sel)) {
    const t = term(key, values, options[key]);
    if (t === null) skipped.push(key);
    else if (t && !has(t)) terms.push(t);
  }
  if (q.trim()) terms.push(q.trim());
  return { filter: terms.join(' '), skipped };
}
