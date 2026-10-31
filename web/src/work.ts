// The words of the Work page (pages/WorkPage.vue, api/board.ts WorkRow):
// what a row waits for, the one action to press, how long ago.
import type { WorkRow } from './api/board.ts';

// ago: "just now", "12 min", "2 h", "3 d", "5 w"
export function ago(iso: string, now: Date = new Date()): string {
  const s = Math.max(0, (now.getTime() - new Date(iso).getTime()) / 1000);
  if (s < 60) return 'just now';
  if (s < 3600) return `${Math.floor(s / 60)} min`;
  if (s < 86400) return `${Math.floor(s / 3600)} h`;
  if (s < 7 * 86400) return `${Math.floor(s / 86400)} d`;
  return `${Math.floor(s / (7 * 86400))} w`;
}

const cut = (s: string, n: number) => (s.length > n ? s.slice(0, n - 1).trimEnd() + '…' : s);

// waitsFor is the "Waits for" cell: what the viewer owes, or who the row
// waits for.
export function waitsFor(r: Pick<WorkRow, 'waits' | 'mine' | 'risk' | 'question'>): string {
  if (r.question) return `Your answer: "${cut(r.question.body.trim(), 80)}"`;
  const w = r.waits;
  if (w.anyone) return `Anyone to ${w.what}`;
  if (!r.mine) return `${w.login} to ${w.what}`;
  if (w.what === 'review the branch') return r.risk === 'structural' ? 'A second human' : 'Your review';
  if (w.what.startsWith('sign off ')) return `Your sign-off: ${w.what.slice('sign off '.length)}`;
  if (w.what.startsWith('move the status to ')) return `The status: ${w.what.slice('move the status to '.length)}`;
  return (
    {
      'set the risk class': 'A risk class',
      'approve the plan': 'Your read of the plan',
      'approve the plan’s gate on GitHub': 'Your approval of the plan’s gate',
      'add acceptance criteria': 'Acceptance criteria',
      'write the failing test': 'The failing test',
      'write the tests': 'The tests',
      'commit and ask for review': 'Your ask for review',
    }[w.what] ?? w.what
  );
}

// Action is the row's one control: the risk buttons, Answer (the panel at
// the question), or a link to the issue's page, at its step.
export type Action = { kind: 'risk' } | { kind: 'answer' } | { kind: 'open'; label: string };

export function actionFor(r: Pick<WorkRow, 'waits' | 'mine' | 'question' | 'actions'>): Action {
  if (r.question) return { kind: 'answer' };
  const has = (a: string) => !!r.mine && !!r.actions?.includes(a);
  if (has('set-risk')) return { kind: 'risk' };
  if (has('approve-plan')) return { kind: 'open', label: 'Read the plan' };
  if (has('review')) return { kind: 'open', label: 'Review' };
  if (has('signoff')) return { kind: 'open', label: `Sign off ${r.waits.what.slice('sign off '.length)}` };
  return { kind: 'open', label: 'Open' };
}

// stepLabel: "06 Review"; the name alone before the list's first step
export function stepLabel(step: number, name: string): string {
  return step ? `${String(step).padStart(2, '0')} ${name}` : name;
}

export function sendWords(pending: number): string {
  if (pending === 0) return 'Nothing to send';
  return `${pending} change${pending === 1 ? '' : 's'}`;
}

export function retroWords(retro: { sprint?: string; corrections: number }): string {
  if (!retro.sprint) return 'No current sprint: [sprints] is the retro’s clock.';
  if (retro.corrections === 0) return 'No correction flagged this sprint.';
  return `${retro.corrections} agent correction${retro.corrections === 1 ? '' : 's'} flagged in reviews.`;
}
