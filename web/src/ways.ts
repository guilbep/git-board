// The sync page's words: what each way would move, in counts.
import type { Ahead, BridgeTally, ForgeMode, Remote, SyncReport } from './api/board';

const n = (v: number) => v.toLocaleString();
export const plural = (v: number, one: string, many = `${one}s`) => `${n(v)} ${v === 1 ? one : many}`;

// "3 issues to send · 0 to receive"
export function aheadWords(a: Ahead, one: string, many?: string): string {
  return `${plural(a.send, one, many)} to send · ${n(a.receive)} to receive`;
}

// what a push to a remote sends: "3 issues and 1 person", or, never
// checked, the lot
export function pushWhat(r: Remote): string {
  if (!r.tracked && !r.checked) return 'the issues and people';
  const parts = [];
  if (r.issues.send || !r.people.send) parts.push(plural(r.issues.send, 'issue'));
  if (r.people.send) parts.push(plural(r.people.send, 'person', 'people'));
  return parts.join(' and ');
}

// a check over every forge: "3 issues to get, 2 to send"
export function checkWords(bridges: Record<string, BridgeTally>): string {
  let get = 0;
  let send = 0;
  for (const t of Object.values(bridges)) {
    get += t.from.issues + t.from.changed;
    send += t.to.issues + t.to.changed;
  }
  return `${plural(get, 'issue')} to get, ${n(send)} to send`;
}

// one forge's part of a run: "create 2 issues, post 3 comments, push 1
// edit or change, pull 2 issues", the half of its mode for get and send
export function runWords(mode: ForgeMode, t: BridgeTally): string {
  const parts = [];
  if (mode !== 'get') {
    parts.push(
      `create ${plural(t.to.issues, 'issue')}`,
      `post ${plural(t.to.comments, 'comment')}`,
      `push ${plural(t.to.edits + t.to.changes, 'edit or change', 'edits and changes')}`,
    );
  }
  if (mode !== 'send') parts.push(`pull ${plural(t.from.issues + t.from.changed, 'issue')}`);
  return parts.join(', ');
}

// what a sync with the Project would do: "update the labels of 2 issues
// and write 1 Project field", leaving out what's zero
export function pendingWords(r: SyncReport): string {
  const parts = [];
  if (r.label_changes) parts.push(`update the labels of ${plural(r.label_changes, 'issue')}`);
  if (r.project_writes) parts.push(`write ${plural(r.project_writes, 'Project field')}`);
  return parts.join(' and ');
}
