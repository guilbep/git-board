import { test } from 'node:test';
import assert from 'node:assert/strict';
import { clampWidth } from './panelsize.ts';

test('clampWidth keeps the panel between 28rem and the window less 4rem', () => {
  assert.equal(clampWidth(300, 1500), 448); // 28rem
  assert.equal(clampWidth(900, 1500), 900);
  assert.equal(clampWidth(1490, 1500), 1436); // 1500 − 4rem
  assert.equal(clampWidth(900, 400), 400); // a phone: the whole width
});
