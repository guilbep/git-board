// The words of the Releases page (pages/ReleasesPage.vue,
// components/ReleaseJob.vue, api/board.ts Releases): environment marks,
// releases grouped by version, the alerts' sentences, a job's steps; and
// of a release's page and its dialogs (ReleasePage.vue, CutReleaseDialog,
// RollbackDialog): the cut's tag and label, the checks, rollback targets.
// Pure, no "@/" imports: node runs releases.test.ts on it.
import type { Draft, EnvState, IssueEnvironment, ReleaseJob, ReleaseRow, ReleaseStep, ReleaseVersion, TargetState } from './api/board.ts';

// a release tag: v1.4.20, v1.4.20-rc.2 (release.ParseTag)
export const tagPattern = /^v(\d+)\.(\d+)\.(\d+)(?:-rc\.(\d+))?$/;

// prod black, stage canard, dev outlined; off: not there
export type EnvMark = 'prod' | 'stage' | 'dev' | 'off';

export function envMark(role: EnvState['role'], here: boolean): EnvMark {
  if (!here) return 'off';
  return role === 'last' ? 'prod' : role === 'candidate' ? 'stage' : 'dev';
}

// envLine: an issue's Development line, one piece per environment, in the
// environments' order: "in dev", "in stage · v1.4.20-rc.2", "not in prod".
// A version shows only when it is a release tag (the followed environment's
// bare dev version doesn't). The marks follow the roles by position, as the
// config has them: the first environment is followed (dev), the last is
// prod, the ones between are candidates (stage).
export function envLine(envs: IssueEnvironment[]): { text: string; mark: EnvMark }[] {
  const n = envs.length;
  return envs.map((e, i) => {
    const role: EnvState['role'] = i === n - 1 ? 'last' : i === 0 ? 'follow' : 'candidate';
    const version = e.in && tagPattern.test(e.version) ? ` · ${e.version}` : '';
    return { text: `${e.in ? 'in' : 'not in'} ${e.name}${version}`, mark: envMark(role, e.in) };
  });
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
// environment; trunk: the trunk's name (Releases.trunk), else the
// environment's, which is named after it (dev)
export function behindText(env: EnvState, trunk = env.name): string {
  const n = env.behind.length;
  return `Runs ${n} merge${n === 1 ? '' : 's'} behind the ${trunk} branch`;
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

// cutLabel: the cut dialog's button, "Cut v1.4.20-rc.3 and deploy to stage"
export function cutLabel(d: Draft, deploy: boolean, env: string): string {
  return `Cut ${d.tag}${deploy ? ` and deploy to ${env}` : ''}`;
}

// checkIcon: a check's mark: held, failed, or undefined: left to the
// action itself, which checks it then
export function checkIcon(ok: boolean | undefined): { name: 'check' | 'error' | 'info'; cls: string } {
  if (ok === undefined) return { name: 'info', cls: '' };
  return ok ? { name: 'check', cls: 'gb-state-ok' } : { name: 'error', cls: 'gb-state-warn' };
}

// nextTag: what a cut of version (1.4.20) pushes, as the engine's tagFor:
// the final, or one candidate past the highest taken; "" for a version
// that isn't M.m.p
export function nextTag(version: string, final: boolean, rows: ReleaseRow[]): string {
  const m = /^v?(\d+\.\d+\.\d+)$/.exec(version.trim());
  if (!m) return '';
  if (final) return `v${m[1]}`;
  const rc = rows.reduce((n, r) => (finalOf(r.tag.version) === m[1] ? Math.max(n, r.tag.version.rc) : n), 0);
  return `v${m[1]}-rc.${rc + 1}`;
}

// finalOf: a version's final, as said: 1.4.20
export const finalOf = (v: ReleaseVersion): string => `${v.major}.${v.minor}.${v.patch}`;

// takenOf: what of version is cut already: its candidates (rc.1, rc.2),
// which a candidate's number goes past; for a final, the final itself,
// which can't be cut again
export function takenOf(version: string, final: boolean, rows: ReleaseRow[]): string[] {
  const v = version.trim().replace(/^v/, '');
  const of = rows.filter((r) => finalOf(r.tag.version) === v);
  if (final) return of.filter((r) => !r.tag.version.rc).map((r) => r.tag.name);
  return of.filter((r) => r.tag.version.rc).map((r) => `rc.${r.tag.version.rc}`).reverse();
}

const list = new Intl.ListFormat('en', { type: 'conjunction' });

// proposal: why the draft proposes its version
function proposal(d: Draft): string {
  const n = d.issues.length;
  const p = d.previous?.version;
  if (!p) return `Proposed: ${finalOf(d.proposed)}, the first release.`;
  if (d.proposed.major !== p.major) return 'Proposed: a major, because an issue is labelled breaking.';
  if (d.proposed.minor !== p.minor) return `Proposed: a minor, because ${n === 1 ? 'its issue' : 'one of the issues'} is a feat.`;
  if (!n) return 'Proposed: a patch, since no merged branch names an issue.';
  return `Proposed: a patch, because ${n === 1 ? 'its one issue isn’t' : `none of the ${n} issues is`} a feat.`;
}

// cutHint: the cut dialog's hint, around its bold tag: "Proposed: a patch,
// because none of the 5 issues is a feat. The tag will be", "v1.4.20-rc.3",
// ": rc.1 and rc.2 are taken."; tag "": the version typed isn't M.m.p
export function cutHint(d: Draft, tag: string, taken: string[], final: boolean): [string, string, string] {
  if (!tag) return [`${proposal(d)} A version is three numbers, like ${finalOf(d.proposed)}.`, '', ''];
  if (final && taken.length) return [`${proposal(d)} The tag`, tag, ' is cut already: choose another version.'];
  return [`${proposal(d)} The tag will be`, tag, taken.length ? `: ${list.format(taken)} ${taken.length === 1 ? 'is' : 'are'} taken.` : '.'];
}

// older: v before w, as SemVer has it: a final after its candidates, equal
// versions not older
export function older(v: ReleaseVersion, w: ReleaseVersion): boolean {
  const rank = (x: ReleaseVersion) => [x.major, x.minor, x.patch, x.rc ? 0 : 1, x.rc];
  const a = rank(v);
  const b = rank(w);
  const i = a.findIndex((x, j) => x !== b[j]);
  return i >= 0 && a[i]! < b[i]!;
}

// olderFinals: what env can roll back to: the finals before what it runs
// (the target ahead, in a split), yanked ones aside, newest first. Running
// what isn't a release tag, every final.
export function olderFinals(env: EnvState, rows: ReleaseRow[]): ReleaseRow[] {
  const now = rows.find((r) => r.tag.name === byVersion(env.targets)[0]?.deployed.version)?.tag.version;
  return rows.filter((r) => !r.tag.version.rc && !r.record?.yanked && (!now || older(r.tag.version, now)));
}

// bump: what v is to the final before it: a major, minor or patch release;
// "" without one
export function bump(prev: ReleaseVersion | null | undefined, v: ReleaseVersion): 'major' | 'minor' | 'patch' | '' {
  if (!prev) return '';
  return v.major !== prev.major ? 'major' : v.minor !== prev.minor ? 'minor' : 'patch';
}

// registryName: a registry as said: ghcr.io/epfl-enac/co2-calculator →
// ghcr. ponytail: its host without the top-level domain; a host without a
// dot (localhost:5000) stays whole
export const registryName = (r: string): string => (r.split('/')[0] ?? r).replace(/\.[^.:]+$/, '');

// repoName: a repository's URL's last part, without .git:
// git@github.com:EPFL-ENAC/enack8s-app-config.git → enack8s-app-config
export const repoName = (url: string): string => url.replace(/(\.git)?\/*$/, '').split(/[/:]/).at(-1) ?? '';

// releaseBadge: "Candidate · in stage", "Release · in prod", "Yanked"
export function releaseBadge(r: ReleaseRow): { text: string; cls: string } {
  if (r.record?.yanked) return { text: 'Yanked', cls: 'gb-rel-badge-yanked' };
  const where = r.runsIn.length ? ` · in ${list.format(r.runsIn)}` : '';
  return r.tag.version.rc ? { text: `Candidate${where}`, cls: 'epfl-badge-tertiary' } : { text: `Release${where}`, cls: 'epfl-badge-dark' };
}

// the release pages' small words
export const sha7 = (c: string): string => c.slice(0, 7);
export const plural = (n: number, w: string): string => `${n} ${w}${n === 1 ? '' : 's'}`;
export const releasePath = (tag: string): string => `/releases/${encodeURIComponent(tag)}`;
// an environment mark's classes; dev is the plain outlined one
export const markClass = (m: EnvMark): string[] => ['gb-envmark', m === 'dev' ? '' : `gb-envmark-${m}`];
// repoNames: each config repo's target name (enack8s), for the job steps
// that name the repo
export const repoNames = (envs: EnvState[]): Record<string, string> => Object.fromEntries(envs.flatMap((e) => e.targets.map((t) => [t.repo, t.name])));
