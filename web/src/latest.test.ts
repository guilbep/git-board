// Run with: node --test web/src/latest.test.ts
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { latestOnly } from './latest.ts';

test('the request asked last is current, an earlier one no longer', () => {
  const seq = latestOnly();
  const team = seq.start();
  const me = seq.start();
  assert.equal(team(), false); // Team answers last: dropped
  assert.equal(me(), true);
});

test('skip makes a request under way stale, the next one current', () => {
  const seq = latestOnly();
  const load = seq.start();
  seq.skip(); // a write answered meanwhile
  assert.equal(load(), false);
  assert.equal(seq.start()(), true);
});

test('two sequences are independent', () => {
  const a = latestOnly();
  const b = latestOnly();
  const x = a.start();
  b.start();
  assert.equal(x(), true);
});
