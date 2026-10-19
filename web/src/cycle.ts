// The words for an issue's step (api/board.ts Cycle): the progress line
// under the title, and the card for whoever's turn it is.
import type { Cycle } from './api/board.ts';
import { on } from './when.ts';

export function progressLine(c: Pick<Cycle, 'step' | 'of' | 'name' | 'done' | 'waits' | 'skipped'>): string {
  if (c.done) return 'Done';
  const skipped = c.skipped?.includes(c.step) ? ', skipped (low risk)' : '';
  const turn = c.waits.anyone ? `, anyone’s turn to ${c.waits.what}` : `, waits for ${c.waits.login} to ${c.waits.what}`;
  return `Step ${c.step} of ${c.of} · ${c.name}${skipped}${turn}`;
}

export interface CardButton {
  label: string;
  action: string; // set-risk is a PATCH of the risk field; the rest POST …/cycle/{action}
  body?: Record<string, unknown>;
}

// cardFor is the card's title and buttons, or null when it isn't the
// viewer's turn or the step has nothing to press (1, 4, 5: edits and code).
// verify and check are the card's run box, not buttons.
export function cardFor(c: Cycle, status: string): { title: string; gate: string; buttons: CardButton[] } | null {
  const has = (a: string) => !!c.actions?.includes(a);
  if (!c.mine || !c.actions?.length) return null;
  const card = (title: string, buttons: CardButton[]) => ({ title, gate: c.gate, buttons });
  if (has('set-risk')) return card('Set the risk', ['low', 'normal', 'structural'].map((risk) => ({ label: risk, action: 'set-risk', body: { risk } })));
  if (has('approve-plan')) return card('Approve the plan', [{ label: 'Approve the plan', action: 'approve-plan' }]);
  if (has('review')) return card('Review', [{ label: 'Review the PR', action: 'review' }]);
  if (has('signoff')) return card(`Sign off ${status}`, [{ label: `Sign off ${status}`, action: 'signoff' }]);
  return null;
}

// The Cycle view (components/IssueCycleView.vue): the spine's one line
// of evidence per step, the sign-off tiles, the status bar's segments.
export const STEPS = ['', 'Intent', 'Risk class', 'Plan', 'Tests first', 'Code', 'Review', 'Verify'];

// gate is the step's rule, as internal/cycle/cycle.go words it: the server
// only sends the current step's, the todo rows show theirs.
export function gate(step: number, risk?: string): string {
  if (step === 3 && risk === 'structural') return 'Structural: plan PR approved first';
  if (step === 3 && risk === 'low') return 'Skipped, low risk';
  if (step === 6 && risk === 'structural') return 'Structural: a second human';
  return ['', 'No issue, no agent', 'Class written on the issue', 'Plan approved by the plan role', 'Every bug fix ships its test', 'A human commits and opens the PR', 'A human who can explain the diff', 'Green codes are not proof'][step] ?? '';
}

export interface SpineRow {
  n: number;
  name: string;
  state: 'done' | 'current' | 'todo';
  line: string;
}

const short = (sha?: string) => (sha ? sha.slice(0, 7) : '');
const cap = (s: string) => s.charAt(0).toUpperCase() + s.slice(1);
const names = (list: string[]) => (list.length < 2 ? list.join('') : `${list.slice(0, -1).join(', ')} and ${list[list.length - 1]}`);

// spine is the seven rows: done under the step, current at it, todo above.
// A row says its evidence, or its gate when there is none yet; the
// current row adds the note. riskBy is who set the risk label (the thread
// knows, the evidence doesn't).
export function spine(c: Cycle, riskBy?: string): SpineRow[] {
  const e = c.evidence ?? {};
  const risk = c.risk ?? e.risk;
  const evidence = (n: number): string | undefined => {
    switch (n) {
      case 1: {
        const k = e.criteria?.length ?? 0;
        return k === 0 ? 'no acceptance criteria' : `${k} acceptance ${k === 1 ? 'criterion' : 'criteria'}`;
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
    const state = c.done || n < c.step ? 'done' : n === c.step ? 'current' : 'todo';
    const skipped = c.skipped?.includes(n);
    let line = skipped ? 'Skipped, low risk' : (state !== 'todo' && evidence(n)) || (n === c.step ? c.gate : gate(n, risk));
    if (state === 'current' && c.note) line += ` · ${c.note}`;
    return { n, name, state, line };
  });
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
// The Cycle view adds the worktree and the agent (getIssueAgent).
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
