import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fit } from './navfit.ts';

test('fit: every view shows when there is room', () => {
  assert.deepEqual(fit([50, 60, 70], 200, 0, 40), { shown: [0, 1, 2], hidden: [] });
});

test('fit: the rest go under "N more", in order, the menu taking its room', () => {
  // 50+60+70 = 180 > 150; with the menu's 40 only 50+60 fit in 110
  assert.deepEqual(fit([50, 60, 70, 30], 150, -1, 40), { shown: [0, 1], hidden: [2, 3] });
});

test('fit: the active view always shows, in the last shown slot', () => {
  assert.deepEqual(fit([50, 60, 70, 30], 150, 3, 40), { shown: [0, 3], hidden: [1, 2] });
});

test('fit: a wide active view that would push the others out shows alone', () => {
  assert.deepEqual(fit([50, 60, 200], 150, 2, 40), { shown: [2], hidden: [0, 1] });
});

test('fit: no room at all keeps the active view, else none', () => {
  assert.deepEqual(fit([50, 60], 10, 1, 40), { shown: [1], hidden: [0] });
  assert.deepEqual(fit([50, 60], 10, -1, 40), { shown: [], hidden: [0, 1] });
});
