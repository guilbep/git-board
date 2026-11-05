import { test } from 'node:test';
import assert from 'node:assert/strict';
import { marksOf, span, ticks } from './roadmap.ts';

const sprints = [
  { slug: 's1', title: 'Sprint 1', start: '2026-10-05', end: '2026-10-19' },
  { slug: 's2', title: 'Sprint 2', start: '2026-10-19', end: '2026-11-02' },
];
const fields = {
  sprint: { single: true, options: ['s1', 's2'], numeric: false, type: 'iteration' },
  due: { single: true, options: null, numeric: false, type: 'date' },
  status: { single: true, options: ['todo'], numeric: false },
};

test('marksOf: a bar over the sprint, a diamond on each date field', () => {
  const m = marksOf(['sprint:s2', 'due:2026-10-30', 'status:todo'], fields, sprints);
  assert.deepEqual(
    m.map((x) => [x.kind, x.field, x.label]),
    [
      ['span', 'sprint', 'Sprint 2'],
      ['point', 'due', 'Due 2026-10-30'],
    ],
  );
  assert.equal(Math.round((m[0]!.end - m[0]!.start) / 864e5), 14); // across a DST change too
  assert.deepEqual(marksOf(['sprint:gone', 'status:todo'], fields, sprints), []);
});

test('span covers the marks and today, padded; ticks are weeks or months', () => {
  const today = Date.parse('2026-10-15T12:00:00');
  const s = span([{ kind: 'span', field: 'sprint', label: '', start: Date.parse('2026-10-05'), end: Date.parse('2026-11-02') }], today);
  assert.ok(s.from < Date.parse('2026-10-05') && s.to > Date.parse('2026-11-02'));
  const weekly = ticks(s.from, s.to);
  assert.ok(weekly.length >= 4 && weekly.every((t) => new Date(t.at).getDay() === 1), 'Mondays');
  const monthly = ticks(Date.parse('2026-01-01'), Date.parse('2026-12-31'));
  assert.ok(monthly.length >= 11 && monthly.every((t) => new Date(t.at).getDate() === 1), 'first days');
});

test('niceTicks are round', async () => {
  const { niceTicks } = await import('./chartsize.ts');
  assert.deepEqual(niceTicks(198), [0, 50, 100, 150, 200]);
  assert.deepEqual(niceTicks(25), [0, 10, 20, 30]);
  assert.deepEqual(niceTicks(0), [0, 1]);
});
