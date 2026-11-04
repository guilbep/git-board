// Run with: node --test web/src/newissue.test.ts
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { fieldWrites } from './newissue.ts';

test('one write per single field, one per value of a multi field, none for an empty one', () => {
  const multi = (n: string) => n === 'assignee';
  assert.deepEqual(fieldWrites({ status: ' todo ', assignee: 'alice, , @me', estimate: '  ' }, multi), [
    { name: 'status', value: 'todo' },
    { name: 'assignee', value: 'alice' },
    { name: 'assignee', value: '@me' },
  ]);
});

test('a single field keeps its commas', () => {
  assert.deepEqual(fieldWrites({ sprint: 'a, b' }, () => false), [{ name: 'sprint', value: 'a, b' }]);
});
