// Run with: node --test web/src/timeline.test.ts
import assert from 'node:assert/strict';
import { test } from 'node:test';
import type { IssueEvent } from './api/board';
import { steps } from './replay.ts';
import { timeline } from './timeline.ts';

const who = (name: string) => ({ id: name, name });
const day = (d: number) => `2026-10-${String(d).padStart(2, '0')}T00:00:00Z`;
const events: IssueEvent[] = [
  { kind: 'created', id: 'c0', author: who('ann'), at: day(1), body: 'x' },
  { kind: 'labels', id: 'l1', author: who('ann'), at: day(3), added: ['status:backlog'] },
  { kind: 'comment', id: 'c1', author: who('bob'), at: day(4), body: 'y' },
  { kind: 'labels', id: 'l2', author: who('bob'), at: day(6), added: ['status:ready'], removed: ['status:backlog'] },
  { kind: 'status', id: 's1', author: who('bob'), at: day(8), status: 'closed' },
  { kind: 'status', id: 's2', author: who('ann'), at: day(9), status: 'open' },
];

test('timeline: the status band, closed spans and markers on one axis', () => {
  const t = timeline(steps(events), Date.parse(day(11)));
  assert.equal(t.start, Date.parse(day(1)));
  assert.equal(t.end, Date.parse(day(11)));
  // 10 days: day 1→3 not set (20%), 3→6 backlog (30%), 6→11 ready (50%)
  assert.deepEqual(
    t.segments.map((s) => [s.value, Math.round(s.left), Math.round(s.width)]),
    [['', 0, 20], ['backlog', 20, 30], ['ready', 50, 50]],
  );
  assert.deepEqual(t.closed.map((c) => [Math.round(c.left), Math.round(c.width)]), [[70, 10]]);
  // texts on one lane, changes on the other, each pointing at its step
  assert.deepEqual(t.markers.map((m) => [m.lane, m.kind, Math.round(m.left), m.index]), [
    ['texts', 'created', 0, 0],
    ['changes', 'labels', 20, 1],
    ['texts', 'comment', 30, 2],
    ['changes', 'labels', 50, 3],
    ['changes', 'status', 70, 4],
    ['changes', 'status', 80, 5],
  ]);
});

test('timeline: one instant does not divide by zero', () => {
  const t = timeline(steps(events.slice(0, 1)), Date.parse(day(1)));
  assert.equal(t.markers[0]!.left, 0);
  assert.equal(t.segments.length, 1);
  assert.equal(t.segments[0]!.width, 100);
});
