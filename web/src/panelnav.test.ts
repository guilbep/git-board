// Run with: node --test web/src/panelnav.test.ts
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { neighbours } from './panelnav.ts';

test('in the list: the issues before and after it', () => {
  assert.deepEqual(neighbours(['a', 'b', 'c'], 'b', 1), { at: 1, prev: 'a', next: 'c' });
  assert.deepEqual(neighbours(['a', 'b', 'c'], 'a', 0), { at: 0, prev: undefined, next: 'b' });
  assert.deepEqual(neighbours(['a', 'b', 'c'], 'c', 2), { at: 2, prev: 'b', next: undefined });
});

test('closed out of an is:open view: j opens the issue that took its place, k the one before', () => {
  // b was at 1, then closed: the list is a, c, d
  assert.deepEqual(neighbours(['a', 'c', 'd'], 'b', 1), { at: -1, prev: 'a', next: 'c' });
});

test('the last one gone: j has nowhere to go, k the new last one', () => {
  assert.deepEqual(neighbours(['a', 'b'], 'c', 2), { at: -1, prev: 'b', next: undefined });
  assert.deepEqual(neighbours(['a'], 'c', 5), { at: -1, prev: 'a', next: undefined });
});

test('an issue never in the list (opened from ⌘K): no neighbours', () => {
  assert.deepEqual(neighbours(['a', 'b'], 'x', -1), { at: -1 });
});
