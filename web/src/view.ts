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

// the git config level that set board.view
function setBy(scope: string | undefined): string {
  if (scope === 'local') return 'this repository’s git config sets board.view.';
  return scope ? `your ${scope} git config sets board.view.` : why['board.view'];
}

export function viewWords(m: Pick<MeResponse, 'view' | 'view_from' | 'view_ignored' | 'view_scope'> | null): string {
  if (!m?.view) return '';
  const page = m.view === 'cycle' ? 'the cycle’s page' : 'the classic page';
  const line = `Your issues open on ${page}: ${m.view_from === 'board.view' ? setBy(m.view_scope) : why[m.view_from ?? 'default']}`;
  return m.view_ignored ? `${line} board.view = “${m.view_ignored}” is neither classic nor cycle, so it’s ignored.` : line;
}
