// Run with: node --test src/addforge.test.ts
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { canGoOn, guessForge, tableOf } from './addforge.ts';

test('the forge an address says', () => {
  assert.equal(guessForge('https://github.com/o/r'), 'github');
  assert.equal(guessForge('https://gitlab.epfl.ch/g/p'), 'gitlab');
  assert.equal(guessForge('nope'), '');
});

test('a step goes on once filled', () => {
  const d = { target: 'github', url: '', name: '' };
  assert.equal(canGoOn(0, d), true);
  assert.equal(canGoOn(1, d), false);
  assert.equal(canGoOn(1, { ...d, url: 'https://github.com/o/r' }), true);
  assert.equal(canGoOn(1, { ...d, url: 'https://github.com/o/r', name: 'a b' }), false);
});

test('the table: a free name, the project, the host when not the forge’s own', () => {
  assert.deepEqual(tableOf({ target: 'github', url: 'https://github.com/o/r.git', name: '' }, ['gh'], true), {
    name: 'gh2', table: { kind: 'github', project: 'o/r', sync: true },
  });
  assert.deepEqual(tableOf({ target: 'gitlab', url: 'https://gitlab.epfl.ch/g/p/-/issues', name: 'epfl' }, [], false), {
    name: 'epfl', table: { kind: 'gitlab', project: 'g/p', sync: false, host: 'gitlab.epfl.ch' },
  });
});
