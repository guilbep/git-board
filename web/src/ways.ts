// The sync page's words: what each way would move, in counts.
import type { Ahead, BridgeTally, ForgeMode, ForgeStatus, Remote, SyncReport, SyncStatus } from './api/board';

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

// What waits, board-wide, as the header says it: issues to send to and
// get from the remotes and forges, conflicts held, and when it was last
// looked at.
// ponytail: no board-wide count of changes waiting for the forges (a
// snapshot per issue, thread.go onForges); Send counts the git remotes and
// the last check's tallies, and the menu shows the check's time. Add a
// server count if the lag misleads.
export type Waiting = { send: number; get: number; conflicts: number; running: boolean; checked?: string | undefined };

export function waiting(remotes: Remote[] | null, forge: ForgeStatus | null, project: SyncStatus | null): Waiting {
  const w: Waiting = { send: 0, get: 0, conflicts: project?.last?.conflicts.length ?? 0, running: forge?.job?.state === 'running' };
  const times: string[] = [];
  for (const r of remotes ?? []) {
    w.send += r.issues.send;
    if (!r.checked) continue; // what it holds is known once fetched
    w.get += r.issues.receive;
    times.push(r.checked);
  }
  const job = forge?.job;
  if (job && job.state !== 'running' && job.ended) {
    times.push(job.ended);
    // a real run's tallies are what it did, not what waits
    if (job.dry_run && job.state === 'done') {
      for (const t of Object.values(job.bridges)) {
        w.send += t.to.issues + t.to.changed;
        w.get += t.from.issues + t.from.changed;
        w.conflicts += t.conflicts;
      }
    }
  }
  if (project?.last) times.push(project.last.checked_at);
  w.checked = times.sort((a, b) => Date.parse(a) - Date.parse(b)).at(-1); // offsets may differ
  return w;
}

// "Send 3 · Get 2 · 1 conflict", "Syncing…", or "Sync" when nothing waits
export function syncLabel(w: Waiting): string {
  if (w.running) return 'Syncing…';
  const parts = [];
  if (w.send) parts.push(`Send ${n(w.send)}`);
  if (w.get) parts.push(`Get ${n(w.get)}`);
  if (w.conflicts) parts.push(plural(w.conflicts, 'conflict'));
  return parts.join(' · ') || 'Sync';
}

// the tab's dot: yours to send first, then theirs to get
export function faviconDot(w: Waiting): 'send' | 'get' | null {
  return w.send ? 'send' : w.get ? 'get' : null;
}
