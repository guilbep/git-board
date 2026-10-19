// The words of the Releases page (pages/ReleasesPage.vue,
// components/ReleaseJob.vue, api/board.ts Releases): environment marks,
// releases grouped by version, the alerts' sentences, a job's steps.
// Pure, no "@/" imports: node runs releases.test.ts on it.
import type { EnvState, ReleaseJob, ReleaseRow, TargetState } from './api/board.ts';

// prod black, stage canard, dev outlined; off: not there
export type EnvMark = 'prod' | 'stage' | 'dev' | 'off';

export function envMark(role: EnvState['role'], here: boolean): EnvMark {
  if (!here) return 'off';
  return role === 'last' ? 'prod' : role === 'candidate' ? 'stage' : 'dev';
}

export interface VersionGroup {
  version: string; // 1.4.20
  final?: ReleaseRow;
  candidates: ReleaseRow[];
}

// groupReleases: a group per version, in the rows' order (newest first),
// its candidates under it
export function groupReleases(rows: ReleaseRow[]): VersionGroup[] {
  const groups = new Map<string, VersionGroup>();
  for (const r of rows) {
    const v = r.tag.version;
    const version = `${v.major}.${v.minor}.${v.patch}`;
    let g = groups.get(version);
    if (!g) groups.set(version, (g = { version, candidates: [] }));
    if (v.rc) g.candidates.push(r);
    else g.final = r;
  }
  return [...groups.values()];
}

// behindText: "Runs 2 merges behind the dev branch", for the followed
// environment. ponytail: names the environment; the trunk's own name isn't
// in EnvState, and the followed environment is named after it (dev)
export function behindText(env: EnvState): string {
  const n = env.behind.length;
  return `Runs ${n} merge${n === 1 ? '' : 's'} behind the ${env.name} branch`;
}

// byRecent: the targets, the one whose overlay changed last first: in a
// split, the one a deploy already moved
export function byRecent(targets: TargetState[]): TargetState[] {
  const at = (t: TargetState) => (t.since ? Date.parse(t.since.date) : 0);
  return [...targets].sort((a, b) => at(b) - at(a));
}

// runs: what a target runs, as said: its version, else its commit
export const runs = (t: TargetState): string => t.deployed.version || t.deployed.commit.slice(0, 7) || 'no known commit';

const counts = ['no', 'one', 'two', 'three', 'four', 'five'];

// splitText: "prod runs two releases. enack8s is on v1.4.20, openshift
// still on v1.4.19." ponytail: a sentence per target; three targets, two
// of them moved, read "b still on" the new one too
export function splitText(env: EnvState): string {
  const [lead, ...rest] = byRecent(env.targets);
  if (!lead) return '';
  const n = new Set(env.targets.map(runs)).size;
  const parts = [`${lead.name} is on ${runs(lead)}`, ...rest.map((t) => `${t.name} still on ${runs(t)}`)];
  return `${env.name} runs ${counts[n] ?? n} releases. ${parts.join(', ')}.`;
}

// promotion: what the environment at i may promote, a candidate in the one
// before last, to the last as its final; null when nothing, or the last
// runs that final already
export function promotion(envs: EnvState[], i: number): { tag: string; final: string; to: string } | null {
  const e = envs[i];
  const last = envs[i + 1];
  if (!e || e.role !== 'candidate' || last?.role !== 'last' || e.split || !/^v\d+\.\d+\.\d+-rc\.\d+$/.test(e.version)) return null;
  const final = e.version.replace(/-rc\.\d+$/, '');
  return last.version === final ? null : { tag: e.version, final, to: last.name };
}

// stepState: a step as the plan list shows it. The server reports a step
// when it starts, so a step not reported yet is todo; a failed job's last
// step carries its failure, even one that failed between steps.
export function stepState(job: ReleaseJob | undefined, i: number): 'done' | 'now' | 'todo' | 'failed' {
  const step = job?.steps[i];
  if (!job || !step) return 'todo';
  if (step.state === 'failed' || (job.state === 'failed' && i === job.steps.length - 1)) return 'failed';
  return step.state === 'running' ? 'now' : 'done';
}

const doing = { deploy: 'Deploying', cut: 'Cutting', promote: 'Promoting', rollback: 'Rolling back' };
const did = { deploy: 'Deployed', cut: 'Cut', promote: 'Promoted', rollback: 'Rolled back' };
const what = { deploy: 'deploy', cut: 'cut', promote: 'promotion', rollback: 'rollback' };

// jobTitle: "Deploying dev", "Rolled back prod to v1.4.19", "The cut of
// v1.4.20 stopped"
export function jobTitle(j: ReleaseJob): string {
  const s = j.subject || 'a release';
  if (j.state === 'running') return `${doing[j.kind]} ${s}`;
  if (j.state === 'done') return `${did[j.kind]} ${s}`;
  return `The ${what[j.kind]} of ${s} stopped`;
}
