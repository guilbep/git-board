import test from 'node:test';
import assert from 'node:assert/strict';
import type { CheckStatus, StepTests, SuiteRow } from './api/board.ts';
import { bugRuns, pill, preselect, reason, whereLabel } from './steptests.ts';

const st = (verdict?: 'PASSED' | 'FAILED'): CheckStatus => ({ name: '', key: '', proven: verdict === 'PASSED', ...(verdict && { verdict }) });
const row = (r: Partial<SuiteRow>): SuiteRow => ({ name: 'unit', run: ['go', 'test'], touched: [], ...r });
const tests = (t: Partial<StepTests>): StepTests => ({ rule: 'failing-test', base: 'origin/main', head: 'bbbbbbbbb', test_commit: 'aaaaaaaaa', suites: [], checks: [], gate: { proven: false, checks: [] }, running: [], ...t });

test('the touched suites and the unproven checks come ticked, never a CI one', () => {
  const t = tests({
    suites: [row({ name: 'unit', touched: ['internal/release/remote_test.go'] }), row({ name: 'web' }), row({ name: 'e2e', where: 'ci', touched: ['web/x'] })],
    checks: [row({ name: 'vet' }), row({ name: 'lint', head: st('PASSED') })],
  });
  assert.deepEqual(preselect(t), ['unit', 'vet']);
  assert.equal(reason(t.suites[0]!), 'the diff touches internal/release');
  assert.equal(reason(t.suites[1]!), 'not touched');
  assert.equal(whereLabel(t.suites[2]!), 'CI, never run here');
});

test('a pill reads the verdict', () => {
  assert.deepEqual(pill(st('FAILED')), { text: 'FAIL', color: 'red' });
  assert.deepEqual(pill(st('PASSED')), { text: 'PASS', color: 'green' });
  assert.deepEqual(pill(), { text: 'not run', color: '' });
});

test('the bug rule: red at the test commit, then green at the head', () => {
  const unit = (at_test?: CheckStatus, head?: CheckStatus) => tests({ suites: [row({ touched: ['a_test.go'], ...(at_test && { at_test }), ...(head && { head }) }), row({ name: 'web', at_test: st('PASSED') })] });
  assert.match(bugRuns(unit())![0].note, /Run the touched suites at aaaaaaa/);
  assert.equal(bugRuns(unit())![1].note, 'Waits for the fix, in step 5');
  const [before, after] = bugRuns(unit(st('FAILED'), st('PASSED')))!;
  assert.equal(before.pill.text, 'FAIL');
  assert.equal(before.note, 'Red first: done');
  assert.equal(after.pill.text, 'PASS');
  assert.match(bugRuns(unit(st('PASSED')))![0].note, /doesn’t show the bug/);
  assert.match(bugRuns(unit(undefined, st('FAILED')))![1].note, /Still red/);
});

test('the feature rule has no bug runs', () => {
  assert.equal(bugRuns(tests({ rule: 'per-criterion' })), null);
});
