// The project's own numbers, CO2-3075, set up from Settings › Numbers
// (POST /api/numbers). Pure, no "@/" imports: node runs numbers.test.ts on
// it.

const prefixRe = /^[A-Za-z][A-Za-z0-9]*$/; // as store's prefixRe

// prefixProblem says what's wrong with a prefix, '' when the server takes it
export function prefixProblem(p: string): string {
  if (!p) return 'A prefix, like ABC.';
  return prefixRe.test(p) ? '' : 'A letter, then letters or digits: no dash, no space.';
}

// suggestPrefix is a prefix from the repository's name: the initials of
// its words (git-board → GB), a first word with a digit kept whole
// (co2-calculator → CO2), one word its first three letters (gitboard →
// GIT); '' when the name has no letter.
export function suggestPrefix(name: string): string {
  const words = name.replace(/^[^A-Za-z]+/, '').split(/[^A-Za-z0-9]+/).filter(Boolean);
  const first = words[0];
  if (!first) return '';
  const p = /\d/.test(first) ? first : words.length > 1 ? words.map((w) => w[0]).join('') : first.slice(0, 3);
  return p.toUpperCase().slice(0, 5);
}
