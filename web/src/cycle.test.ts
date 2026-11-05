// Run with: node --test src/cycle.test.ts
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { kindOf, listOf } from './cycle.ts';

test('the rail is the list’s, else the default seven', () => {
  assert.deepEqual(listOf(undefined).map((s) => s.id), ['intent', 'risk', 'plan', 'tests', 'code', 'review', 'verify']);
  const steps = [{ id: 'intent', kind: 'intent', name: 'Intent' }, { id: 'qa', kind: 'signoff', name: 'QA', status: 'qa' }];
  assert.deepEqual(listOf({ steps } as never), steps);
});

test('a step’s kind as its number: a sign-off is 7', () => {
  assert.equal(kindOf({ id: 'review', kind: 'review', name: 'Review' }), 6);
  assert.equal(kindOf({ id: 'qa', kind: 'signoff', name: 'QA' }), 7);
});
