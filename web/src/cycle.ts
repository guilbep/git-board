// The words for an issue's step (api/board.ts Cycle): the progress line
// under the title, and the card for whoever's turn it is.
import type { Cycle } from './api/board.ts';

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
