// The issue page's cycle panel (components/CyclePanel.vue): the steps'
// cells, the one step's controls from the actions the server offers the
// viewer (api.md route 50), the criteria to check at Verify, the PR's
// line. v1's IssueStep and its Cycle* panels, lean. Pure: node runs
// cycleact.test.ts.
import type { Cycle, GatePR } from './api/cycletypes.ts';
import { kindOf, listOf } from './cycle.ts';

export interface Cell {
  id: string;
  name: string;
  n: number; // its place, from 1
  state: 'done' | 'current' | 'ahead' | 'skipped';
}

// cells are the project's steps: those before the issue's done (or
// skipped, low risk's Plan), its own current, the rest ahead.
export function cells(c: Cycle): Cell[] {
  const list = listOf(c.stages);
  const at = list.findIndex((s) => s.id === c.at);
  return list.map((s, i) => {
    let state: Cell['state'] = i < at || c.done ? 'done' : i === at ? 'current' : 'ahead';
    if (state !== 'current' && c.skipped?.includes(kindOf(s))) state = 'skipped';
    return { id: s.id, name: s.name, n: i + 1, state };
  });
}

// Control is what the step shows the viewer: one press, or a form.
export type Control =
  | { kind: 'press'; action: string; label: string }
  | { kind: 'risk' } // Low, Normal, Structural
  | { kind: 'verdict' } // Approve, or Request changes with a comment
  | { kind: 'review' } // Approve, Request changes or Comment, with a line explaining it
  | { kind: 'verify' }; // the criteria's checks, a run, the sign-off

// controls are the step's, from the actions the viewer may take now.
export function controls(c: Cycle): Control[] {
  const s = c.stages;
  const out: Control[] = [];
  for (const a of c.actions ?? []) {
    switch (a) {
      case 'set-risk':
        out.push({ kind: 'risk' });
        break;
      case 'spec-verdict':
        out.push({ kind: 'verdict' });
        break;
      case 'review':
        out.push({ kind: 'review' });
        break;
      case 'verify':
        out.push({ kind: 'verify' });
        break;
      case 'check':
      case 'signoff':
        break; // Verify's own
      case 'no-criteria':
        out.push({ kind: 'press', action: a, label: 'Accept without criteria' });
        break;
      case 'move-ready':
        out.push({ kind: 'press', action: a, label: `Move to ${s.ready}` });
        break;
      case 'start':
        out.push({ kind: 'press', action: a, label: `Start: move to ${s.in_progress[0] ?? 'in progress'}` });
        break;
      case 'approve-plan':
        out.push({ kind: 'press', action: a, label: 'Approve the plan' });
        break;
      case 'ready-for-review':
        out.push({ kind: 'press', action: a, label: 'Mark ready for review' });
        break;
      case 'to-validation':
        out.push({ kind: 'press', action: a, label: `Move to ${s.validation[0] ?? 'validation'}` });
        break;
    }
  }
  return out;
}

export interface Criterion {
  index: number;
  text: string;
  by?: string; // who checked it, when
}

// criteria are the description's, each with its check at Verify.
export function criteria(c: Cycle): Criterion[] {
  const ev = c.evidence;
  return (ev.criteria ?? []).map((text, index) => {
    const ch = ev.checks?.findLast((x) => x.index === index);
    return ch ? { index, text, by: ch.by } : { index, text };
  });
}

// ranAtHead: a run was pasted at the evidence's head (or there is no
// branch): the sign-off needs no reason.
export function ranAtHead(c: Cycle): boolean {
  const ev = c.evidence;
  return (ev.runs ?? []).some((r) => !ev.head || r.commit === ev.head || (!!ev.pr?.merge_commit && r.commit === ev.pr.merge_commit));
}

// prLine is a PR as its chip says it: "#7 open, draft", "#7 merged".
export function prLine(pr: GatePR): string {
  const state = { OPEN: 'open', MERGED: 'merged', CLOSED: 'closed' }[pr.state] ?? pr.state.toLowerCase();
  return `#${pr.number} ${state}${pr.draft ? ', draft' : ''}`;
}

// short is a commit as git shows it.
export const short = (sha?: string) => (sha ?? '').slice(0, 7);
