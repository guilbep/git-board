// Run with: node --test src/newissue.test.ts
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { assignments } from './newissue.ts';

test('one assignment per single field, one +value per value of a multi field, none for an empty one', () => {
  const multi = (n: string) => n === 'assignee';
  assert.deepEqual(assignments({ status: ' todo ', assignee: 'alice, , @me', estimate: '  ' }, multi), ['status=todo', 'assignee=+alice', 'assignee=+@me']);
});

test('a single field keeps its commas', () => {
  assert.deepEqual(assignments({ sprint: 'a, b' }, () => false), ['sprint=a, b']);
});
