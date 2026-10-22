import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { Cycle, GatePR, IssueEvent } from './api/board.ts';
import { cardFor, forStep, steps, statusBar, tiles, turn, withCriteria } from './cycle.ts';
import { on } from './when.ts';

const at = (c: Partial<Cycle>): Cycle => ({
  step: 7, of: 7, name: 'Verify', stage: 'validation', done: false, gate: 'Green codes are not proof', mine: true,
  waits: { role: 'signoff', login: 'alice', what: 'sign off qa', anyone: false }, actions: ['verify', 'check', 'signoff'],
  evidence: {}, stages: { ready: 'ready', in_progress: [], in_review: [], validation: ['qa'], closing: 'validated', guessed: true, criteria: ['Acceptance criteria', 'Done when', 'Expected'], kinds: { bug: 'failing-test', feat: 'per-criterion', doc: 'none', chore: 'none' } },
  ...c,
});

test('whose turn it is', () => {
  assert.equal(turn(at({})), 'waits for alice to sign off qa');
  assert.equal(turn(at({ waits: { what: 'set the risk class', anyone: true } })), 'anyone’s turn to set the risk class');
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

test('the step buttons move the status to their stage', () => {
  const stages = { ready: 'ready', in_progress: ['doing'], in_review: ['review'], validation: ['qa'], closing: 'done', guessed: false, criteria: [], kinds: {} };
  const press = (action: string) => cardFor(at({ stages, actions: [action] }), '')?.buttons.map((b) => [b.label, b.action, b.body]);
  assert.deepEqual(press('move-ready'), [['Move to ready', 'move-ready', { status: 'ready' }]]);
  assert.deepEqual(press('start'), [['Start work', 'start', { status: 'doing' }]]);
  assert.deepEqual(press('ready-for-review'), [['Mark ready for review', 'ready-for-review', { status: 'review' }]]);
  assert.deepEqual(press('to-validation'), [['Start verification', 'to-validation', { status: 'qa' }]]);
});

const pr = (p: Partial<GatePR>): GatePR =>
  ({ number: 1, url: 'https://github.com/o/r/pull/1', state: 'OPEN', draft: false, title: '', base: 'dev', head: '', author: 'dan', decision: '', mergeable: '', merge_state: '', checks: [], reviews: [], requested: [], can_merge: false, checking: false, waits: [], ...p });

test('the step bar: done, current, locked with its reason', () => {
  const rows = steps(
    at({
      step: 3, name: 'Plan', risk: 'structural', gate: 'Structural: plan PR approved first', note: 'plan not approved',
      evidence: { criteria: ['a', 'b', 'c', 'd'], risk: 'structural', plan_pr: pr({ number: 3089 }) },
    }),
    'erin',
  );
  assert.deepEqual(rows.map((r) => [r.n, r.name, r.state, r.line]), [
    [1, 'Intent', 'done', '4 criteria'],
    [2, 'Risk', 'done', 'Structural, set by erin'],
    [3, 'Plan', 'current', 'plan not approved'],
    [4, 'Tests', 'locked', 'opens when work starts'],
    [5, 'Code', 'locked', 'opens with the test'],
    [6, 'Review', 'locked', 'opens when the PR is ready for review'],
    [7, 'Verify', 'locked', 'opens when the PR is merged'],
  ]);
  // each step's own gate, for the main section when it is shown
  assert.deepEqual(rows.map((r) => r.gate), [
    'No issue, no agent', 'Class written on the issue', 'Structural: plan PR approved first', 'The touched suites green, each criterion names its test',
    'A human commits and opens the PR', 'Structural: a second human', 'Green codes are not proof',
  ]);
});

test('the step bar at step 1, and with steps skipped', () => {
  const first = steps(at({ step: 1, name: 'Intent', gate: 'No issue, no agent', evidence: {}, waits: { role: 'author', login: 'dan', what: 'add acceptance criteria', anyone: false } }));
  assert.deepEqual(first.map((r) => [r.state, r.line]), [
    ['current', 'waits for dan to add acceptance criteria'],
    ['locked', 'opens at ready'],
    ['locked', 'opens once the risk is set'],
    ['locked', 'opens when work starts'],
    ['locked', 'opens with the test'],
    ['locked', 'opens when the PR is ready for review'],
    ['locked', 'opens when the PR is merged'],
  ]);
  // low risk waits at step 3 for Start work: current, its gate says skipped
  const low = steps(at({ step: 3, risk: 'low', skipped: [3], gate: 'Skipped, low risk', note: 'low risk, waits for the status in-progress', evidence: { criteria: ['a'] } }));
  assert.deepEqual([low[2]?.state, low[2]?.gate], ['current', 'Skipped, low risk']);
  const past = steps(at({ step: 5, risk: 'low', skipped: [3, 4], evidence: { kind: 'doc' } }));
  assert.deepEqual(past.slice(2, 5).map((r) => [r.state, r.line, r.gate]), [
    ['skipped', 'skipped, low risk', 'Skipped, low risk'],
    ['skipped', 'skipped, doc', 'Skipped: no tests for this kind'],
    ['current', 'waits for alice to sign off qa', 'Green codes are not proof'],
  ]);
});

test('the criteria, written under the description\'s own heading', () => {
  const hs = ['Acceptance criteria', 'Done when', 'Expected'];
  // under Expected, the heading the description has, not a new Acceptance criteria
  assert.equal(withCriteria('It fails.\n\n## Expected\n\nTo be written.', ['a', 'b'], hs), 'It fails.\n\n## Expected\n\n- a\n- b\n\nTo be written.');
  assert.equal(withCriteria('It fails.', ['a'], hs), 'It fails.\n\n## Acceptance criteria\n\n- a');
  assert.equal(withCriteria('**done when**', ['a'], ['Done when']), '**done when**\n\n- a');
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

test('the step bar at verify', () => {
  const rows = steps(
    at({
      evidence: {
        criteria: ['a'], risk: 'normal', plan_approval: { by: 'erin', at: '2026-10-01T00:00:00Z' },
        tests: [{ path: 'api/a_test.go', commit: '2c8e4a1f00' }], branch: 'fix/1-x', head: '4f2e9c1a00',
        pr: pr({ number: 7, author: 'dan', reviews: [{ login: 'erin', state: 'APPROVED', commit: '' }, { login: 'alice', state: 'APPROVED', commit: '' }, { login: 'bob', state: 'COMMENTED', commit: '' }] }),
        signoffs: [{ status: 'ready-for-validation', by: 'alice', at: '2026-10-18T10:00:00Z' }],
      },
      stages: { ready: 'ready', in_progress: [], in_review: [], validation: ['ready-for-validation', 'qa', 'tests-metier'], closing: 'validated', guessed: false, criteria: ['Acceptance criteria'], kinds: {} },
    }),
  );
  assert.deepEqual(rows.map((r) => r.line), ['1 criterion', 'Normal', 'Approved by erin', 'api/a_test.go at 2c8e4a1', 'Committed by dan', 'erin and alice approved', 'waits for alice to sign off qa']);
  assert.deepEqual(rows.map((r) => r.state), ['done', 'done', 'done', 'done', 'done', 'done', 'current']);
  assert.equal(steps(at({ evidence: { branch: 'fix/1-x', head: '4f2e9c1a00', pr: pr({ number: 7, state: 'MERGED' }) } }))[5]?.line, 'PR #7 merged');
  assert.equal(steps(at({ evidence: { branch: 'fix/1-x', head: '4f2e9c1a00' } }))[4]?.line, 'fix/1-x at 4f2e9c1');
  assert.deepEqual(steps(at({ done: true, name: 'Done', evidence: {} })).map((r) => r.state), Array(7).fill('done'));
});

test('each step has its own discussion', () => {
  const ev = (id: string, kind: IssueEvent['kind'], step?: string) => ({ id, kind, meta: step ? { 'cycle:step': step } : undefined }) as unknown as IssueEvent;
  const events = [ev('c', 'created'), ev('1', 'comment'), ev('2', 'comment', '3'), ev('3', 'labels'), ev('4', 'comment', '1')];
  assert.deepEqual(forStep(events, 1).map((e) => e.id), ['1', '4']);
  assert.deepEqual(forStep(events, 3).map((e) => e.id), ['2']);
  assert.deepEqual(forStep(events, 6), []);
});

test('the sign-off tiles', () => {
  const c = at({
    evidence: { signoffs: [{ status: 'ready-for-validation', by: 'alice', at: '2026-10-18T10:00:00Z' }] },
    stages: { ready: 'ready', in_progress: [], in_review: [], validation: ['ready-for-validation', 'qa', 'tests-metier'], closing: 'validated', guessed: false, criteria: ['Acceptance criteria'], kinds: {} },
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
