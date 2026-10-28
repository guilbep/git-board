// Which page an issue opens on (docs/prd/classic-view.md): the classic
// page, unless the board's .git-board.toml has a [cycle] table or the
// person's git config says board.view = cycle. /api/me says it, and why;
// Settings › You says it in words.
import type { MeResponse } from './api/board';

const why = {
  default: 'the board’s .git-board.toml has no [cycle] table.',
  '.git-board.toml': 'the board’s .git-board.toml has a [cycle] table.',
  'board.view': 'your git config sets board.view.',
} as const;

export function viewWords(m: Pick<MeResponse, 'view' | 'view_from' | 'view_ignored'> | null): string {
  if (!m?.view) return '';
  const page = m.view === 'cycle' ? 'the cycle’s page' : 'the classic page';
  const line = `Your issues open on ${page}: ${why[m.view_from ?? 'default']}`;
  return m.view_ignored ? `${line} board.view = “${m.view_ignored}” is neither classic nor cycle, so it’s ignored.` : line;
}
