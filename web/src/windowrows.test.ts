// Run with: node --test src/windowrows.test.ts
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { LONG, windowRange } from './windowrows.ts';

test('a short list renders whole', () => {
  assert.deepEqual(windowRange(LONG, -50_000, 900, 40), [0, LONG]);
});

test('a long one, a screen around the viewport', () => {
  // at its top: the first screens
  assert.deepEqual(windowRange(5000, 200, 900, 40), [0, 41]);
  // 4,000 px down: rows 100 on, a screen (23 rows) above and two below
  assert.deepEqual(windowRange(5000, -4000, 900, 40), [77, 146]);
  // past its end: nothing more than it has
  assert.deepEqual(windowRange(5000, -1_000_000, 900, 40), [5000, 5000]);
});
