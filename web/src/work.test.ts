import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { WorkRow } from './api/board.ts';
import { actionFor, ago, retroWords, sendWords, stepLabel, waitsFor } from './work.ts';

const now = new Date('2026-10-19T12:00:00Z');
const before = (s: number) => new Date(now.getTime() - s * 1000).toISOString();

test('ago', () => {
  assert.equal(ago(before(5), now), 'just now');
  assert.equal(ago(before(12 * 60), now), '12 min');
  assert.equal(ago(before(2 * 3600 + 5), now), '2 h');
  assert.equal(ago(before(3 * 86400), now), '3 d');
  assert.equal(ago(before(36 * 86400), now), '5 w');
  assert.equal(ago(before(-60), now), 'just now');
});

const row = (what: string, more: Partial<WorkRow> = {}): Pick<WorkRow, 'waits' | 'mine' | 'risk' | 'question' | 'actions'> => ({
  waits: { login: 'alice', what, anyone: false }, mine: true, ...more,
});

test('waitsFor says what the viewer owes', () => {
  assert.equal(waitsFor(row('set the risk class')), 'A risk class');
  assert.equal(waitsFor(row('approve the plan')), 'Your read of the plan');
  assert.equal(waitsFor(row('approve the plan PR on GitHub')), 'Your approval of the plan PR');
  assert.equal(waitsFor(row('review the PR')), 'Your review');
  assert.equal(waitsFor(row('review the PR', { risk: 'structural' })), 'A second human');
  assert.equal(waitsFor(row('sign off qa')), 'Your sign-off: qa');
  assert.equal(waitsFor(row('add acceptance criteria')), 'Acceptance criteria');
  assert.equal(waitsFor(row('write the failing test')), 'The failing test');
  assert.equal(waitsFor(row('commit and open the PR')), 'The PR');
  assert.equal(waitsFor(row('move the status to in-progress')), 'The status: in-progress');
});

test('waitsFor names who else, and the question', () => {
  assert.equal(waitsFor(row('review the PR', { mine: false, waits: { login: 'bob', what: 'review the PR', anyone: false } })), 'bob to review the PR');
  assert.equal(waitsFor(row('write the failing test', { waits: { what: 'write the failing test', anyone: true } })), 'Anyone to write the failing test');
  const q = { id: 'c1', body: ' Which tolerance? ', by: { id: 'b', name: 'Bob' }, at: before(60) };
  assert.equal(waitsFor(row('review the PR', { question: q })), 'Your answer: "Which tolerance?"');
  const long = { ...q, body: 'x'.repeat(100) };
  assert.equal(waitsFor(row('review the PR', { question: long })), `Your answer: "${'x'.repeat(79)}…"`);
});

test('actionFor', () => {
  assert.deepEqual(actionFor(row('set the risk class', { actions: ['set-risk'] })), { kind: 'risk' });
  assert.deepEqual(actionFor(row('approve the plan', { actions: ['approve-plan'] })), { kind: 'open', label: 'Read the plan' });
  assert.deepEqual(actionFor(row('review the PR', { actions: ['review'] })), { kind: 'open', label: 'Review' });
  assert.deepEqual(actionFor(row('sign off qa', { actions: ['verify', 'check', 'signoff'] })), { kind: 'open', label: 'Sign off qa' });
  assert.deepEqual(actionFor(row('write the failing test')), { kind: 'open', label: 'Open' });
  assert.deepEqual(actionFor(row('review the PR', { mine: false, actions: ['review'] })), { kind: 'open', label: 'Open' });
  const q = { id: 'c1', body: 'Which?', by: { id: 'b', name: 'Bob' }, at: before(60) };
  assert.deepEqual(actionFor(row('set the risk class', { actions: ['set-risk'], question: q })), { kind: 'answer' });
});

test('the small words', () => {
  assert.equal(stepLabel(6, 'Review'), '06 Review');
  assert.equal(sendWords(0), 'Nothing to send');
  assert.equal(sendWords(1), '1 change');
  assert.equal(sendWords(3), '3 changes');
  assert.equal(retroWords({ sprint: 's1', corrections: 5 }), '5 agent corrections flagged in reviews.');
  assert.equal(retroWords({ sprint: 's1', corrections: 1 }), '1 agent correction flagged in reviews.');
  assert.equal(retroWords({ sprint: 's1', corrections: 0 }), 'No correction flagged this sprint.');
  assert.equal(retroWords({ corrections: 0 }), 'No current sprint: [sprints] is the retro’s clock.');
});
