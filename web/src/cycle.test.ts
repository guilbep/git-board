import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { Cycle } from './api/board.ts';
import { cardFor, progressLine } from './cycle.ts';

const at = (c: Partial<Cycle>): Cycle => ({
  step: 7, of: 7, name: 'Verify', stage: 'validation', done: false, gate: 'Green codes are not proof', mine: true,
  waits: { role: 'signoff', login: 'alice', what: 'sign off qa', anyone: false }, actions: ['verify', 'check', 'signoff'],
  evidence: {}, stages: { ready: 'ready', in_progress: [], in_review: [], validation: ['qa'], closing: 'validated', guessed: true },
  ...c,
});

test('the progress line says the step and whose turn it is', () => {
  assert.equal(progressLine(at({})), 'Step 7 of 7 · Verify, waits for alice to sign off qa');
  assert.equal(
    progressLine(at({ step: 3, name: 'Plan', skipped: [3], waits: { role: 'assignee', login: 'erin', what: 'move the status to in-progress', anyone: false } })),
    'Step 3 of 7 · Plan, skipped (low risk), waits for erin to move the status to in-progress',
  );
  assert.equal(progressLine(at({ done: true, name: 'Done' })), 'Done');
  assert.equal(progressLine(at({ step: 2, name: 'Risk class', waits: { what: 'set the risk class', anyone: true } })), 'Step 2 of 7 · Risk class, anyone’s turn to set the risk class');
});

test('the card is for the person whose turn it is', () => {
  assert.equal(cardFor(at({ mine: false }), 'qa'), null);
  assert.equal(cardFor(at({ step: 4, actions: [] }), 'in-progress'), null);
  const risk = cardFor(at({ step: 2, gate: 'Class written on the issue', actions: ['set-risk'] }), 'ready');
  assert.equal(risk?.title, 'Set the risk');
  assert.deepEqual(risk?.buttons.map((b) => [b.label, b.action, b.body]), [
    ['low', 'set-risk', { risk: 'low' }],
    ['normal', 'set-risk', { risk: 'normal' }],
    ['structural', 'set-risk', { risk: 'structural' }],
  ]);
  const signoff = cardFor(at({}), 'qa');
  assert.equal(signoff?.title, 'Sign off qa');
  assert.equal(signoff?.gate, 'Green codes are not proof');
  assert.deepEqual(signoff?.buttons.map((b) => b.action), ['signoff']);
  assert.equal(cardFor(at({ step: 3, actions: ['approve-plan'] }), 'ready')?.title, 'Approve the plan');
});
