// Which git remotes the inbox checks on opening (a fetch: nothing local
// changes), which offer Pull, what a refused check says, and which rows
// stay read across a reload. Pure: the page calls the server.
import type { FollowState, InboxRow, Remote } from './api/board';

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
// remotes until it ends (internal/server beginWrite).
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

const PROJECT = ' (project)'; // internal/inbox: a role held for the whole project

// followLine: the side column's Inbox block, said. A role you hold for the
// whole project (lead-dev, pm…) brings an issue's creation and closing only.
export function followLine(f: FollowState): { state: string; because: string } {
  if (f.set === true) return { state: 'Following', because: 'You follow it.' };
  if (f.set === false) return { state: 'Not following', because: 'You unfollowed it.' };
  if (f.following) return { state: 'Following', because: `Because: ${f.reasons.join(' · ')}` };
  const roles = f.reasons.filter((r) => r.endsWith(PROJECT)).map((r) => r.slice(0, -PROJECT.length));
  if (roles.length) return { state: 'New and closed only', because: `As ${roles.join(', ')} of the project.` };
  return { state: 'Not following', because: "You're not in it." };
}

// followingLine: what the empty inbox says you follow.
export function followingLine(following: number, views: string[], roles: string[]): string {
  const parts = [`Following ${following} open ${following === 1 ? 'issue' : 'issues'}`];
  if (views.length) parts.push(`${views.length === 1 ? 'view' : 'views'} ${views.join(', ')}`);
  if (roles.length) parts.push(`${roles.join(', ')} of the project: its new and closed issues`);
  return parts.join(' · ');
}
