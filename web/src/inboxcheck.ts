// Which inbox rows stay read across a reload, and the follow state in
// words and as a bell. Pure: the page calls the server.
import type { FollowState, InboxRow } from './api/types.ts';

// mergeRows: the rows after a reload, and which stay read (by the latest
// they had when read). A row you read stays, dimmed, until Refresh, so the
// list doesn't jump; one with something newer since is unread again.
export function mergeRows(fresh: InboxRow[], shown: InboxRow[], read: Map<string, string>): { rows: InboxRow[]; read: Map<string, string> } {
  const stillRead = new Map(read);
  for (const f of fresh) if (stillRead.has(f.issue.id) && stillRead.get(f.issue.id) !== f.latest) stillRead.delete(f.issue.id);
  const kept = shown.filter((r) => stillRead.has(r.issue.id) && !fresh.some((f) => f.issue.id === r.issue.id));
  return { rows: [...fresh, ...kept].sort((a, b) => Date.parse(b.latest) - Date.parse(a.latest)), read: stillRead };
}

const PROJECT = ' (project)'; // domain/issues' inbox: a role held for the whole project

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

// bellState: the bar's bell (docs/design/issue-head, 16g), from followLine.
// Following: a bell in the text color; a project role, a grey bell (new
// and closed only); not following, a grey bell-off. Its label is the
// line's state and reason, the button's name and tooltip.
export function bellState(f: FollowState): { shape: 'bell' | 'bell-off'; tone: 'on' | 'off' | 'part'; label: string } {
  const l = followLine(f);
  const label = `Inbox: ${l.state.toLowerCase()}. ${l.because}`;
  if (l.state === 'Following') return { shape: 'bell', tone: 'on', label };
  if (l.state === 'New and closed only') return { shape: 'bell', tone: 'part', label };
  return { shape: 'bell-off', tone: 'off', label };
}
