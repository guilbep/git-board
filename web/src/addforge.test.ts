// Run with: node --test src/addforge.test.ts
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { canGoOn, guessForge } from './addforge.ts';

test('the forge an address says', () => {
  assert.equal(guessForge('https://github.com/o/r'), 'github');
  assert.equal(guessForge('https://gitlab.epfl.ch/g/p'), 'gitlab');
  assert.equal(guessForge('nope'), '');
});

test('a step goes on once filled', () => {
  const d = { target: 'github', url: '', name: '', token: '' };
  assert.equal(canGoOn(0, d), true);
  assert.equal(canGoOn(1, d), false);
  assert.equal(canGoOn(1, { ...d, url: 'https://github.com/o/r' }), true);
  assert.equal(canGoOn(1, { ...d, url: 'https://github.com/o/r', name: 'a b' }), false);
});
