// Run with: node --test web/src/replay.test.ts
import assert from 'node:assert/strict';
import { test } from 'node:test';
import type { IssueEvent } from './api/board';
import { stateAfter, steps, wordDiff } from './replay.ts';

const who = (name: string) => ({ id: name, name });
const events: IssueEvent[] = [
  {
    kind: 'created', id: 'c0', author: who('ann'), at: '2026-10-01T09:00:00Z', body: 'Export is wrong', edited: true,
    history: [
      { author: who('ann'), at: '2026-10-01T09:00:00Z', body: 'Export is wrong' },
      { author: who('bob'), at: '2026-10-03T09:00:00Z', body: 'The CSV export is wrong' },
    ],
  },
  { kind: 'labels', id: 'l1', author: who('ann'), at: '2026-10-01T09:01:00Z', added: ['status:backlog', 'bug'] },
  { kind: 'title', id: 't1', author: who('bob'), at: '2026-10-02T09:00:00Z', was: 'Export broken', title: 'CSV export broken' },
  { kind: 'labels', id: 'l2', author: who('bob'), at: '2026-10-02T10:00:00Z', added: ['status:ready'], removed: ['status:backlog'] },
  {
    kind: 'comment', id: 'c1', author: who('cid'), at: '2026-10-02T11:00:00Z', body: 'fixed in #12', edited: true, edit_pending: ['GitLab'],
    history: [
      { author: who('cid'), at: '2026-10-02T11:00:00Z', body: 'fixed' },
      { author: who('cid'), at: '2026-10-04T08:00:00Z', body: 'fixed in #12' },
    ],
  },
  { kind: 'status', id: 's1', author: who('cid'), at: '2026-10-04T09:00:00Z', status: 'closed', pending: ['GitHub'] },
];

test('steps: every change in time order, edits included', () => {
  const s = steps(events);
  assert.deepEqual(s.map((x) => `${x.kind}:${x.author.name}`), [
    'created:ann', 'labels:ann', 'title:bob', 'labels:bob', 'comment:cid', 'edit:bob', 'edit:cid', 'status:cid',
  ]);
  const descEdit = s[5]!;
  assert.equal(descEdit.before, 'Export is wrong');
  assert.equal(descEdit.after, 'The CSV export is wrong');
  assert.equal(descEdit.event.id, 'c0');
  // the comment's latest edit is the one GitLab lacks; the close, GitHub
  assert.deepEqual(s[6]!.pending, ['GitLab']);
  assert.deepEqual(s[4]!.pending, []);
  assert.deepEqual(s[7]!.pending, ['GitHub']);
});

test('stateAfter: the issue as it was', () => {
  const s = steps(events);
  const first = stateAfter(s, 0, 'CSV export broken');
  assert.equal(first.title, 'Export broken'); // before the rename
  assert.equal(first.status, 'open');
  assert.deepEqual(first.labels, []);
  assert.deepEqual(first.comments.map((c) => c.body), ['Export is wrong']);

  const mid = stateAfter(s, 4, 'CSV export broken');
  assert.equal(mid.title, 'CSV export broken');
  assert.deepEqual(mid.labels, ['bug', 'status:ready']);
  assert.deepEqual(mid.comments.map((c) => c.body), ['Export is wrong', 'fixed']);

  const last = stateAfter(s, s.length - 1, 'CSV export broken');
  assert.equal(last.status, 'closed');
  assert.deepEqual(last.comments.map((c) => c.body), ['The CSV export is wrong', 'fixed in #12']);
});

test('wordDiff marks words in and out', () => {
  const ops = wordDiff('Export broken', 'CSV export broken').map((t) => `${t.op}${t.text}`).join('|');
  assert.equal(ops, '-Export |+CSV export | broken'); // ' ' marks the unchanged word
});
