import { test } from 'node:test';
import assert from 'node:assert/strict';
import { initials } from './initials.ts';

test('initials ignore a login in parentheses and punctuation', () => {
  assert.equal(initials('Guilbert Pierre (guilbep)'), 'GP');
  assert.equal(initials('Pierre Guilbert'), 'PG');
  assert.equal(initials('charlottegiseleweil'), 'CH');
  assert.equal(initials('martina-gallato'), 'MG');
  assert.equal(initials('Élodie Ünal'), 'ÉÜ');
  assert.equal(initials('(guilbep)'), 'GU');
  assert.equal(initials('  '), '?');
});
