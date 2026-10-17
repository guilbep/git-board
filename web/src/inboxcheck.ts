// Which git remotes the inbox checks on opening (a fetch: nothing local
// changes), which offer Pull, what a refused check says, and which rows
// stay read across a reload. Pure: the page calls the server.
import type { InboxRow, Remote } from './api/board';

// FRESH: a remote checked this recently isn't checked again on opening
// the inbox (a reload); Refresh checks regardless.
export const FRESH = 5 * 60 * 1000;

export function toCheck(remotes: Remote[], now: number, force: boolean): string[] {
  return remotes.filter((r) => force || !r.checked || now - Date.parse(r.checked) > FRESH).map((r) => r.name);
}

// toPull: the remotes with issues to receive, for "3 issues new · Pull".
export function toPull(remotes: Remote[]): { name: string; issues: number }[] {
  return remotes.filter((r) => r.issues.receive > 0).map((r) => ({ name: r.name, issues: r.issues.receive }));
}

// checkFailure: a remote's check refused. syncing: a forge sync holds the
// remotes until it ends (cmd/git-board-server beginWrite).
export function checkFailure(name: string, code: string, message: string): string {
  if (code === 'syncing') return `${name}: a forge sync is running: check again when it ends`;
  return `${name}: couldn't check: ${message}`;
}

// mergeRows: the rows after a reload, and which stay read (by the latest
// they had when read). A row you read stays, dimmed, until Refresh, so the
// list doesn't jump; one with something newer since is unread again.
export function mergeRows(fresh: InboxRow[], shown: InboxRow[], read: Map<string, string>): { rows: InboxRow[]; read: Map<string, string> } {
  const stillRead = new Map(read);
  for (const f of fresh) if (stillRead.has(f.issue.id) && stillRead.get(f.issue.id) !== f.latest) stillRead.delete(f.issue.id);
  const kept = shown.filter((r) => stillRead.has(r.issue.id) && !fresh.some((f) => f.issue.id === r.issue.id));
  return { rows: [...fresh, ...kept].sort((a, b) => Date.parse(b.latest) - Date.parse(a.latest)), read: stillRead };
}
