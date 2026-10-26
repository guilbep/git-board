// The project's own numbers, CO2-3075, set up from Settings › Numbers
// (GET /api/numbers). Pure, no "@/" imports: node runs numbers.test.ts on
// it.

const prefixRe = /^[A-Za-z][A-Za-z0-9]*$/; // as store's prefixRe

// prefixProblem says what's wrong with a prefix, '' when the server takes it
export function prefixProblem(p: string): string {
  if (!p) return 'A prefix, like CO2.';
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

// suggestStart is one above every number known: the issues' forge
// numbers, and GitHub's highest issue or pull request when it answered
export const suggestStart = (highest: number, github = 0) => Math.max(highest, github) + 1;

// startProblem says what's wrong with a start, '' when it's fine: at or
// below a number an issue has on a forge, a later import collides
export function startProblem(start: number, highest: number): string {
  if (!Number.isInteger(start) || start < 1) return 'A whole number, 1 or more.';
  return start > highest ? '' : `Start above ${highest}: that number is taken on a forge.`;
}

const issues = (n: number) => `${n} issue${n === 1 ? '' : 's'}`;
const span = (prefix: string, from: number, count: number) => (count === 1 ? `${prefix}-${from}` : `${prefix}-${from} … ${prefix}-${from + count - 1}`);

// numbersPreview says what starting at start does to the issues on origin,
// each by its GitHub number (0 for none): those below start keep it, the
// others take the counter's; unpushed get theirs with their push
export function numbersPreview(prefix: string, start: number, github: number[], unpushed: number): string {
  const kept = github.filter((n) => n > 0 && n < start);
  const count = github.length - kept.length;
  let s = !github.length ? 'No issue on origin yet' : count ? `Your ${issues(count)} on origin will become ${span(prefix, start, count)}` : 'No issue on origin needs a number';
  if (kept.length) {
    const k = Math.max(...kept);
    s += `; #${k} stays ${prefix}-${k}`;
    if (kept.length > 1) s += `, as ${kept.length - 1} more ${kept.length === 2 ? 'keeps its' : 'keep their'} GitHub number`;
  }
  s += '.';
  if (unpushed) s += ` ${issues(unpushed)} only here ${unpushed === 1 ? 'gets its' : 'get their'} number when pushed.`;
  return s;
}

// numbered says what a numbering did, next being the counter's next number after it
export function numbered(prefix: string, next: number, done: number): string {
  return done ? `${issues(done)} numbered: ${span(prefix, next - done, done)}.` : 'No issue needed a number.';
}

// numbersSummary is the Overview's line
export function numbersSummary(n: { on: boolean; prefix?: string | undefined; next?: number | undefined }): string {
  return n.on ? `${n.prefix} · next ${n.prefix}-${n.next}.` : 'Off: issues go by their forge numbers.';
}
