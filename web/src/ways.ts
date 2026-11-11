// Sync's words: a run (POST /api/sync, its steps as progress events), a
// remote's fetch, what each moved. Pure: node runs ways.test.ts on it.
import type { Ahead, RemoteResult, SyncPart, SyncRun, SyncStep, SyncTally } from './api/types.ts';

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

const reportNames = { created: 'created', updated: 'updated', posted: 'posted', pushed: 'pushed', restored: 'put back on a backup' } as const;

// the counts of a forge run: "2 created, 3 updated, 1 posted"
export function reportWords(r: SyncRun['report']): string {
  if (!r) return '';
  const parts = (Object.keys(reportNames) as (keyof typeof reportNames)[]).filter((k) => r[k]).map((k) => `${n(r[k] ?? 0)} ${reportNames[k]}`);
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

// what each tally counts, in words: a write by its operation, an issue's
// change by what happened to it
const tallyNames: Record<string, [string, string]> = {
  created: ['new issue to bring in', 'new issues to bring in'],
  updated: ['issue with changes to take', 'issues with changes to take'],
  linked: ['issue linked by its marker', 'issues linked by their marker'],
  posted: ['issue created there', 'issues created there'],
  pushed: ['issue sent its changes', 'issues sent their changes'],
  restored: ['value put back on the backup', 'values put back on the backup'],
  release: ['release published', 'releases published'],
  'create-comment': ['comment posted', 'comments posted'],
  'edit-comment': ['comment edited', 'comments edited'],
  'set-title': ['title changed', 'titles changed'],
  'set-body': ['description changed', 'descriptions changed'],
  'set-status': ['issue opened or closed', 'issues opened or closed'],
  'change-labels': ['label change', 'label changes'],
  'set-assignees': ['assignee change', 'assignee changes'],
};

// a tally as a line: "12 comments posted", "212 times: refused assignees
// "…" of issue …: no account there"
export function tallyWords(t: SyncTally): string {
  if (t.what.startsWith('warning: ')) return `${plural(t.n, 'time')}: ${t.what.slice('warning: '.length)}`;
  const [one, many] = tallyNames[t.what] ?? [t.what, t.what];
  return `${n(t.n)} ${t.n === 1 ? one : many}`;
}

export interface TallyGroup {
  forge: string;
  ways: { way: string; tallies: SyncTally[] }[];
  warnings: SyncTally[];
}

// a run's tallies by forge, then by way (pull before push), each forge's
// warnings apart, the most frequent first
export function tallyGroups(ts: SyncTally[]): TallyGroup[] {
  const groups: TallyGroup[] = [];
  const byN = (a: SyncTally, b: SyncTally) => b.n - a.n;
  for (const t of ts) {
    const forge = t.forge || '';
    let g = groups.find((x) => x.forge === forge);
    if (!g) groups.push((g = { forge, ways: [], warnings: [] }));
    if (t.what.startsWith('warning: ')) {
      g.warnings.push(t);
      continue;
    }
    const way = t.way || '';
    let w = g.ways.find((x) => x.way === way);
    if (!w) g.ways.push((w = { way, tallies: [] }));
    w.tallies.push(t);
  }
  const order = (way: string) => ['pull', 'push'].indexOf(way) + 1 || 3;
  for (const g of groups) {
    g.ways.sort((a, b) => order(a.way) - order(b.way));
    for (const w of g.ways) w.tallies.sort(byN);
    g.warnings.sort(byN);
  }
  return groups;
}

// the parts of a sync, in a run's order, as a check counts them: the
// person picks those the sync runs, and the others wait for the next
export const syncParts: { id: SyncPart; one: string; many: string }[] = [
  { id: 'pull-changes', one: 'issue to pull that changed', many: 'issues to pull that changed' },
  { id: 'pull-new', one: 'new issue to pull', many: 'new issues to pull' },
  { id: 'create', one: 'issue to create on the forge', many: 'issues to create on the forge' },
  { id: 'send', one: 'issue to send its changes', many: 'issues to send their changes' },
];

// the parts a sync runs, as POST /api/sync takes them: absent for every one
export function onlyOf(chosen: SyncPart[]): SyncPart[] | undefined {
  if (chosen.length === syncParts.length) return undefined;
  return syncParts.map((p) => p.id).filter((id) => chosen.includes(id));
}
