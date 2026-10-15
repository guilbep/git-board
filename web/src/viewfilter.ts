// composeFilter is the filter a view saves from what the board shows: its
// filter expression, each ticked facet as one condition (several values:
// a,b, either; "(none)": no:field), then the search box. "(none)" with
// values ("all but validated") is the values left out, negated
// (-status:validated), when options lists the facet's values; without
// them it can't be said, and the facet is named in skipped.
import { NONE, PLAIN, STATE, type Selection } from './facets.ts';

const quote = (v: string) => (/[\s"]/.test(v) ? `"${v.replace(/"/g, '')}"` : v);

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
    if (!values.length) continue;
    const none = values.includes(NONE);
    let term: string;
    const name = key === STATE ? 'is' : key;
    if (none && values.length > 1) {
      const left = (options[key] ?? []).filter((v) => v !== NONE && !values.includes(v));
      if (!options[key]) {
        skipped.push(key);
        continue;
      }
      if (!left.length) continue; // every value and none: no condition
      term = `-${name}:${left.map(quote).join(',')}`;
    } else if (none) {
      term = `no:${key === PLAIN ? 'label' : key}`;
    } else {
      term = `${name}:${values.map(quote).join(',')}`;
    }
    if (!has(term)) terms.push(term);
  }
  if (q.trim()) terms.push(q.trim());
  return { filter: terms.join(' '), skipped };
}
