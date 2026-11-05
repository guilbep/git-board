import { test } from 'node:test';
import assert from 'node:assert/strict';
import { branchItems, branchLine, branchState, faceOf, folderName } from './branchstate.ts';
import type { GatePR, IssueBranch } from './api/board';

const br = (name: string, o: Partial<IssueBranch> = {}): IssueBranch => ({
  name, ahead: 2, behind: 0, merged: false, in_base: false, last_commit: { sha: 'a1b2c3d', subject: 'x', author: 'Person A', date: '2026-10-29T10:00:00Z' }, ...o,
});
const pr = (o: Partial<GatePR> = {}): GatePR => ({
  number: 3155, url: '', state: 'OPEN', draft: false, title: '', base: 'dev', head: 'h', author: 'person-a', decision: '', mergeable: 'MERGEABLE', merge_state: 'CLEAN',
  checks: [], reviews: [], requested: [], can_merge: false, checking: false, waits: [], ...o,
});
const st = (p: GatePR | null, b = br('test/2771/1-implement-tests'), remote = true) => branchState(b, remote, p, 'dev');

test('a chip names its branch in the issue folder, else its gate, else its last part', () => {
  assert.equal(faceOf('test/2771/2-reference-examples'), '2771/2');
  assert.equal(faceOf('2811/1-pdf-footer'), '2811/1');
  assert.equal(faceOf('test/2771-implement-tests', pr()), '#3155');
  assert.equal(faceOf('test/2771-implement-tests'), '2771-implement-tests');
});

test('one tone per state, whose move it is', () => {
  assert.equal(st(pr({ checks: [{ name: 'ci', state: 'failure', required: true }] })).tone, 'author');
  assert.equal(st(pr({ decision: 'CHANGES_REQUESTED' })).word, 'changes asked');
  assert.equal(st(pr({ mergeable: 'CONFLICTING' })).word, 'conflict');
  assert.equal(st(pr({ merge_state: 'BEHIND' })).word, 'behind dev');
  assert.equal(st(pr({ checking: true })).tone, 'checks');
  assert.deepEqual([st(pr({ decision: 'REVIEW_REQUIRED', requested: ['person-b'] })).tone, st(pr({ decision: 'REVIEW_REQUIRED', requested: ['person-b'] })).who], ['review', 'person-b']);
  assert.equal(st(pr({ can_merge: true })).tone, 'ready');
  assert.equal(st(pr({ draft: true })).tone, 'draft');
  assert.equal(st(pr({ state: 'MERGED' })).tone, 'merged');
  assert.equal(st(pr({ state: 'CLOSED' })).tone, 'closed');
});

test('without a gate: not in review, local, or already in the base', () => {
  assert.equal(st(null).word, 'not in review');
  assert.equal(st(null, br('test/2771/1-x'), false).word, 'local');
  assert.equal(st(null, br('test/2771/1-x', { merged: true, behind: 4 })).tone, 'merged');
  assert.equal(st(null, br('test/2771/1-x', { in_base: true })).tone, 'merged');
  // a new branch still on the base's commit is not merged work
  assert.equal(st(null, br('test/2771/1-x', { merged: true, in_base: true, behind: 0, ahead: 0 })).tone, 'branch');
});

test('a failing required check is the author’s move before a review', () => {
  const s = st(pr({ decision: 'REVIEW_REQUIRED', requested: ['person-b'], checks: [{ name: 'ci', state: 'failure', required: true }], waits: ['ci', 'a review'] }));
  assert.equal(s.tone, 'author');
  assert.equal(s.who, 'person-a');
  assert.match(s.sentence, /waits for ci, a review$/);
  assert.match(s.sentence, /the gate: GitHub #3155 into dev/);
});

test('the remote copy and the local one are one chip', () => {
  const items = branchItems([br('test/2771/1-x'), br('test/2771/1-x', { remote: 'origin' }), br('test/2771/2-y')], [{ branch: 'test/2771/1-x', pr: pr({ can_merge: true }) }], 'dev');
  assert.deepEqual(items.map((i) => [i.state.face, i.state.tone]), [['2771/1', 'ready'], ['2771/2', 'branch']]);
  assert.equal(items[0]!.branch.remote, 'origin');
});

test('ten branches: four chips by whose move, two more, three merged, one closed', () => {
  const tones = ['ready', 'branch', 'author', 'merged', 'review', 'merged', 'checks', 'draft', 'closed', 'merged'] as const;
  const items = tones.map((tone, i) => ({ i, state: { tone, icon: 'branch' as const, word: '', who: '', face: '', sentence: '' } }));
  const line = branchLine(items);
  assert.deepEqual(line.chips.map((c) => c.state.tone), ['author', 'review', 'checks', 'ready']);
  assert.deepEqual(line.more.map((c) => c.state.tone), ['draft', 'branch']);
  assert.equal(line.merged.length, 3);
  assert.equal(line.closed.length, 1);
});

test('the proposed name goes in the issue folder, n one more than the highest', () => {
  assert.equal(folderName('test/2771-implement-tests', []), 'test/2771/1-implement-tests');
  assert.equal(folderName('test/2771-reference-examples', ['test/2771/1-implement-tests', 'test/2771/3-x', 'test/2771-old']), 'test/2771/4-reference-examples');
  assert.equal(folderName('2811-pdf-footer', ['2811/1-pdf']), '2811/2-pdf-footer');
  assert.equal(folderName('test/2771/2-already', []), 'test/2771/2-already');
  assert.equal(folderName('release-notes', []), 'release-notes');
});
