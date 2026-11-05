// Each field value's color, as v1's server gave them (config.Colors): the
// color .git-board.toml sets, else one its words name (done green, blocked
// red…), else one of four spread over the rest; a number has none, and a
// field that isn't a select or an iteration only its configured ones.
// v2's GET /api/fields answers the configured colors alone (api.md,
// Fields): the page colors the rest. Pure: node runs valuecolors.test.ts.

const aliases: Record<string, string> = { gray: 'grey', yellow: 'amber', violet: 'purple' };

const keywords: [string, string[]][] = [
  ['green', ['done', 'validated', 'closed', 'shipped', 'released', 'merged', 'resolved', 'complete', 'completed', 'accepted', 'approved', 'fixed']],
  ['red', ['blocked', 'bug', 'fix', 'urgent', 'critical', 'p0', 'incident', 'broken']],
  ['orange', ['p1', 'high', 'test', 'tests', 'testing']],
  ['purple', ['review', 'reviewing', 'qa']],
  ['pink', ['validation', 'approval', 'sign-off', 'signoff']],
  ['amber', ['waiting', 'wait', 'hold', 'pending', 'p2', 'medium', 'comite', 'comité', 'committee', 'triage', 'paused']],
  ['blue', ['progress', 'doing', 'wip', 'active', 'started', 'dev', 'development', 'ongoing']],
  ['teal', ['ready', 'feat', 'feature', 'enhancement', 'planned', 'next']],
  ['grey', ['backlog', 'todo', 'new', 'icebox', 'someday', 'low', 'p3', 'p4', 'draft', 'idea', 'later', 'wontfix']],
];

const spread = ['blue', 'purple', 'teal', 'pink'];

const isNumber = (v: string) => v.trim() !== '' && !Number.isNaN(Number(v));
const norm = (c: string) => aliases[c.trim().toLowerCase()] ?? c.trim().toLowerCase();

function configured(colors: Record<string, string> | undefined, value: string): string {
  for (const [v, c] of Object.entries(colors ?? {})) if (v.toLowerCase() === value.toLowerCase()) return norm(c);
  return '';
}

function byWord(value: string): string {
  const words = value.toLowerCase().split(/[^\p{L}\p{N}]+/u).filter(Boolean);
  return keywords.find(([, ws]) => words.some((w) => ws.includes(w)))?.[0] ?? '';
}

export interface Colorable {
  type?: string;
  colors?: Record<string, string> | undefined;
}

// valueColors: field → value → color, for the values in use and the
// fields' options
export function valueColors(fields: Record<string, Colorable>, values: Record<string, string[]>): Record<string, Record<string, string>> {
  const out: Record<string, Record<string, string>> = {};
  for (const name of new Set([...Object.keys(fields), ...Object.keys(values)])) {
    const f = fields[name];
    const vs = values[name] ?? [];
    const res: Record<string, string> = {};
    const select = !f?.type || f.type === 'select' || f.type === 'iteration';
    const rest: string[] = [];
    for (const v of vs) {
      const c = configured(f?.colors, v) || (select && !isNumber(v) ? byWord(v) : '');
      if (c) res[v] = c;
      else if (select && !isNumber(v)) rest.push(v);
    }
    [...new Set(rest)].sort().forEach((v, i) => (res[v] = spread[i % spread.length]!));
    out[name] = res;
  }
  return out;
}

// hexRules is a CSS rule per #rrggbb color of the board's values (a
// GitHub label's): it paints the dot only; the chip keeps the ink and a
// plain background, since a label's color (ebcf04, ffffff) is often
// unreadable as text. colorClass names them gb-c-x<hex>.
export function hexRules(colors: Record<string, Record<string, string>>): string {
  const hex = new Set(Object.values(colors).flatMap((values) => Object.values(values).filter((c) => /^#[0-9a-f]{6}$/.test(c))));
  return [...hex].map((c) => `.gb-c-x${c.slice(1)}{--c-bg:var(--bg-subtle);--c-fg:var(--fg);--c-dot:${c}}`).join('\n');
}
