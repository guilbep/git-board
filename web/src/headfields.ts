// The fields line under the title (README 16a, the plan's Calls taken 1):
// the status first (the stepper's), "by #N" for a blocked-by value, then
// the top fields: assignee, prio, sprint and kind when the project has
// them, else the first four. The other fields set count as "+N more"; the
// top ones unset read "Set …". Pure: IssueHead draws it.
export interface FieldLine {
  order: string[]; // the top fields, set or not, in the line's order
  top: { name: string; values: string[] }[];
  unset: string[];
  more: string[];
  blockedBy: string[];
}

const PREFERRED = ['assignee', 'prio', 'sprint', 'kind'];
const SPECIAL = ['status', 'blocked-by'];

const valuesOf = (labels: string[], name: string) => labels.filter((l) => l.startsWith(`${name}:`)).map((l) => l.slice(name.length + 1));

export function fieldLine(names: string[], labels: string[]): FieldLine {
  const others = names.filter((n) => !SPECIAL.includes(n));
  const preferred = PREFERRED.filter((n) => others.includes(n));
  const top = preferred.length ? preferred : others.slice(0, 4);
  const has = (n: string) => valuesOf(labels, n).length > 0;
  return {
    order: top,
    top: top.filter(has).map((name) => ({ name, values: valuesOf(labels, name) })),
    unset: top.filter((n) => !has(n)),
    more: others.filter((n) => !top.includes(n) && has(n)),
    blockedBy: valuesOf(labels, 'blocked-by'),
  };
}

// The labels line (README 16b, open point 7): an issue's plain labels (a
// field's value has a colon; a flat kind reads as kind:bug), those of the
// project's [labels] list picked, the others outside it, to show with a
// warning. In the list's order, then the issue's.
export function labelLine(labels: string[], list: string[]): { picked: string[]; outside: string[] } {
  const plain = labels.filter((l) => !l.includes(':'));
  return { picked: list.filter((l) => plain.includes(l)), outside: plain.filter((l) => !list.includes(l)) };
}

// A date field against today, in a date editor (the old side column's
// note): "2 days late", "Today", "In 15 days"; '' for what isn't a date,
// a day that doesn't exist too (2026-02-30, which Date rolls into March).
// Counted between noons, so a clock change never makes a day of 23 hours.
export function dueNote(iso: string, today: Date): string {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(iso);
  if (!m) return '';
  const [y, mo, d] = [Number(m[1]), Number(m[2]) - 1, Number(m[3])];
  const due = new Date(y, mo, d, 12);
  if (due.getFullYear() !== y || due.getMonth() !== mo || due.getDate() !== d) return '';
  const days = Math.round((due.getTime() - new Date(today.getFullYear(), today.getMonth(), today.getDate(), 12).getTime()) / 864e5);
  if (days < 0) return `${-days} day${days === -1 ? '' : 's'} late`;
  if (days === 0) return 'Today';
  return `In ${days} day${days === 1 ? '' : 's'}`;
}
