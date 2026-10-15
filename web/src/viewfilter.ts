// composeFilter is the filter a view saves from what the board shows: its
// filter expression, each ticked facet as one condition (several values:
// a,b, either; "(none)": no:field), then the search box. A facet mixing
// "(none)" with values has no single condition: it is left out, named in
// skipped.
import { NONE, PLAIN, STATE, type Selection } from './facets.ts';

const quote = (v: string) => (/[\s"]/.test(v) ? `"${v.replace(/"/g, '')}"` : v);

export function composeFilter(base: string, sel: Selection, q: string): { filter: string; skipped: string[] } {
  const terms = base.trim() ? [base.trim()] : [];
  const has = (t: string) => ` ${terms.join(' ')} `.includes(` ${t} `);
  const skipped: string[] = [];
  for (const [key, values] of Object.entries(sel)) {
    if (!values.length) continue;
    const none = values.includes(NONE);
    let term: string;
    if (none && values.length > 1) {
      skipped.push(key);
      continue;
    } else if (none) {
      term = `no:${key === PLAIN ? 'label' : key}`;
    } else {
      term = `${key === STATE ? 'is' : key}:${values.map(quote).join(',')}`;
    }
    if (!has(term)) terms.push(term);
  }
  if (q.trim()) terms.push(q.trim());
  return { filter: terms.join(' '), skipped };
}
