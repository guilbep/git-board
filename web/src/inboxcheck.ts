// Which git remotes the inbox checks on opening (a fetch: nothing local
// changes), which offer Pull, and what a refused check says. Pure: the
// page calls the server.
import type { Remote } from './api/board';

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
