// Run with: node --test web/src/timeline.test.ts
import assert from 'node:assert/strict';
import { test } from 'node:test';
import type { IssueEvent } from './api/board';
import { steps } from './replay.ts';
import { timeline } from './timeline.ts';
import { segments } from './flow.ts';

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
    ['versions', 'created', 0, 0],
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

const desc: IssueEvent = {
  kind: 'created', id: 'd0', author: who('ann'), at: day(1), body: '- [x] a', edited: true,
  history: [
    { author: who('ann'), at: day(1), body: '- [ ] a' },
    { author: who('ann'), at: day(2), body: '- [ ] b' },
    { author: who('ann'), at: day(3), body: '- [x] b' },
  ],
};
const said: IssueEvent = { kind: 'comment', id: 'k1', author: who('bob'), at: day(4), body: 'y', history: [
  { author: who('bob'), at: day(4), body: 'y' }, { author: who('bob'), at: day(5), body: 'z' },
] };

test('the description goes to the versions lane', () => {
  const t = timeline(steps([desc, said]), Date.parse(day(11)));
  assert.deepEqual(t.markers.map((m) => [m.lane, m.tick ?? false]), [
    ['versions', false], ['versions', false], ['versions', true], ['texts', false], ['texts', false],
  ]);
});

test('one version is one diamond', () => {
  const t = timeline(steps(events), Date.parse(day(11)));
  assert.equal(t.markers.filter((m) => m.lane === 'versions').length, 1);
});

const stages = {
  before: ['backlog'], ready: 'ready', in_progress: ['in-progress'], in_review: ['in-review'],
  validation: [], closing: 'done', criteria: [], kinds: {}, guessed: false,
};
test('the step bars run from each opening to the next', () => {
  const t = timeline(steps(events), Date.parse(day(11)), 'status', segments(events.slice(0, 4), stages, 2));
  // step 1 from the opening, step 2 from day 6 (ready) to now
  assert.deepEqual(t.bars.map((b) => [b.n, Math.round(b.left), Math.round(b.width)]), [[1, 0, 50], [2, 50, 50]]);
});

test('no cycle, no bars', () => {
  assert.deepEqual(timeline(steps(events), Date.parse(day(11)), 'status', segments(events, stages, 0)).bars, []);
  assert.deepEqual(timeline(steps(events), Date.parse(day(11))).bars, []);
});
