// Run with: node --test src/local/query.test.ts
import assert from 'node:assert/strict';
import { test } from 'node:test';
import type { Issue } from '../api/types.ts';
import { localIDs } from './query.ts';

const at = (d: number) => new Date(Date.UTC(2026, 0, d)).toISOString();
const is = (id: string, d: number, status: string, labels: string[], updated = d, inherited?: string[]): Issue =>
  ({ id, title: id, shown_title: id, status, labels, inherited, created_at: at(d), updated_at: at(updated), comments: 0, version: '' }) as Issue;
const board = [
  is('c', 3, 'open', ['status:ready', 'prio:p1', 'bug']),
  is('a', 1, 'open', ['status:backlog'], 9),
  is('b', 2, 'closed', ['status:ready', 'Bug']),
  is('d', 4, 'open', [], 4, ['role:lead']),
];
const ids = (f: string, sort = '', limit = 0) => localIDs(board, f, sort, limit)?.ids;

test('the terms a board is made of, as the server matches them', () => {
  assert.deepEqual(ids(''), ['a', 'b', 'c', 'd']); // oldest first
  assert.deepEqual(ids('is:open'), ['a', 'c', 'd']);
  assert.deepEqual(ids('is:open,closed'), ['a', 'b', 'c', 'd']);
  assert.deepEqual(ids('status:ready'), ['b', 'c']);
  assert.deepEqual(ids('-status:ready is:open'), ['a', 'd']);
  assert.deepEqual(ids('status:ready,backlog'), ['a', 'b', 'c']);
  assert.deepEqual(ids('no:status'), ['d']);
  assert.deepEqual(ids('no:label'), ['a', 'd']); // b and c have a plain label
  assert.deepEqual(ids('label:bug'), ['b', 'c']); // case aside, as the server
  assert.deepEqual(ids('role:lead'), ['d']); // an inherited role counts
});

test('sorts by creation and update; a page and its total', () => {
  assert.deepEqual(ids('', 'created desc'), ['d', 'c', 'b', 'a']);
  assert.deepEqual(ids('is:open', 'updated desc'), ['a', 'd', 'c']);
  assert.deepEqual(localIDs(board, '', '', 2, 1), { ids: ['b', 'c'], total: 4 });
});

test('what the server alone answers: null', () => {
  for (const f of ['login fails', '#12', 'assignee:@me', 'sprint:@current', 'title:x*', 'updated:>2026-01-01', 'involves:ann'])
    assert.equal(localIDs(board, f, '', 0), null, f);
  assert.equal(localIDs(board, 'is:open', 'prio desc', 0), null); // a field's order: its options'
});
