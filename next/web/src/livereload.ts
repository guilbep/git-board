// What a live event (live.ts) reloads on the board page, the page's own
// changes heard too:
// - config: the views and fields, then the issues, for a change made
//   elsewhere (this page's own save already shows)
// - issues: the issues, for any write to them, this page's own included:
//   a write from the panel can answer after the panel closed (j, Esc), and
//   its changed is then never heard
// - sync from elsewhere: the issues
// - the rest (a forge run's progress, the inbox, settings, releases, a
//   cycle step): nothing. progress comes every 200 ms during a run.

export function boardReload(e: { kind: string; by?: string }, clientId: string): 'config' | 'issues' | null {
  const own = !!e.by && e.by === clientId;
  if (e.kind === 'config') return own ? null : 'config';
  if (e.kind === 'issues') return 'issues';
  if (e.kind === 'sync') return own ? null : 'issues';
  return null;
}
