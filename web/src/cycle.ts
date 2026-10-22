// The words for an issue's step (api/board.ts Cycle): the step bar, the
// step's buttons, the criteria written into the description.
import type { Cycle, CycleStages, IssueEvent } from './api/board.ts';
import { on } from './when.ts';

// turn is whose move it is: "waits for erin to set the risk class"
export function turn(c: Pick<Cycle, 'waits'>): string {
  return c.waits.anyone ? `anyone’s turn to ${c.waits.what}` : `waits for ${c.waits.login} to ${c.waits.what}`;
}

export interface CardButton {
  label: string;
  action: string; // a PATCH of a field when in patches; else POST …/cycle/{action}
  body?: Record<string, unknown>;
}

// The step buttons that move the status: a PATCH to the stage they name.
const moves: Record<string, [string, (s: CycleStages) => string | undefined]> = {
  'move-ready': ['Move to ready', (s) => s.ready],
  start: ['Start work', (s) => s.in_progress[0]],
  'ready-for-review': ['Mark ready for review', (s) => s.in_review[0]],
  'to-validation': ['Start verification', (s) => s.validation[0]],
};
export const patches = ['set-risk', ...Object.keys(moves)];

// cardFor is the step's title and buttons, or null when it isn't the
// viewer's turn or the step has nothing to press (tests, code).
// verify and check are CycleVerify's run box, not buttons.
export function cardFor(c: Cycle, status: string): { title: string; gate: string; buttons: CardButton[] } | null {
  const has = (a: string) => !!c.actions?.includes(a);
  if (!c.mine || !c.actions?.length) return null;
  const card = (title: string, buttons: CardButton[]) => ({ title, gate: c.gate, buttons });
  if (has('set-risk')) return card('Set the risk', ['low', 'normal', 'structural'].map((risk) => ({ label: risk, action: 'set-risk', body: { risk } })));
  if (has('approve-plan')) return card('Approve the plan', [{ label: 'Approve the plan', action: 'approve-plan' }]);
  if (has('review')) return card('Review', [{ label: 'Review the PR', action: 'review' }]);
  if (has('signoff')) return card(`Sign off ${status}`, [{ label: `Sign off ${status}`, action: 'signoff' }]);
  for (const [action, [label, to]] of Object.entries(moves)) {
    if (has(action)) return card(label, [{ label, action, body: { status: to(c.stages) ?? '' } }]);
  }
  return null;
}

// The step bar (components/IssueStep.vue): one cell per step, with its
// evidence when done, whose turn when current, why when locked.
export const STEPS = ['', 'Intent', 'Risk', 'Plan', 'Tests', 'Code', 'Review', 'Verify'];
const LOCKS = ['', '', 'opens at ready', 'opens once the risk is set', 'opens when work starts', 'opens with the test', 'opens when the PR is ready for review', 'opens when the PR is merged'];

// gate is the step's rule, as internal/cycle/cycle.go words it: the server
// only sends the current step's. rule is step 4's, by kind ([cycle.kinds]).
export function gate(step: number, risk?: string, rule?: string): string {
  if (step === 3 && risk === 'structural') return 'Structural: plan PR approved first';
  if (step === 3 && risk === 'low') return 'Skipped, low risk';
  if (step === 4 && rule === 'none') return 'Skipped: no tests for this kind';
  if (step === 4 && rule === 'failing-test') return 'A test that fails before the fix';
  if (step === 4) return 'The touched suites green, each criterion names its test';
  if (step === 6 && risk === 'structural') return 'Structural: a second human';
  return ['', 'No issue, no agent', 'Class written on the issue', 'Plan approved by the plan role', '', 'A human commits and opens the PR', 'A human who can explain the diff', 'Green codes are not proof'][step] ?? '';
}

export interface StepRow {
  n: number;
  name: string;
  state: 'done' | 'current' | 'locked' | 'skipped';
  line: string;
  gate: string;
}

const short = (sha?: string) => (sha ? sha.slice(0, 7) : '');
const cap = (s: string) => s.charAt(0).toUpperCase() + s.slice(1);
const names = (list: string[]) => (list.length < 2 ? list.join('') : `${list.slice(0, -1).join(', ')} and ${list[list.length - 1]}`);

// steps is the seven cells. The current step is the issue's, even when
// skipped (low risk waits at step 3 for Start work); a skipped one is
// skipped wherever it stands. riskBy is who set the risk label (the thread
// knows, the evidence doesn't).
export function steps(c: Cycle, riskBy?: string): StepRow[] {
  const e = c.evidence ?? {};
  const risk = c.risk ?? e.risk;
  const rule = c.stages.kinds?.[e.kind ?? ''] ?? 'per-criterion';
  const evidence = (n: number): string | undefined => {
    switch (n) {
      case 1: {
        const k = e.criteria?.length ?? 0;
        return k === 0 ? 'no criteria' : `${k} ${k === 1 ? 'criterion' : 'criteria'}`;
      }
      case 2:
        return risk ? cap(risk) + (riskBy ? `, set by ${riskBy}` : '') : undefined;
      case 3:
        if (e.plan_approval) return `Approved by ${e.plan_approval.by}`;
        if (e.plan_pr) return e.plan_pr.decision === 'APPROVED' || e.plan_pr.state === 'MERGED' ? `Plan PR #${e.plan_pr.number} approved` : `Plan PR #${e.plan_pr.number}, waits for approval`;
        return e.plan?.path;
      case 4:
        return e.tests?.[0] ? e.tests[0].path + (e.tests[0].commit ? ` at ${short(e.tests[0].commit)}` : '') : undefined;
      case 5:
        if (e.pr?.author) return `Committed by ${e.pr.author}`;
        return e.branch ? e.branch + (e.head ? ` at ${short(e.head)}` : '') : undefined;
      case 6: {
        if (!e.pr) return undefined;
        const by = (e.pr.reviews ?? []).filter((r) => r.state === 'APPROVED').map((r) => r.login);
        if (by.length) return `${names(by)} approved`;
        return e.pr.state === 'MERGED' ? `PR #${e.pr.number} merged` : `PR #${e.pr.number} open`;
      }
      case 7: {
        const n = c.stages?.validation?.length ?? 0;
        const k = e.signoffs?.length ?? 0;
        if (c.done) return `${k} sign-off${k === 1 ? '' : 's'}`;
        return n ? `Sign-off ${Math.min(k + 1, n)} of ${n}` : undefined;
      }
    }
    return undefined;
  };
  return STEPS.slice(1).map((name, i) => {
    const n = i + 1;
    const g = n === c.step ? c.gate : gate(n, risk, rule);
    if (n === c.step && !c.done) return { n, name, state: 'current', line: c.note || turn(c), gate: g };
    if (c.skipped?.includes(n)) return { n, name, state: 'skipped', line: `skipped, ${n === 3 ? 'low risk' : (e.kind ?? 'no tests')}`, gate: g };
    if (c.done || n < c.step) return { n, name, state: 'done', line: evidence(n) || g, gate: g };
    return { n, name, state: 'locked', line: LOCKS[n] ?? '', gate: g };
  });
}

// forStep is a step's discussion: the comments carrying its cycle:step.
// Step 1's also has those carrying none, every one written before steps.
export function forStep(events: IssueEvent[], n: number): IssueEvent[] {
  return events.filter((e) => e.kind === 'comment' && (e.meta?.['cycle:step'] ?? '1') === String(n));
}

// The headings and items internal/cycle/cycle.go's Criteria reads.
const typedItem = /^([-*+]|\d+[.)])\s+|^\[[ xX]\]\s*/;
const esc = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
const headingOf = (hs: string[]) => new RegExp(`^\\s*(#{1,6}\\s*|\\*\\*)(${hs.map(esc).join('|')}):?(\\*\\*)?:?\\s*$`, 'i');

// withCriteria is the description with the criteria written as a list:
// under the first criteria heading it has (the one the server reads),
// under a new one, the first of headings, at the end otherwise.
export function withCriteria(description: string, lines: string[], headings: string[] = ['Acceptance criteria']): string {
  const list = lines
    .map((l) => l.trim().replace(typedItem, '').replace(typedItem, '').trim())
    .filter(Boolean)
    .map((l) => `- ${l}`);
  const ls = description.split('\n');
  const re = headingOf(headings.length ? headings : ['Acceptance criteria']);
  const h = ls.findIndex((l) => re.test(l));
  if (h < 0) return `${description.trimEnd()}${description.trim() ? '\n\n' : ''}## ${headings[0] ?? 'Acceptance criteria'}\n\n${list.join('\n')}`;
  let at = h + 1;
  while (at < ls.length && !ls[at]?.trim()) at++;
  const rest = ls.slice(at);
  return [...ls.slice(0, h + 1), '', ...list, ...(rest.length ? ['', ...rest] : [])].join('\n');
}

export interface Tile {
  status: string;
  state: 'done' | 'current' | 'next';
  by: string | undefined;
  when: string;
}

// tiles is one per validation status, in the configured order.
export function tiles(c: Cycle, status: string): Tile[] {
  return (c.stages?.validation ?? []).map((s) => {
    const so = c.evidence?.signoffs?.find((x) => x.status === s);
    if (so) return { status: s, state: 'done', by: so.by, when: `Signed off ${on(so.at)}` };
    return { status: s, state: s === status ? 'current' : 'next', by: undefined, when: s === status ? 'Now' : 'Next' };
  });
}

// statusBar reads what the side already knows: the branch, the PR and its
// checks, what waits for the forges (null: no forge). Nothing new is fetched.
// The step adds the worktree and the agent (getIssueAgent).
export function statusBar(c: Cycle, waiting: number | null): string[] {
  const e = c.evidence ?? {};
  const out = [e.branch ? e.branch + (e.head ? ` at ${short(e.head)}` : '') : 'no branch yet'];
  if (!e.pr) out.push('no PR');
  else if (e.pr.state === 'MERGED') out.push(`PR #${e.pr.number} merged`);
  else if (e.pr.checks?.length) out.push(`PR #${e.pr.number} · ${e.pr.checks.filter((k) => k.state === 'success').length} of ${e.pr.checks.length} checks green`);
  else out.push(`PR #${e.pr.number} open`);
  if (waiting === null) return out;
  out.push(waiting ? `waits for ${waiting} change${waiting === 1 ? '' : 's'} on the forges` : 'in sync');
  return out;
}
