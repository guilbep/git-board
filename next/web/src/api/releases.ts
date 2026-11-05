// Releases (docs/rewrite/api.md, routes 43 to 48; next/tools/ship): what
// runs where, the releases, a release's page and its record's writes, and
// the jobs that move releases (cut, promote, deploy, rollback), one at a
// time in the background, each step a `release` event. A write while a
// job runs answers 409 busy.
import { enc, get, send } from './http.ts';

export interface ReleaseVersion {
  major: number;
  minor: number;
  patch: number;
  rc: number; // 0: a final
}

export interface ReleaseTag {
  name: string; // v1.4.20, v1.4.20-rc.2
  commit: string;
  message: string;
  version: ReleaseVersion;
  date: string;
}

// a commit that touched an overlay
export interface OverlayChange {
  commit: string;
  author: string;
  version: string;
  date: string;
}

export interface TargetState {
  name: string; // enack8s; with the registry's host when two targets share one
  repo: string;
  path: string; // the overlay's directory
  deployed: { commit: string; version: string; images: Record<string, { tag: string; digest: string }>; chart: string }; // commit "": not adopted
  since: OverlayChange | null; // the overlay's newest commit
}

export interface EnvState {
  name: string;
  role: 'follow' | 'candidate' | 'last' | ''; // "": before the followed one
  targets: TargetState[];
  commit: string; // the targets' when they agree
  version: string; // the targets' when they agree
  split: boolean; // targets run different commits
  adopted: boolean; // every target has git-board/commit
  behind: string[]; // follow only: subjects of the trunk's merges it doesn't run
}

export interface ReleaseRecord {
  id: string;
  commit: string;
  previous: string; // the previous final's tag
  artifacts: { image: string; registry: string; digest: string }[];
  chart: { version: string; digest: string };
  assets: { name: string; sha256: string }[] | null; // the archives published with the tag, binaries only
  notes: string; // markdown
  comments: { author: string; message: string; date: string }[];
  yanked: string; // the reason, "" when not
  author: string;
  created: string;
}

export interface ReleaseRow {
  tag: ReleaseTag;
  names: string[]; // every release tag on its commit, newest first
  record: ReleaseRecord | null; // null until reconciled
  runsIn: string[]; // environments running its commit
  beforeGitBoard: boolean; // its tag wasn't cut by git-board (no " (git-board)" in its subject)
}

export interface ReleaseStep {
  name: string;
  detail: string;
  state: 'running' | 'done' | 'failed';
}

export interface ReleaseJob {
  kind: 'deploy' | 'cut' | 'promote' | 'rollback';
  subject: string; // dev, v1.0.1-rc.1 to stage, prod to v1.0.0, the rc promoted, a cut's tag
  state: 'running' | 'done' | 'failed';
  error?: string;
  code?: 'taken' | 'older' | 'not-adopted'; // a failed job's
  tag?: string; // the tag a cut or promote pushed or tried, once known
  steps: ReleaseStep[]; // each once, as last reported
  started_at: string;
  ended_at?: string;
}

export interface Releases {
  enabled: boolean;
  remote: string; // the authority remote, [release] remote (origin)
  trunk: string; // the trunk branch (dev)
  environments: EnvState[];
  binaries: string[]; // [release] binaries: a cut builds and publishes their archives
  releases: ReleaseRow[]; // newest first
  job: ReleaseJob | null; // running, or the last one
  proposals: Proposal[]; // the commits the overlays not adopted run, proposed
}

export interface DraftIssue {
  number: string; // CO2-3075, #2518
  title: string;
  kind: string;
  merge: string;
}

export interface Draft {
  from: string; // the followed environment's commit, else the trunk's head
  fromVersion: string;
  undeployed: string[]; // trunk merges after from
  previous: ReleaseTag | null;
  proposed: ReleaseVersion;
  tag: string; // what a cut would push
  issues: DraftIssue[];
  notes: string;
  checks: { ok: boolean; text: string; detail: string }[];
}

export interface ReleaseDetail {
  row: ReleaseRow;
  issues: DraftIssue[];
  candidates: ReleaseRow[]; // the rc rows of its final, but itself
  history: Record<string, (OverlayChange & { target: string })[]>; // environment → its overlays' changes naming this version
}

// the commit an overlay is thought to run, for adopt
export interface Proposal {
  env: string;
  repo: string;
  path: string;
  runs: string;
  commit: string; // "": unknown, to type in
  from: 'tag' | 'label' | '';
}

export interface IssueEnvironment {
  name: string;
  version: string;
  in: boolean;
}

const tagPath = (tag: string) => `/api/releases/${enc(tag)}`;
const envPath = (env: string) => `/api/environments/${enc(env)}/deploy`;

const releaseRow = (r: ReleaseRow): ReleaseRow => ({
  ...r,
  names: r.names ?? [],
  runsIn: r.runsIn ?? [],
  record: r.record && { ...r.record, artifacts: r.record.artifacts ?? [], comments: r.record.comments ?? [] },
});
const envState = (e: EnvState): EnvState => ({
  ...e,
  behind: e.behind ?? [],
  targets: (e.targets ?? []).map((t) => ({ ...t, deployed: { ...t.deployed, images: t.deployed.images ?? {} } })),
});
const releaseJob = (j: ReleaseJob | null): ReleaseJob | null => j && { ...j, steps: j.steps ?? [] };

// local: what this clone has, at once, without fetching origin and the overlays
export async function getReleases(local = false): Promise<Releases> {
  const r = await get<Partial<Releases>>('/api/releases', { query: { local: local || undefined } });
  return {
    enabled: !!r.enabled,
    remote: r.remote ?? '',
    trunk: r.trunk ?? '',
    environments: (r.environments ?? []).map(envState),
    binaries: r.binaries ?? [],
    releases: (r.releases ?? []).map(releaseRow),
    job: releaseJob(r.job ?? null),
    proposals: r.proposals ?? [],
  };
}

// the draft of the next cut: the tag next
export async function getDraft(final = false): Promise<Draft> {
  const d = await get<Draft>(tagPath('next'), { query: { final: final || undefined } });
  return { ...d, undeployed: d.undeployed ?? [], issues: d.issues ?? [], checks: d.checks ?? [] };
}

export async function getRelease(tag: string): Promise<ReleaseDetail> {
  const d = await get<ReleaseDetail>(tagPath(tag));
  const history = Object.fromEntries(Object.entries(d.history ?? {}).map(([env, h]) => [env, h ?? []]));
  return { row: releaseRow(d.row), issues: d.issues ?? [], candidates: (d.candidates ?? []).map(releaseRow), history };
}

// version: the final form (1.4.20), "" the draft's; the rc number is the
// server's. commit, notes: "" the draft's
export interface CutRequest {
  version?: string;
  final?: boolean;
  hotfix?: boolean;
  commit?: string;
  notes?: string;
  deploy?: boolean;
}

export async function cutRelease(c: CutRequest): Promise<ReleaseJob> {
  return releaseJob(await send<ReleaseJob>('POST', '/api/releases', { body: c }))!;
}

export async function promoteRelease(tag: string): Promise<ReleaseJob> {
  return releaseJob(await send<ReleaseJob>('PATCH', tagPath(tag), { body: { state: 'promoted' } }))!;
}

// tag "": the trunk's head, to the followed environment only
export async function deployEnvironment(env: string, tag = ''): Promise<ReleaseJob> {
  return releaseJob(await send<ReleaseJob>('POST', envPath(env), { body: tag ? { tag } : {} }))!;
}

// a rollback is a deploy of an older tag, with a reason
export async function rollbackEnvironment(env: string, tag: string, reason: string): Promise<ReleaseJob> {
  return releaseJob(await send<ReleaseJob>('POST', envPath(env), { body: { tag, reason } }))!;
}


// the overlays adopt the commits they run: a PATCH of a release, any
export function adoptOverlays(proposals: Proposal[]): Promise<object> {
  return send('PATCH', tagPath(proposals[0]?.runs || 'next'), { body: { adopt: proposals } });
}

export function setReleaseNotes(tag: string, notes: string): Promise<object> {
  return send('PATCH', tagPath(tag), { body: { notes } });
}

export function commentRelease(tag: string, body: string): Promise<object> {
  return send('POST', `${tagPath(tag)}/comments`, { body: { body } });
}

export function yankRelease(tag: string, reason: string): Promise<object> {
  return send('PATCH', tagPath(tag), { body: { state: 'yanked', reason } });
}
