// Step 4's words (components/CycleTests.vue, the canvas's C4): what each
// suite runs, why it is ticked, its result, and the bug rule's two runs.
import type { CheckStatus, StepTests, SuiteRow } from './api/board.ts';

const short = (sha?: string) => (sha ? sha.slice(0, 7) : '');

export const runnable = (s: SuiteRow) => s.where !== 'ci';

// where it runs, as C4 words it
export const whereLabel = (s: SuiteRow) => (runnable(s) ? 'worktree' : 'CI, never run here');

// reason is why a suite is ticked: the first directory the diff touches
// that it reads
export function reason(s: SuiteRow): string {
  const f = s.touched?.[0];
  if (!f) return 'not touched';
  const dir = f.includes('/') ? f.slice(0, f.lastIndexOf('/')) : f;
  return `the diff touches ${dir}`;
}

export interface Pill {
  text: string;
  color: '' | 'green' | 'red';
}

export function pill(st?: CheckStatus): Pill {
  if (st?.verdict === 'PASSED') return { text: 'PASS', color: 'green' };
  if (st?.verdict === 'FAILED') return { text: 'FAIL', color: 'red' };
  return { text: 'not run', color: '' };
}

// preselect: the touched suites and the checks not yet proven, those that
// run here
export function preselect(t: StepTests): string[] {
  return [...t.suites.filter((s) => runnable(s) && s.touched?.length), ...t.checks.filter((c) => runnable(c) && !c.head?.proven)].map((s) => s.name);
}

export interface RunCard {
  pill: Pill;
  line: string;
  note: string;
}

// bugRuns is the bug rule's pair (C4): the touched suites FAIL at the
// commit adding the test, then PASS at the head. A suite that passes
// before the fix doesn't show the bug. Null for another rule.
export function bugRuns(t: StepTests): [RunCard, RunCard] | null {
  if (t.rule !== 'failing-test') return null;
  const touched = t.suites.filter((s) => runnable(s) && s.touched?.length);
  const suites = touched.length ? touched : t.suites.filter(runnable);
  const at = short(t.test_commit);
  const verdicts = (pick: (s: SuiteRow) => CheckStatus | undefined) => suites.map((s) => pick(s)?.verdict);

  const was = verdicts((s) => s.at_test);
  let before: RunCard;
  if (!at) before = { pill: pill(), line: 'FAIL before the fix', note: 'No test file in the diff yet' };
  else if (was.includes('FAILED')) before = { pill: pill({ verdict: 'FAILED' } as CheckStatus), line: `at ${at}, before the fix`, note: 'Red first: done' };
  else if (was.length && was.every((v) => v === 'PASSED')) before = { pill: pill({ verdict: 'PASSED' } as CheckStatus), line: `at ${at}, before the fix`, note: 'Passes before the fix: the test doesn’t show the bug' };
  else before = { pill: pill(), line: `FAIL at ${at}, before the fix`, note: `Run the touched suites at ${at}` };

  const now = verdicts((s) => s.head);
  let after: RunCard;
  if (now.length && now.every((v) => v === 'PASSED')) after = { pill: pill({ verdict: 'PASSED' } as CheckStatus), line: `at ${short(t.head)}, the head`, note: 'Green after the fix' };
  else if (now.includes('FAILED')) after = { pill: pill({ verdict: 'FAILED' } as CheckStatus), line: `at ${short(t.head)}, the head`, note: 'Still red: the fix, in step 5' };
  else after = { pill: pill(), line: 'PASS at the head, after the fix', note: 'Waits for the fix, in step 5' };
  return [before, after];
}

// ran is a result's who and when: "dev-laptop-1a2b, 12s"
export function ran(st?: CheckStatus): string {
  const r = st?.result;
  if (!r) return '';
  return `${st.signer || st.by}, ${Math.max(1, Math.round(r.dur / 1e9))}s`;
}
