// What a change event (live.ts) reloads on the board page, the page's own
// changes heard too:
// - config: the views and fields, then the issues, for a change made
//   elsewhere (this page's own save already shows)
// - issue: the issues, for any write to them, this page's own included:
//   a write from the panel can answer after the panel closed (j, Esc), and
//   its event is then the only news of it
// - sync from elsewhere (a run ended, a remote moved), resync: the issues
// - the rest (progress, the inbox, a person, who writes): nothing.
//   progress comes four times a second during a run.

export function boardReload(e: { kind: string; by?: string | undefined }, clientId: string): 'config' | 'issues' | null {
  const own = !!e.by && e.by === clientId;
  if (e.kind === 'config') return own ? null : 'config';
  if (e.kind === 'issue' || e.kind === 'resync') return 'issues';
  if (e.kind === 'sync') return own ? null : 'issues';
  return null;
}
