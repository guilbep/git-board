// Sync's words: a run (POST /api/sync, its steps as progress events), a
// remote's fetch, what each moved. Pure: node runs ways.test.ts on it.
import type { Ahead, RemoteResult, SyncRun, SyncStep } from './api/types.ts';

const n = (v: number) => v.toLocaleString();
export const plural = (v: number, one: string, many = `${one}s`) => `${n(v)} ${v === 1 ? one : many}`;

// "3 issues to send · 0 to receive"
export function aheadWords(a: Ahead, one: string, many?: string): string {
  return `${plural(a.send, one, many)} to send · ${n(a.receive)} to receive`;
}

// what a remote's action answered: a fetch what waits, a pull or a push
// its note
export function remoteWords(r: RemoteResult): string {
  if (r.action === 'fetch') {
    const parts = [r.issues && aheadWords(r.issues, 'issue'), r.people && aheadWords(r.people, 'person', 'people')].filter(Boolean);
    return parts.length ? parts.join('; ') : 'Up to date';
  }
  const done = r.action === 'pull' ? 'Pulled' : 'Pushed';
  return r.note ? `${done}: ${r.note}` : done;
}

const stepNames: Record<string, string> = { pull: 'git pull', forges: 'the forges', project: 'the GitHub Project', push: 'git push' };

// a step as a line: a whole sync's ("git pull: done"), a forge's event
// ("github: created #12")
export function stepWords(s: SyncStep): string {
  if (s.name) return `${stepNames[s.name] ?? s.name}: ${s.state ?? ''}${s.line ? ` (${s.line})` : ''}`;
  const where = [s.forge, s.way].filter(Boolean).join(' ');
  const count = s.total ? ` ${n(s.done ?? 0)} of ${n(s.total)}` : '';
  return `${where ? `${where}: ` : ''}${s.line || [s.what, s.issue].filter(Boolean).join(' ')}${count}`;
}

// the counts of a forge run: "2 created, 3 updated, 1 posted"
export function reportWords(r: SyncRun['report']): string {
  if (!r) return '';
  const parts = (['created', 'updated', 'posted', 'pushed'] as const).filter((k) => r[k]).map((k) => `${n(r[k])} ${k}`);
  return parts.length ? parts.join(', ') : 'nothing to change';
}

// what the Sync button says of the run in progress or the last one
export function runWords(run: SyncRun | null): string {
  if (!run) return 'not synced from here yet';
  if (run.state === 'running') return 'syncing now';
  if (run.state === 'failed') return `the last sync failed${run.error ? `: ${run.error}` : ''}`;
  const what = run.scope === 'all' ? 'synced' : run.dry_run ? 'checked the forges' : 'synced the forges';
  const counts = reportWords(run.report);
  return counts ? `${what}: ${counts}` : what;
}
