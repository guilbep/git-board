// layout = "roadmap": each issue on a time line, a bar over the dates of
// its sprint (an iteration field, [[sprints.list]]) and a diamond on each
// date field (due). Pure: RoadmapView.vue draws it.
import type { SprintWindow } from './api/board';
import type { Fields } from './fields.ts';

export interface Mark {
  kind: 'span' | 'point';
  field: string;
  label: string;
  start: number; // ms
  end: number; // ms; a point: start
}

const day = 864e5;
const parse = (iso: string) => Date.parse(`${iso}T00:00:00`);

export function marksOf(labels: string[], fields: Fields, sprints: SprintWindow[]): Mark[] {
  const marks: Mark[] = [];
  for (const l of labels) {
    const i = l.indexOf(':');
    if (i < 0) continue;
    const field = l.slice(0, i);
    const value = l.slice(i + 1);
    const type = fields[field]?.type;
    if (type === 'iteration') {
      const s = sprints.find((x) => x.slug === value);
      if (s) marks.push({ kind: 'span', field, label: s.title, start: parse(s.start), end: parse(s.end) });
    } else if (type === 'date' && /^\d{4}-\d{2}-\d{2}$/.test(value)) {
      const t = parse(value);
      marks.push({ kind: 'point', field, label: `${field[0]!.toUpperCase()}${field.slice(1)} ${value}`, start: t, end: t });
    }
  }
  return marks;
}

// span is the time line's extent: every mark and today, a few days of air
// on each side.
export function span(marks: Mark[], today: number): { from: number; to: number } {
  let from = today;
  let to = today;
  for (const m of marks) {
    from = Math.min(from, m.start);
    to = Math.max(to, m.end);
  }
  const pad = Math.max(3 * day, (to - from) * 0.03);
  return { from: from - pad, to: to + pad };
}

const short = new Intl.DateTimeFormat(undefined, { day: 'numeric', month: 'short' });
const month = new Intl.DateTimeFormat(undefined, { month: 'short', year: '2-digit' });

// ticks: every Monday up to ~4 months, else the first of each month.
export function ticks(from: number, to: number): { at: number; label: string }[] {
  const res: { at: number; label: string }[] = [];
  const d = new Date(from);
  d.setHours(0, 0, 0, 0);
  if (to - from <= 120 * day) {
    d.setDate(d.getDate() + ((8 - d.getDay()) % 7)); // the next Monday, or this one
    for (; d.getTime() <= to; d.setDate(d.getDate() + 7)) res.push({ at: d.getTime(), label: short.format(d) });
  } else {
    d.setDate(1);
    d.setMonth(d.getMonth() + 1);
    for (; d.getTime() <= to; d.setMonth(d.getMonth() + 1)) res.push({ at: d.getTime(), label: month.format(d) });
  }
  return res;
}
