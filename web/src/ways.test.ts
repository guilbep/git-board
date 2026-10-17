import { test } from 'node:test';
import assert from 'node:assert/strict';
import { aheadWords, checkWords, pendingWords, pushWhat, runWords } from './ways.ts';

const dir = (o: Partial<Record<'issues' | 'changed' | 'comments' | 'edits' | 'changes', number>> = {}) => ({ issues: 0, changed: 0, comments: 0, edits: 0, changes: 0, ...o });
const tally = (to = dir(), from = dir()) => ({ to, from, linked: 0, stays_local: 0, warnings: 0, errors: 0, lines: [] });
const remote = (send: number, receive: number, people = { send: 0, receive: 0 }, tracked = true) => ({
  name: 'origin', url: '/srv/r.git', tracked, issues: { send, receive }, people,
});

test('aheadWords says what a remote would move', () => {
  assert.equal(aheadWords({ send: 3, receive: 0 }, 'issue'), '3 issues to send · 0 to receive');
  assert.equal(aheadWords({ send: 1, receive: 2 }, 'person', 'people'), '1 person to send · 2 to receive');
});

test('pushWhat names what a push sends, or the lot before any check', () => {
  assert.equal(pushWhat(remote(3, 0, { send: 1, receive: 0 })), '3 issues and 1 person');
  assert.equal(pushWhat(remote(1, 0)), '1 issue');
  assert.equal(pushWhat(remote(0, 0, undefined, false)), 'the issues and people');
});

test('checkWords sums a check over the forges', () => {
  const bridges = { gh: tally(dir({ issues: 1 }), dir({ changed: 2 })), gl: tally(dir({ changed: 1 }), dir({ issues: 1 })) };
  assert.equal(checkWords(bridges), '3 issues to get, 2 to send');
});

test('runWords says one forge’s part of a run, in its mode', () => {
  const t = tally(dir({ issues: 2, comments: 3, edits: 1 }), dir({ issues: 1, changed: 1 }));
  assert.equal(runWords('sync', t), 'create 2 issues, post 3 comments, push 1 edit or change, pull 2 issues');
  assert.equal(runWords('send', t), 'create 2 issues, post 3 comments, push 1 edit or change');
  assert.equal(runWords('get', t), 'pull 2 issues');
});

test('pendingWords leaves out what is zero', () => {
  const r = { checked_at: '', conflicts: [], label_changes: 2, project_writes: 0, warnings: 0, unmatched: 0 };
  assert.equal(pendingWords(r), 'update the labels of 2 issues');
  assert.equal(pendingWords({ ...r, label_changes: 0, project_writes: 1 }), 'write 1 Project field');
});
