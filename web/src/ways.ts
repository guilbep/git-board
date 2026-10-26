// Settings › Sync's words: what each place would move, in counts.
import type { Ahead, BridgeTally, ForgeJob, ForgeStatus, JobStep, Remote, SyncReport, SyncStatus, SyncStep } from './api/board';
import { forgeLabel } from './issuesync.ts';

const n = (v: number) => v.toLocaleString();
export const plural = (v: number, one: string, many = `${one}s`) => `${n(v)} ${v === 1 ? one : many}`;

// "3 issues to send · 0 to receive"
export function aheadWords(a: Ahead, one: string, many?: string): string {
  return `${plural(a.send, one, many)} to send · ${n(a.receive)} to receive`;
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

// one forge's part of a sync: "create 2 issues, post 3 comments, push 1
// edit or change, pull 2 issues"
export function runWords(t: BridgeTally): string {
  return [
    `create ${plural(t.to.issues, 'issue')}`,
    `post ${plural(t.to.comments, 'comment')}`,
    `push ${plural(t.to.edits + t.to.changes, 'edit or change', 'edits and changes')}`,
    `pull ${plural(t.from.issues + t.from.changed, 'issue')}`,
  ].join(', ');
}

// a step of the whole sync, in words: "git pull from origin", "the
// GitHub Project", and how it went
const stepNames = { pull: 'git pull', forges: 'the forges', project: 'the GitHub Project', push: 'git push' } as const;
const stateWords = { waiting: 'waits', running: 'running…', done: 'done', failed: 'failed', skipped: 'skipped' } as const;

export function stepLine(s: JobStep, remote: string, dryRun: boolean): string {
  const name = s.name === 'pull' && dryRun ? 'git fetch' : stepNames[s.name];
  const where = s.name === 'pull' || s.name === 'push' ? `${name} ${s.name === 'push' ? 'to' : 'from'} ${remote}` : name;
  return `${where}: ${stateWords[s.state]}${s.note ? `, ${s.note}` : ''}`;
}

// where a run is: "GitHub (gh) · pulling 12 of 40 · #43 import: users
// lose their work"; a pull counts up with no total
export function stepWords(s: SyncStep, bridges: { name: string; target: string }[]): string {
  const b = bridges.find((b) => b.name === s.bridge);
  const count = s.total ? `${n(s.done)} of ${n(s.total)}` : n(s.done);
  return [b ? forgeLabel(b) : s.bridge, `${s.way === 'pull' ? 'pulling' : 'pushing'} ${count}`, s.issue].filter(Boolean).join(' · ');
}

// a real run's one line: "pulled 2 issues, pushed 0", the half of a
// send, and its errors
export function doneWords(job: ForgeJob): string {
  let get = 0;
  let send = 0;
  for (const t of Object.values(job.bridges)) {
    get += t.from.issues + t.from.changed;
    send += t.to.issues + t.to.changed;
  }
  const parts = [];
  if (job.mode !== 'send') parts.push(`pulled ${plural(get, 'issue')}`);
  parts.push(parts.length ? `pushed ${n(send)}` : `pushed ${plural(send, 'issue')}`);
  if (job.errors) parts.push(plural(job.errors, 'error'));
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
        w.conflicts += t.conflicts ?? 0; // the server stopped serving it with git-bug (M5)
      }
    }
  }
  if (project?.last) times.push(project.last.checked_at);
  w.checked = times.sort((a, b) => Date.parse(a) - Date.parse(b)).at(-1); // offsets may differ
  return w;
}

// what waits, for the header's Sync: "3 to send · 2 to get · 1 conflict",
// "Nothing waits"
export function waitWords(w: Waiting): string {
  const parts = [];
  if (w.send) parts.push(`${n(w.send)} to send`);
  if (w.get) parts.push(`${n(w.get)} to get`);
  if (w.conflicts) parts.push(plural(w.conflicts, 'conflict'));
  return parts.join(' · ') || 'Nothing waits';
}

// the tab's dot: yours to send first, then theirs to get
export function faviconDot(w: Waiting): 'send' | 'get' | null {
  return w.send ? 'send' : w.get ? 'get' : null;
}
