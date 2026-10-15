import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { Bucket, Issue } from './api/board';
import { boardLanes, laneChoices, limitState, pickCards, toggleLane } from './lanes.ts';

const issue = (id: string): Issue => ({ id, title: id, status: 'open', labels: [] });
const b = (group: string, ...ids: string[]): Bucket => ({ group, issues: ids.map(issue) });

test('boardLanes shows the lanes in order and counts the rest', () => {
  const buckets = [b('backlog', 'a', 'b'), b('ready', 'c'), b('in-progress', 'd'), b('(none)', 'e', 'f', 'g')];
  const { shown, hidden } = boardLanes(buckets, ['in-progress', 'ready', 'in-review']);
  assert.deepEqual(shown.map((x) => `${x.group}:${x.issues.length}`), ['in-progress:1', 'ready:1', 'in-review:0']);
  assert.equal(hidden, 5);
  assert.deepEqual(boardLanes(buckets, undefined), { shown: buckets, hidden: 0 });
});

test('laneChoices lists the lanes first, then the other values', () => {
  assert.deepEqual(laneChoices(['qa', 'ready'], ['backlog', 'ready', 'qa', '(none)']), ['qa', 'ready', 'backlog', '(none)']);
  assert.deepEqual(laneChoices(undefined, ['backlog', 'ready']), ['backlog', 'ready']);
});

test('toggleLane starts from every value and comes back to none', () => {
  const all = ['backlog', 'ready', 'qa'];
  assert.deepEqual(toggleLane(undefined, 'backlog', all), ['ready', 'qa']);
  assert.deepEqual(toggleLane(['ready', 'qa'], 'backlog', all), ['ready', 'qa', 'backlog']);
  assert.equal(toggleLane(['backlog', 'ready'], 'qa', all), undefined); // every value, in order: no lanes of its own
});

test('limitState says at or over a lane limit', () => {
  assert.equal(limitState(3, 5), '');
  assert.equal(limitState(5, 5), 'at');
  assert.equal(limitState(6, 5), 'over');
  assert.equal(limitState(9, undefined), '');
});

test('pickCards toggles with Cmd and takes a range with Shift', () => {
  const order = ['a', 'b', 'c', 'd'];
  assert.deepEqual(pickCards([], 'b', order, { toggle: true }), ['b']);
  assert.deepEqual(pickCards(['b'], 'b', order, { toggle: true }), []);
  assert.deepEqual(pickCards(['b'], 'd', order, { range: true, anchor: 'b' }), ['b', 'c', 'd']);
  assert.deepEqual(pickCards(['x'], 'c', order, { range: true, anchor: 'a' }), ['x', 'a', 'b', 'c']);
});
