// Run with: node --test web/src/replay.test.ts
import assert from 'node:assert/strict';
import { test } from 'node:test';
import type { IssueEvent } from './api/board';
import { asOf, stateAfter, steps, wordDiff } from './replay.ts';
import { bodyThreads } from './flow.ts';
import { stateOf, versions } from './sections.ts';

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

// M4: the core's operations, and the issue as of a step
const T = '2026-10-05T10:00:00Z';
const applied: IssueEvent[] = [
  {
    kind: 'created', id: 'op0:0', author: who('ann'), at: '2026-10-01T09:00:00Z', body: '## Expected\n\n- [x] b', edited: true,
    history: [
      { id: 'op0:0', author: who('ann'), at: '2026-10-01T09:00:00Z', body: '## Expected\n\n- [ ] a' },
      { id: 'op3:0', author: who('ann'), at: T, body: '## Expected\n\n- [ ] b' },
      { id: 'op4:0', author: who('ann'), at: '2026-10-06T10:00:00Z', body: '## Expected\n\n- [x] b' },
    ],
  },
  {
    kind: 'comment', id: 'op1:0', author: who('bob'), at: '2026-10-02T09:00:00Z', body: 'b, not a',
    meta: { 'thread:anchor': 'body@v1/expected', 'thread:resolved': 'v2', 'thread:applied': 'op3:0', 'cycle:step': '1' },
    meta_history: [{ id: 'op3:1', author: who('ann'), at: T, meta: { 'thread:resolved': 'v2', 'thread:applied': 'op3:0' } }],
  },
  { kind: 'labels', id: 'op2:0', author: who('ann'), at: '2026-10-03T09:00:00Z', added: ['status:ready'] },
  {
    kind: 'comment', id: 'op5:0', author: who('cid'), at: '2026-10-07T09:00:00Z', body: 'on §1',
    meta: { 'thread:anchor': 'body@v2/expected', 'thread:resolved': '' },
    meta_history: [
      { id: 'op6:0', author: who('cid'), at: '2026-10-07T10:00:00Z', meta: { 'thread:resolved': 'cid' } },
      { id: 'op7:0', author: who('cid'), at: '2026-10-07T11:00:00Z', meta: { 'thread:resolved': '' } },
    ],
  },
];

test('steps name the core\'s operations, each with its id', () => {
  const s = steps([...events.slice(0, 3), ...applied.slice(1, 2)]);
  assert.deepEqual(s.map((x) => `${x.op}:${x.id}`), [
    'create:c0', 'change-labels:l1', 'set-title:t1', 'comment:op1:0', 'edit-text:', 'set-meta:op3:1',
  ]);
  assert.equal(steps(events).find((x) => x.kind === 'status')!.op, 'set-status');
});

test('a tick publishes no version', () => {
  assert.deepEqual(steps(applied).filter((x) => x.event.id === 'op0:0').map((x) => x.version), [1, 2, 0]);
  assert.equal(steps(events).find((x) => x.kind === 'comment')!.version, undefined);
});

test('an apply\'s edit sorts before its meta', () => {
  assert.deepEqual(steps(applied).filter((x) => x.at === T).map((x) => x.op), ['edit-text', 'set-meta']);
});

test('asOf before v2 is v1', () => {
  const all = steps(applied);
  const at = all.findIndex((x) => x.op === 'change-labels');
  const evs = asOf(all, at);
  const created = evs.find((e) => e.kind === 'created')!;
  assert.equal(versions(created).length, 1);
  assert.equal(created.body, '## Expected\n\n- [ ] a');
  const [t] = bodyThreads(evs);
  assert.equal(t!.resolved, false);
  assert.equal(stateOf(t!.event, versions(created)).state, 'current');
  assert.equal(t!.event.meta?.['cycle:step'], '1');
  // the edit alone: v2, the thread not yet resolved
  const edit = asOf(all, all.findIndex((x) => x.op === 'edit-text'));
  assert.equal(versions(edit.find((e) => e.kind === 'created')!).length, 2);
  assert.equal(bodyThreads(edit)[0]!.resolved, false);
  assert.equal(bodyThreads(asOf(all, all.findIndex((x) => x.op === 'set-meta')))[0]!.resolved, true);
});

test('asOf follows a reopen', () => {
  const all = steps(applied);
  const of = (id: string) => bodyThreads(asOf(all, all.findIndex((x) => x.id === id))).find((t) => t.event.id === 'op5:0')!.resolved;
  assert.deepEqual([of('op5:0'), of('op6:0'), of('op7:0')], [false, true, false]);
});

test('asOf drops later comments', () => {
  const all = steps(applied);
  assert.deepEqual(asOf(all, 0).map((e) => e.id), ['op0:0']);
  assert.equal(asOf(all, all.length - 1).length, applied.length);
});
