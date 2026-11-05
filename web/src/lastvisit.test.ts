import { test } from 'node:test';
import assert from 'node:assert/strict';
import { changedSince, visit, GAP } from './lastvisit.ts';

test('visit keeps the previous visit while you stay, and moves on after a break', () => {
  const t0 = 1_000_000_000_000;
  // first time here: nothing to compare against
  assert.deepEqual(visit(null, t0), { prev: null, current: t0 });
  // back within the gap (a reload): same visit, same comparison point
  assert.deepEqual(visit({ prev: 5, current: t0 }, t0 + GAP - 1), { prev: 5, current: t0 + GAP - 1 });
  // after a break: what you saw then is the new comparison point
  assert.deepEqual(visit({ prev: 5, current: t0 }, t0 + GAP + 1), { prev: t0, current: t0 + GAP + 1 });
});

test('changedSince picks the issues changed after the comparison point', () => {
  const issues = [
    { id: 'a', updated_at: '2026-10-15T10:00:00Z' },
    { id: 'b', updated_at: '2026-10-15T12:00:00Z' },
    { id: 'c' },
  ];
  assert.deepEqual([...changedSince(issues, Date.parse('2026-10-15T11:00:00Z'))], ['b']);
  assert.deepEqual([...changedSince(issues, null)], []);
});
