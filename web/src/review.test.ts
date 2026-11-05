// Run with: node --test src/review.test.ts
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { editable, threadsOf, verbOf } from './review.ts';
import type { IssueEvent, Review } from './api/types.ts';

const person = (login: string) => ({ id: login, name: login, login });

const ev = (id: string, who: string, review?: Review, body = ''): IssueEvent => ({
  kind: 'comment',
  id,
  author: person(who),
  at: '2026-10-18T10:00:00Z',
  body,
  ...(review && { review }),
});

test('threadsOf puts replies under their first comment, resolved from it', () => {
  const anchor = { path: 'a.txt', line: 3, side: 'new' as const, commit: 'c1' };
  const events = [
    ev('r1', 'ann', { ...anchor, resolved: true }),
    ev('plain', 'bob'),
    ev('s1', 'bob', { state: 'APPROVE', commit: 'c1' }),
    ev('r2', 'bob', { ...anchor, reply_to: 'r1' }),
    ev('lost', 'bob', { ...anchor, reply_to: 'gone' }),
  ];
  const th = threadsOf(events);
  assert.equal(th.length, 2);
  assert.equal(th[0]!.root.id, 'r1');
  assert.deepEqual(th[0]!.replies.map((e) => e.id), ['r2']);
  assert.equal(th[0]!.resolved, true);
  // a reply whose first comment isn't here stands alone
  assert.equal(th[1]!.root.id, 'lost');
});

test('editable: a review comment goes to the pull request, its edits never do', () => {
  assert.equal(editable(ev('plain', 'ann')), true);
  assert.equal(editable(ev('line', 'ann', { path: 'a', line: 1, side: 'new', commit: 'c1' })), false);
  assert.equal(editable(ev('pr', 'ann', { pr: 'github.com/o/r#26' })), false);
});

test('verbOf names a pull request’s own comment', () => {
  assert.equal(verbOf(ev('plain', 'ann')), 'commented');
  assert.equal(verbOf(ev('pr', 'ann', { pr: 'github.com/o/r#26' })), 'commented on pull request #26');
  assert.equal(verbOf(ev('ok', 'ann', { state: 'APPROVE', commit: 'c1', pr: 'github.com/o/r#26' })), 'approved');
  assert.equal(verbOf({ ...ev('d', 'ann'), kind: 'created' }), 'opened');
});
