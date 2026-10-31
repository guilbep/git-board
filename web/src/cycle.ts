// The words for an issue's step (api/board.ts Cycle): the step bar, the
// step's buttons, the criteria written into the description.
import type { Cycle, CycleStages, IssueEvent, StageStep } from './api/board.ts';
import { approvers, reviewLine } from './review.ts';
import { versions } from './sections.ts';
import { verdicts } from './spec.ts';
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
  if (has('review')) return card('Review', [{ label: 'Review the branch', action: 'review' }]);
  if (has('signoff')) return card(`Sign off ${status}`, [{ label: `Sign off ${status}`, action: 'signoff' }]);
  // a POST: the server moves the status once the checks are proven
  if (has('ready-for-review')) return card('Mark ready for review', [{ label: 'Mark ready for review', action: 'ready-for-review' }]);
  for (const [action, [label, to]] of Object.entries(moves)) {
    if (has(action)) return card(label, [{ label, action, body: { status: to(c.stages) ?? '' } }]);
  }
  return null;
}

// The step bar (components/IssueStep.vue): one cell per step, with its
// evidence when done, whose turn when current, why when locked.
export const STEPS = ['', 'Intent', 'Risk', 'Plan', 'Tests', 'Code', 'Review', 'Verify'];
// the default list's ids by number: "6" is review
export const DEFAULT_IDS: readonly string[] = ['', 'intent', 'risk', 'plan', 'tests', 'code', 'review', 'verify'];
// listOf is the project's list ([cycle] steps), the default seven when the
// server sends none (internal/cycle's list)
export const listOf = (s: CycleStages | undefined): StageStep[] =>
  s?.steps?.length ? s.steps : DEFAULT_IDS.slice(1).map((id, i) => ({ id, kind: id === 'verify' ? 'signoff' : id, name: STEPS[i + 1]! }));
// kindOf is an entry's kind as the step's number, 1 to 7: what its gate,
// evidence and workspace are read by
export const kindOf = (s: StageStep): number => (s.kind === 'signoff' ? 7 : DEFAULT_IDS.indexOf(s.kind));
const LOCKS = ['', '', 'opens at ready', 'opens once the risk is set', 'opens when work starts', 'opens with the test', 'opens when the branch is in review', 'opens when the branch is merged'];

// gate is the step's rule, as internal/cycle/cycle.go words it: the server
// only sends the current step's. rule is step 4's, by kind ([cycle.kinds]).
export function gate(step: number, risk?: string, rule?: string): string {
  if (step === 3 && risk === 'structural') return 'Structural: the plan’s gate approved first';
  if (step === 3 && risk === 'low') return 'Skipped, low risk';
  if (step === 4 && rule === 'none') return 'Skipped: no tests for this kind';
  if (step === 4 && rule === 'failing-test') return 'A test that fails before the fix';
  if (step === 4) return 'The touched suites green, each criterion names its test';
  if (step === 6 && risk === 'structural') return 'Structural: a second human';
  return ['', 'No issue, no agent', 'Class written on the issue', 'Plan approved by the plan role', '', 'A human commits and asks for review', 'A human who can explain the diff', 'Green codes are not proof'][step] ?? '';
}

export interface StepRow {
  id: string;
  i: number; // the entry's place in the list, from 1: the rail's number
  n: number; // its kind's step, 1 to 7: the gate, the evidence, the workspace
  name: string;
  state: 'done' | 'current' | 'locked' | 'skipped';
  line: string;
  gate: string;
}

const short = (sha?: string) => (sha ? sha.slice(0, 7) : '');
const cap = (s: string) => s.charAt(0).toUpperCase() + s.slice(1);
const names = (list: string[]) => (list.length < 2 ? list.join('') : `${list.slice(0, -1).join(', ')} and ${list[list.length - 1]}`);

// steps is one cell per entry of the list. The current one is the
// issue's (at), even when skipped (low risk waits at plan for Start work);
// a skipped kind is skipped wherever it stands; before the first entry
// opens, every cell is to come. riskBy is who set the risk label, events
// the review's approvals and threads (the thread knows, the evidence
// doesn't).
export function steps(c: Cycle, riskBy?: string, events: IssueEvent[] = []): StepRow[] {
  const e = c.evidence ?? {};
  const risk = c.risk ?? e.risk;
  const rule = c.stages.kinds?.[e.kind ?? ''] ?? 'per-criterion';
  // the draft gate holding the plan, read on step 3's line (C3)
  const draft = e.pr?.state === 'OPEN' && e.pr.draft ? ` · GitHub #${e.pr.number}, a draft` : '';
  const evidence = (n: number, status?: string): string | undefined => {
    // a sign-off's own cell: who signed its status off
    if (status) return e.signoffs?.find((x) => x.status === status)?.by.replace(/^/, 'Signed off by ');
    switch (n) {
      case 1: {
        const k = e.criteria?.length ?? 0;
        if (k === 0) return e.no_criteria ? 'accepted without criteria' : 'no criteria';
        return `${k} ${k === 1 ? 'criterion' : 'criteria'}`;
      }
      case 2:
        return risk ? cap(risk) + (riskBy ? `, set by ${riskBy}` : '') : undefined;
      case 3:
        if (e.plan_approval) return `Approved by ${e.plan_approval.by}`;
        if (e.plan_pr) return e.plan_pr.decision === 'APPROVED' || e.plan_pr.state === 'MERGED' ? `The plan’s gate, GitHub #${e.plan_pr.number}, approved` : `The plan’s gate, GitHub #${e.plan_pr.number}, waits for approval`;
        return e.plan?.path && e.plan.path + draft;
      case 4:
        return e.tests?.[0] ? e.tests[0].path + (e.tests[0].commit ? ` at ${short(e.tests[0].commit)}` : '') : undefined;
      case 5:
        if (e.pr?.author) return `Committed by ${e.pr.author}`;
        return e.branch ? e.branch + (e.head ? ` at ${short(e.head)}` : '') : undefined;
      case 6: {
        if (!e.pr) return undefined;
        const by = approvers(e.pr, events);
        if (by.length) return `${names(by)} approved`;
        return e.pr.state === 'MERGED' ? `GitHub #${e.pr.number} merged` : `GitHub #${e.pr.number} open`;
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
  // step 1 with a spec role: the version that waits for its verdict (F2)
  const created = events.find((x) => x.kind === 'created');
  const v = created ? versions(created).length : 0;
  const spec = c.waits?.role === 'spec' && created ? `v${v} · ${verdicts(events).findLast((x) => x.v === v)?.verdict === 'request-changes' ? 'changes requested' : 'awaits the verdict'}` : '';
  const list = listOf(c.stages);
  const cur = list.findIndex((s) => s.id === c.at) + 1;
  return list.map((s, k): StepRow => {
    const i = k + 1;
    const n = kindOf(s);
    const row = { id: s.id, i, n, name: s.name, gate: i === cur ? c.gate : gate(n, risk, rule) };
    // step 6 under review counts approvals and open threads; merged, its note
    if (i === cur && !c.done) return { ...row, state: 'current', line: (c.note || spec || (n === 6 ? reviewLine(c, events) : turn(c))) + (n === 3 ? draft : '') };
    if (c.skipped?.includes(n)) return { ...row, state: 'skipped', line: `skipped, ${n === 3 ? 'low risk' : (e.kind ?? 'no tests')}` };
    if (c.done || i < cur) return { ...row, state: 'done', line: evidence(n, s.status) || row.gate };
    return { ...row, state: 'locked', line: s.status ? `opens at ${s.status}` : (LOCKS[n] ?? '') };
  });
}


// suggestion is the AI's newest class for step 2 (internal/server/cycle.go
// suggestRisk): the class, and the sentence under "Suggested: **class**".
export function suggestion(events: IssueEvent[]): { risk: string; why: string } | null {
  const e = events.findLast((e) => e.kind === 'comment' && e.meta?.['ai:risk']);
  if (!e?.meta) return null;
  return { risk: e.meta['ai:risk'] ?? '', why: (e.body ?? '').replace(/^Suggested:.*\n*/, '').trim() };
}

// riskWords is what a class does next, for step 2's gate (C2)
export const riskWords = (risk: string) => (risk === 'low' ? 'low skips step 3, Start work opens step 4' : `${risk} opens step 3`);

// The headings and items internal/cycle/cycle.go's Criteria reads.
const marker = /^\s*([-*+]|\d+[.)])\s+/;
const box = /^\[([ xX])\]\s*/;
const esc = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
const headingOf = (hs: string[]) => new RegExp(`^\\s*(#{1,6}\\s*|\\*\\*)(${hs.map(esc).join('|')}):?(\\*\\*)?:?\\s*$`, 'i');

// criterionOf is a line's criterion: its list marker and box stripped,
// done when the box is ticked
export function criterionOf(line: string): { text: string; done: boolean } {
  const t = line.trim().replace(marker, '');
  return { text: t.replace(box, '').trim(), done: /^\[[xX]\]/.test(t) };
}

// withCriteria is the description with the criteria written as a "- [ ]"
// list: in place of the list under the first criteria heading it has (the
// one the server reads), a criterion ticked there kept ticked; under a new
// heading, the first of headings, at the end otherwise.
export function withCriteria(description: string, lines: string[], headings: string[] = ['Acceptance criteria']): string {
  const ls = description.split('\n');
  const re = headingOf(headings.length ? headings : ['Acceptance criteria']);
  const h = ls.findIndex((l) => re.test(l));
  let at = h + 1;
  const ticked = new Set<string>();
  for (; h >= 0 && at < ls.length && (!ls[at]?.trim() || marker.test(ls[at] ?? '') || box.test(ls[at]?.trim() ?? '')); at++) {
    const c = criterionOf(ls[at] ?? '');
    if (c.done) ticked.add(c.text);
  }
  const list = lines
    .map(criterionOf)
    .filter((c) => c.text)
    .map((c) => `- [${c.done || ticked.has(c.text) ? 'x' : ' '}] ${c.text}`);
  if (h < 0) return `${description.trimEnd()}${description.trim() ? '\n\n' : ''}## ${headings[0] ?? 'Acceptance criteria'}\n\n${list.join('\n')}`;
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
// the run step 7 reads: the newest at the merge commit once the PR is merged,
// else the newest; merge says it is at the merge commit
export function mergedRun(c: Cycle) {
  const runs = c.evidence?.runs ?? [];
  const pr = c.evidence?.pr;
  const merge = pr?.state === 'MERGED' ? pr.merge_commit : undefined;
  const run = (merge && runs.findLast((r) => r.commit === merge)) || runs.at(-1);
  return run ? { ...run, merge: !!merge && run.commit === merge } : null;
}

// the acceptance criteria nobody has checked yet
export const criteriaLeft = (c: Cycle) => (c.evidence?.criteria ?? []).filter((_, i) => !c.evidence?.checks?.some((k) => k.index === i)).length;

// what signing off does, as the server's signedOff: the next validation
// status, or the closing one, which closes the issue (and its forge copy)
export function signoffLine(c: Cycle, status: string, forge?: string): string {
  const v = c.stages?.validation ?? [];
  const i = v.indexOf(status);
  const next = i + 1 < v.length ? v[i + 1] : c.stages?.closing;
  if (!next) return 'The sign-off is written on the issue; its status stays.';
  if (next !== c.stages.closing) return `The sign-off moves the issue to ${next}.`;
  return `The sign-off closes the issue as ${next}${forge ? `, and ${forge} with it` : ''}.`;
}

export function statusBar(c: Cycle, waiting: number | null): string[] {
  const e = c.evidence ?? {};
  const out = [e.branch ? e.branch + (e.head ? ` at ${short(e.head)}` : '') : 'no branch yet'];
  if (!e.pr) out.push('no gate yet'); // not "not in review": in-review needs no PR (cycle.go)
  else if (e.pr.state === 'MERGED') out.push(`GitHub #${e.pr.number} merged`);
  else if (e.pr.draft) out.push(`GitHub #${e.pr.number}, a draft`);
  else if (e.pr.checks?.length) out.push(`GitHub #${e.pr.number} · ${e.pr.checks.filter((k) => k.state === 'success').length} of ${e.pr.checks.length} checks green`);
  else out.push(`GitHub #${e.pr.number} open`);
  if (waiting === null) return out;
  out.push(waiting ? `waits for ${waiting} change${waiting === 1 ? '' : 's'} on the forges` : 'in sync');
  return out;
}
