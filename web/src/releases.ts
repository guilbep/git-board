// The words of the Releases page (pages/ReleasesPage.vue,
// components/ReleaseJob.vue, api/board.ts Releases): environment marks,
// releases grouped by version, the alerts' sentences, a job's steps.
// Pure, no "@/" imports: node runs releases.test.ts on it.
import type { EnvState, ReleaseJob, ReleaseRow, ReleaseStep, TargetState } from './api/board.ts';

// a release tag: v1.4.20, v1.4.20-rc.2 (release.ParseTag)
export const tagPattern = /^v(\d+)\.(\d+)\.(\d+)(?:-rc\.(\d+))?$/;

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

// byVersion: the targets, the highest version first: in a split, the one a
// deploy already moved (deploys only move forward). A release tag is above
// anything else, a final above its candidates. ponytail: two versions that
// aren't release tags (dev's 1.4.21-dev.3.gabc counts from the last tag,
// so it doesn't order) go by the overlay changed last
export function byVersion(targets: TargetState[]): TargetState[] {
  const rank = (t: TargetState) => {
    const m = tagPattern.exec(t.deployed.version);
    return m ? [1, +m[1]!, +m[2]!, +m[3]!, m[4] ? +m[4] : Infinity] : [0, t.since ? Date.parse(t.since.date) : 0];
  };
  const cmp = (a: number[], b: number[]) => a.reduce((d, x, i) => d || x - (b[i] ?? 0), 0);
  return [...targets].sort((a, b) => cmp(rank(b), rank(a)));
}

// newest: the newest of the targets' overlay changes, null when none
export function newest(targets: TargetState[]): TargetState['since'] {
  return targets.reduce<TargetState['since']>((n, t) => (t.since && (!n || Date.parse(t.since.date) > Date.parse(n.date)) ? t.since : n), null);
}

// runs: what a target runs, as said: its version, else its commit
export const runs = (t: TargetState): string => t.deployed.version || t.deployed.commit.slice(0, 7) || 'no known commit';

const counts = ['no', 'one', 'two', 'three', 'four', 'five'];

// splitParts: the split alert, its bold sentence and the rest: "prod runs
// two releases.", "enack8s is on v1.4.20, openshift still on v1.4.19."
// ponytail: a clause per target; three targets, two of them moved, read
// "b still on" the new one too
export function splitParts(env: EnvState): [string, string] {
  const [lead, ...rest] = byVersion(env.targets);
  if (!lead) return ['', ''];
  const n = new Set(env.targets.map(runs)).size;
  const parts = [`${lead.name} is on ${runs(lead)}`, ...rest.map((t) => `${t.name} still on ${runs(t)}`)];
  return [`${env.name} runs ${counts[n] ?? n} releases.`, `${parts.join(', ')}.`];
}

export const splitText = (env: EnvState): string => splitParts(env).join(' ').trim();

// promotion: what the environment at i may promote, a candidate in the one
// before last, to the last as its final; null when nothing, or the last
// runs that final already
export function promotion(envs: EnvState[], i: number): { tag: string; final: string; to: string } | null {
  const e = envs[i];
  const last = envs[i + 1];
  const rc = tagPattern.exec(e?.version ?? '')?.[4];
  if (!e || e.role !== 'candidate' || last?.role !== 'last' || e.split || !rc) return null;
  const final = e.version.slice(0, -`-rc.${rc}`.length);
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

// stepLabel: what a job's step did, as the plan list says it: the engine
// names a step after an image, chart, tag, record, or "<env> on <repo>".
// Done, it says what was done; running or failed, what it is about.
export function stepLabel(s: ReleaseStep, names: Record<string, string> = {}): string {
  const done = s.state === 'done';
  const on = /^(\S+) on (.+)$/.exec(s.name);
  if (on) return `Pointing ${on[1]} at it (${names[on[2]!] ?? on[2]})`;
  switch (s.name) {
    case 'tag':
      return done ? `Tag ${s.detail.replace(/^pushed /, '')} pushed` : 'Tag';
    case 'chart':
      return done ? 'Chart packaged' : 'Chart';
    case 'record':
      return done ? 'Release record written' : 'Release record';
  }
  if (/\s/.test(s.name)) return s.name; // no image name has a space: a step this page doesn't know
  if (!done) return `Image ${s.name}`;
  return s.detail.startsWith('reused ') ? `Image ${s.name} ready` : `Image ${s.name} built and pushed`;
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
