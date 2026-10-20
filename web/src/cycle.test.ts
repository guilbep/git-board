import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { Cycle, GatePR } from './api/board.ts';
import { cardFor, progressLine, spine, statusBar, tiles, withCriteria } from './cycle.ts';
import { on } from './when.ts';

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

const pr = (p: Partial<GatePR>): GatePR =>
  ({ number: 1, url: 'https://github.com/o/r/pull/1', state: 'OPEN', draft: false, title: '', base: 'dev', head: '', author: 'dan', decision: '', mergeable: '', merge_state: '', checks: [], reviews: [], requested: [], can_merge: false, checking: false, waits: [], ...p });

test('the spine, one line of evidence per step', () => {
  const rows = spine(
    at({
      step: 3, name: 'Plan', risk: 'structural', gate: 'Structural: plan PR approved first', note: 'plan not approved',
      evidence: { criteria: ['a', 'b', 'c', 'd'], risk: 'structural', plan_pr: pr({ number: 3089 }) },
    }),
    'erin',
  );
  assert.deepEqual(rows.map((r) => [r.n, r.name, r.state, r.line]), [
    [1, 'Intent', 'done', '4 acceptance criteria'],
    [2, 'Risk class', 'done', 'Structural, set by erin'],
    [3, 'Plan', 'current', 'Plan PR #3089, waits for approval · plan not approved'],
    [4, 'Tests first', 'todo', 'Every bug fix ships its test'],
    [5, 'Code', 'todo', 'A human commits and opens the PR'],
    [6, 'Review', 'todo', 'Structural: a second human'],
    [7, 'Verify', 'todo', 'Green codes are not proof'],
  ]);
  // each row's own gate, for its workspace when it is opened
  assert.deepEqual(rows.map((r) => r.gate), [
    'No issue, no agent', 'Class written on the issue', 'Structural: plan PR approved first', 'Every bug fix ships its test',
    'A human commits and opens the PR', 'Structural: a second human', 'Green codes are not proof',
  ]);
});

test('the spine with no evidence', () => {
  const rows = spine(at({ step: 1, name: 'Intent', gate: 'No issue, no agent', evidence: {} }));
  assert.deepEqual(rows.map((r) => [r.state, r.line]), [
    ['current', 'no acceptance criteria'],
    ['todo', 'Class written on the issue'],
    ['todo', 'Plan approved by the plan role'],
    ['todo', 'Every bug fix ships its test'],
    ['todo', 'A human commits and opens the PR'],
    ['todo', 'A human who can explain the diff'],
    ['todo', 'Green codes are not proof'],
  ]);
  assert.equal(rows[0]?.gate, 'No issue, no agent');
  const low = spine(at({ step: 4, risk: 'low', skipped: [3], evidence: { plan_approval: { by: 'erin', at: '2026-10-01T00:00:00Z' } } }))[2];
  assert.deepEqual([low?.line, low?.gate], ['Skipped, low risk', 'Skipped, low risk']);
});

test('the criteria, written into the description', () => {
  assert.equal(withCriteria('', ['a', 'b']), '## Acceptance criteria\n\n- a\n- b');
  assert.equal(withCriteria('The import fails.\n', ['a']), 'The import fails.\n\n## Acceptance criteria\n\n- a');
  // under the heading already there: the server reads the first one
  assert.equal(
    withCriteria('Intro\n\n**Acceptance criteria:**\n\nTo be written.\n\n## Notes', ['a']),
    'Intro\n\n**Acceptance criteria:**\n\n- a\n\nTo be written.\n\n## Notes',
  );
  assert.equal(withCriteria('### Acceptance criteria', ['a']), '### Acceptance criteria\n\n- a');
  assert.equal(withCriteria('', ['- a', '', ' 2) b ', '[ ] c', '* [x] d']), '## Acceptance criteria\n\n- a\n- b\n- c\n- d');
});

test('the spine at verify', () => {
  const rows = spine(
    at({
      evidence: {
        criteria: ['a'], risk: 'normal', plan_approval: { by: 'erin', at: '2026-10-01T00:00:00Z' },
        tests: [{ path: 'api/a_test.go', commit: '2c8e4a1f00' }], branch: 'fix/1-x', head: '4f2e9c1a00',
        pr: pr({ number: 7, author: 'dan', reviews: [{ login: 'erin', state: 'APPROVED', commit: '' }, { login: 'alice', state: 'APPROVED', commit: '' }, { login: 'bob', state: 'COMMENTED', commit: '' }] }),
        signoffs: [{ status: 'ready-for-validation', by: 'alice', at: '2026-10-18T10:00:00Z' }],
      },
      stages: { ready: 'ready', in_progress: [], in_review: [], validation: ['ready-for-validation', 'qa', 'tests-metier'], closing: 'validated', guessed: false },
    }),
  );
  assert.deepEqual(rows.map((r) => r.line), ['1 acceptance criterion', 'Normal', 'Approved by erin', 'api/a_test.go at 2c8e4a1', 'Committed by dan', 'erin and alice approved', 'Sign-off 2 of 3']);
  assert.deepEqual(rows.map((r) => r.state), ['done', 'done', 'done', 'done', 'done', 'done', 'current']);
  assert.equal(spine(at({ evidence: { branch: 'fix/1-x', head: '4f2e9c1a00', pr: pr({ number: 7, state: 'MERGED' }) } }))[5]?.line, 'PR #7 merged');
  assert.equal(spine(at({ evidence: { branch: 'fix/1-x', head: '4f2e9c1a00' } }))[4]?.line, 'fix/1-x at 4f2e9c1');
  assert.deepEqual(spine(at({ done: true, name: 'Done', evidence: {} })).map((r) => r.state), Array(7).fill('done'));
});

test('the sign-off tiles', () => {
  const c = at({
    evidence: { signoffs: [{ status: 'ready-for-validation', by: 'alice', at: '2026-10-18T10:00:00Z' }] },
    stages: { ready: 'ready', in_progress: [], in_review: [], validation: ['ready-for-validation', 'qa', 'tests-metier'], closing: 'validated', guessed: false },
  });
  assert.deepEqual(tiles(c, 'qa'), [
    { status: 'ready-for-validation', state: 'done', by: 'alice', when: `Signed off ${on('2026-10-18T10:00:00Z')}` },
    { status: 'qa', state: 'current', by: undefined, when: 'Now' },
    { status: 'tests-metier', state: 'next', by: undefined, when: 'Next' },
  ]);
});

test('the status bar', () => {
  assert.deepEqual(statusBar(at({ evidence: {} }), null), ['no branch yet', 'no PR']);
  const checks = [{ name: 'Tests', state: 'success', required: true }, { name: 'Lint', state: 'pending', required: false }] as GatePR['checks'];
  assert.deepEqual(statusBar(at({ evidence: { branch: 'fix/1-x', head: '4f2e9c1a00', pr: pr({ number: 7, checks }) } }), 2), ['fix/1-x at 4f2e9c1', 'PR #7 · 1 of 2 checks green', 'waits for 2 changes on the forges']);
  assert.deepEqual(statusBar(at({ evidence: { branch: 'fix/1-x', pr: pr({ number: 7, state: 'MERGED' }) } }), 0), ['fix/1-x', 'PR #7 merged', 'in sync']);
  assert.deepEqual(statusBar(at({ evidence: { pr: pr({ number: 7 }) } }), 1)[1], 'PR #7 open');
});
