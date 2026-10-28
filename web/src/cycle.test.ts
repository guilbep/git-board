import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { Cycle, GatePR, IssueEvent } from './api/board.ts';
import { cardFor, criteriaLeft, criterionOf, DEFAULT_IDS, mergedRun, riskWords, signoffLine, steps, statusBar, suggestion, tiles, turn, withCriteria } from './cycle.ts';
import { on } from './when.ts';

const at = (c: Partial<Cycle>): Cycle => ({
  step: 7, of: 7, name: 'Verify', stage: 'validation', done: false, gate: 'Green codes are not proof', mine: true,
  waits: { role: 'signoff', login: 'alice', what: 'sign off qa', anyone: false }, actions: ['verify', 'check', 'signoff'],
  evidence: {}, stages: { ready: 'ready', in_progress: [], in_review: [], validation: ['qa'], closing: 'validated', guessed: true, criteria: ['Acceptance criteria', 'Done when', 'Expected'], kinds: { bug: 'failing-test', feat: 'per-criterion', doc: 'none', chore: 'none' } },
  // the default list's entry for the step, unless the test names one
  at: DEFAULT_IDS[c.step ?? 7]!,
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
  // a POST with no body: the server moves it once the checks are proven
  assert.deepEqual(press('ready-for-review'), [['Mark ready for review', 'ready-for-review', undefined]]);
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
  assert.equal(withCriteria('It fails.\n\n## Expected\n\nTo be written.', ['a', 'b'], hs), 'It fails.\n\n## Expected\n\n- [ ] a\n- [ ] b\n\nTo be written.');
  assert.equal(withCriteria('It fails.', ['a'], hs), 'It fails.\n\n## Acceptance criteria\n\n- [ ] a');
  assert.equal(withCriteria('**done when**', ['a'], ['Done when']), '**done when**\n\n- [ ] a');
});

test('the criteria, written into the description', () => {
  assert.equal(withCriteria('', ['a', 'b']), '## Acceptance criteria\n\n- [ ] a\n- [ ] b');
  assert.equal(withCriteria('The import fails.\n', ['a']), 'The import fails.\n\n## Acceptance criteria\n\n- [ ] a');
  // under the heading already there: the server reads the first one
  assert.equal(
    withCriteria('Intro\n\n**Acceptance criteria:**\n\nTo be written.\n\n## Notes', ['a']),
    'Intro\n\n**Acceptance criteria:**\n\n- [ ] a\n\nTo be written.\n\n## Notes',
  );
  assert.equal(withCriteria('### Acceptance criteria', ['a']), '### Acceptance criteria\n\n- [ ] a');
  // the list under the heading is replaced, and its ticks kept
  assert.equal(
    withCriteria('## Acceptance criteria\n\n- [x] a\n- b\n1. [ ] gone\n\nNotes', ['a', 'c']),
    '## Acceptance criteria\n\n- [x] a\n- [ ] c\n\nNotes',
  );
  assert.equal(withCriteria('## Acceptance criteria\n\n- [x] a', ['b']), '## Acceptance criteria\n\n- [ ] b');
  assert.equal(withCriteria('', ['- a', '', ' 2) b ', '[ ] c', '* [x] d']), '## Acceptance criteria\n\n- [ ] a\n- [ ] b\n- [ ] c\n- [x] d');
});

test('a line, as a criterion', () => {
  assert.deepEqual(criterionOf(' 2) [X] b '), { text: 'b', done: true });
  assert.deepEqual(criterionOf('- [ ] a'), { text: 'a', done: false });
  assert.deepEqual(criterionOf('plain'), { text: 'plain', done: false });
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

test('verify: the run at the merge, the criteria left, what the sign-off does', () => {
  const runs = [{ by: 'dan', commit: 'aaa', at: '2026-10-18T10:00:00Z' }, { by: 'erin', commit: 'bbb', at: '2026-10-18T11:00:00Z' }];
  assert.equal(mergedRun(at({})), null);
  assert.deepEqual(mergedRun(at({ evidence: { runs } })), { ...runs[1], merge: false });
  assert.deepEqual(mergedRun(at({ evidence: { runs, pr: pr({ number: 7, state: 'MERGED', merge_commit: 'aaa' }) } })), { ...runs[0], merge: true });
  assert.deepEqual(mergedRun(at({ evidence: { runs, pr: pr({ number: 7, state: 'OPEN', merge_commit: 'aaa' }) } })), { ...runs[1], merge: false });

  assert.equal(criteriaLeft(at({ evidence: { criteria: ['a', 'b', 'c'], checks: [{ index: 1, by: 'erin', at: '' }] } })), 2);
  assert.equal(criteriaLeft(at({})), 0);

  const v = (validation: string[], closing = 'validated') => at({ stages: { ...at({}).stages, validation, closing } });
  assert.equal(signoffLine(v(['ready-for-validation', 'qa']), 'ready-for-validation'), 'The sign-off moves the issue to qa.');
  assert.equal(signoffLine(v(['qa']), 'qa', 'GitHub #37'), 'The sign-off closes the issue as validated, and GitHub #37 with it.');
  assert.equal(signoffLine(v([], 'done'), 'in-review'), 'The sign-off closes the issue as done.');
  assert.equal(signoffLine(v([], ''), 'in-review'), 'The sign-off is written on the issue; its status stays.');
});

test('step 6 counts its approvals and open threads', () => {
  const review = (id: string, r: object) => ({ id, kind: 'comment', author: { id, name: id, login: id }, at: '', review: r }) as IssueEvent;
  const events = [review('b', { path: 'a.go', line: 1, side: 'new', commit: 'c' }), review('x', { path: 'a.go', line: 9, side: 'new', commit: 'c', resolved: true })];
  const c = at({ step: 6, name: 'Review', stage: 'in-review', risk: 'normal', note: '', evidence: { pr: pr({ number: 41 }) } });
  assert.equal(steps(c, undefined, events)[5]!.line, '0 of 1 approval · 1 thread open');
  const approved = [...events, review('erin', { state: 'APPROVE', commit: 'c' })];
  assert.equal(steps({ ...c, risk: 'structural' }, undefined, approved)[5]!.line, '1 of 2 approvals · 1 thread open');
  // merged, the server's note says what waits
  assert.equal(steps({ ...c, note: 'merged, waits for the status qa' }, undefined, events)[5]!.line, 'merged, waits for the status qa');
  // done, the issue's approval counts before the sync takes it to the PR
  assert.equal(steps({ ...c, step: 7, at: 'verify', name: 'Verify' }, undefined, approved)[5]!.line, 'erin approved');
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
  assert.deepEqual(statusBar(at({ evidence: { pr: pr({ number: 41, draft: true }) } }), 1)[1], 'draft PR #41');
});

test('the draft PR that holds the plan, on step 3\'s line', () => {
  const plan = { path: 'docs/plans/41-x.md', status: 'draft' } as NonNullable<Cycle['evidence']['plan']>;
  const draft = pr({ number: 41, draft: true });
  const current = steps(at({ step: 3, name: 'Plan', risk: 'normal', note: '', waits: { role: 'plan', login: 'bob', what: 'approve the plan', anyone: false }, evidence: { plan, pr: draft } }));
  assert.equal(current[2]?.line, 'waits for bob to approve the plan · draft PR #41');
  const done = steps(at({ step: 4, name: 'Tests', evidence: { plan, pr: draft } }));
  assert.equal(done[2]?.line, 'docs/plans/41-x.md · draft PR #41');
  // ready for review: no longer a draft
  assert.equal(steps(at({ step: 4, name: 'Tests', evidence: { plan, pr: pr({ number: 41 }) } }))[2]?.line, 'docs/plans/41-x.md');
});

test('the AI\'s newest suggestion, and what a class opens', () => {
  const ev = (id: string, meta: Record<string, string>, body = ''): IssueEvent => ({ kind: 'comment', id, author: { id: 'bot', name: 'bot' }, at: '', body, meta });
  assert.equal(suggestion([ev('a', { 'cycle:step': '2' })]), null);
  assert.deepEqual(suggestion([
    ev('a', { 'cycle:step': '2', 'ai:risk': 'low' }, 'Suggested: **low**\n\nA copy.'),
    ev('b', { 'cycle:step': '2', 'ai:risk': 'structural' }, 'Suggested: **structural**\n\nThe export changes a schema.'),
  ]), { risk: 'structural', why: 'The export changes a schema.' });
  assert.equal(riskWords('low'), 'low skips step 3, Start work opens step 4');
  assert.equal(riskWords('normal'), 'normal opens step 3');
});

// the project's list, as the server sends it: qa and tests-metier are sign-offs
const list = (...ids: string[]) => ({
  ...at({}).stages, validation: ['qa', 'tests-metier'],
  steps: ids.map((id) => (['qa', 'tests-metier'].includes(id) ? { id, kind: 'signoff', name: id, status: id } : { id, kind: id, name: id })),
});

test("the rail is the list's", () => {
  const rows = steps(at({ step: 6, at: 'review', name: 'review', stages: list('code', 'review'), evidence: { branch: 'docs/1-x' } }));
  assert.deepEqual(rows.map((r) => [r.id, r.i, r.n, r.state, r.line]), [['code', 1, 5, 'done', 'docs/1-x'], ['review', 2, 6, 'current', '0 of 1 approval · 0 threads open']]);
});

test('a struck plan stays a cell', () => {
  const rows = steps(at({ step: 5, at: 'code', risk: 'low', skipped: [3], stages: list('intent', 'plan', 'code', 'review') }));
  assert.deepEqual(rows.map((r) => [r.id, r.i, r.state]), [['intent', 1, 'done'], ['plan', 2, 'skipped'], ['code', 3, 'current'], ['review', 4, 'locked']]);
});

test('nothing current before the first step opens', () => {
  assert.deepEqual(steps(at({ step: 2, at: '', stages: list('code', 'review') })).map((r) => r.state), ['locked', 'locked']);
});

test('the sign-off cells', () => {
  const stages = list('intent', 'code', 'review', 'qa', 'tests-metier');
  const signoffs = [{ status: 'qa', by: 'alice', at: '2026-10-18T10:00:00Z' }];
  const rows = steps(at({ at: 'tests-metier', stages, evidence: { signoffs }, waits: { role: 'signoff', login: 'bob', what: 'sign off tests-metier', anyone: false } }));
  assert.deepEqual(rows.slice(3).map((r) => [r.id, r.i, r.n, r.state, r.line]), [
    ['qa', 4, 7, 'done', 'Signed off by alice'],
    ['tests-metier', 5, 7, 'current', 'waits for bob to sign off tests-metier'],
  ]);
  assert.equal(steps(at({ step: 6, at: 'review', stages }))[3]!.line, 'opens at qa');
});
