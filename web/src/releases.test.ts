import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { EnvState, ReleaseJob, ReleaseRow, TargetState } from './api/board.ts';
import { behindText, groupReleases, jobTitle, promotion, splitText, stepState } from './releases.ts';

function row(name: string): ReleaseRow {
  const [, major, minor, patch, rc] = /^v(\d+)\.(\d+)\.(\d+)(?:-rc\.(\d+))?$/.exec(name)!.map(Number);
  return {
    tag: { name, commit: 'c-' + name, message: '', date: '2026-10-01T10:00:00Z', version: { major: major!, minor: minor!, patch: patch!, rc: rc || 0 } },
    names: [name],
    record: null,
    runsIn: [],
  };
}

function target(name: string, version: string, commit = 'c-' + version): TargetState {
  return { name, repo: name, path: 'overlays/prod', deployed: { commit, version, images: {}, chart: '' }, since: null };
}

function env(e: Partial<EnvState>): EnvState {
  return { name: 'prod', role: 'last', targets: [], commit: '', version: '', split: false, adopted: true, behind: [], ...e };
}

test('groupReleases puts candidates under their version', () => {
  const g = groupReleases([row('v1.4.20-rc.2'), row('v1.4.20-rc.1'), row('v1.4.19')]);
  assert.deepEqual(g.map((x) => x.version), ['1.4.20', '1.4.19']);
  assert.equal(g[0]!.final, undefined);
  assert.deepEqual(g[0]!.candidates.map((c) => c.tag.name), ['v1.4.20-rc.2', 'v1.4.20-rc.1']);
  assert.equal(g[1]!.final?.tag.name, 'v1.4.19');
  assert.deepEqual(g[1]!.candidates, []);
});

test('behindText', () => {
  assert.equal(behindText(env({ name: 'dev', behind: ['a', 'b'] })), 'Runs 2 merges behind the dev branch');
  assert.equal(behindText(env({ name: 'dev', behind: ['a'] })), 'Runs 1 merge behind the dev branch');
});

test('splitText names each target', () => {
  const split = env({ split: true, targets: [target('enack8s', 'v1.4.20'), target('openshift', 'v1.4.19')] });
  assert.equal(splitText(split), 'prod runs two releases. enack8s is on v1.4.20, openshift still on v1.4.19.');
  // the target whose overlay changed last leads, whatever the config's order
  const moved = (t: TargetState, date: string): TargetState => ({ ...t, since: { commit: 'x', author: 'a', version: t.deployed.version, date } });
  const reversed = env({ split: true, targets: [moved(target('openshift', 'v1.4.19'), '2026-09-28T10:00:00Z'), moved(target('enack8s', 'v1.4.20'), '2026-10-17T16:02:00+02:00')] });
  assert.equal(splitText(reversed), 'prod runs two releases. enack8s is on v1.4.20, openshift still on v1.4.19.');
  // an overlay not adopted has no version: its commit, else none
  const unadopted = env({ split: true, targets: [target('enack8s', 'v1.4.20'), target('openshift', '', '')] });
  assert.equal(splitText(unadopted), 'prod runs two releases. enack8s is on v1.4.20, openshift still on no known commit.');
});

test('stepState', () => {
  const step = (name: string, state: 'running' | 'done' | 'failed') => ({ name, detail: '', state });
  const running: ReleaseJob = { kind: 'deploy', subject: 'dev', state: 'running', error: '', code: '', steps: [step('frontend', 'done'), step('backend', 'running')], started: '', ended: null };
  assert.equal(stepState(running, 0), 'done');
  assert.equal(stepState(running, 1), 'now');
  assert.equal(stepState(running, 2), 'todo');
  assert.equal(stepState(undefined, 0), 'todo');
  const failed: ReleaseJob = { ...running, state: 'failed', error: 'boom', steps: [step('frontend', 'done'), step('backend', 'failed')] };
  assert.equal(stepState(failed, 0), 'done');
  assert.equal(stepState(failed, 1), 'failed');
  // failed outside a step: its last step still carries the failure
  const after: ReleaseJob = { ...running, state: 'failed', error: 'push refused', steps: [step('frontend', 'done'), step('backend', 'done')] };
  assert.equal(stepState(after, 1), 'failed');
});

test('promotion: a candidate in the environment before last', () => {
  const envs = [env({ name: 'dev', role: 'follow', version: '1.4.20-dev.7.g8c41d2e' }), env({ name: 'stage', role: 'candidate', version: 'v1.4.20-rc.2' }), env({ name: 'prod', role: 'last', version: 'v1.4.19' })];
  assert.deepEqual(promotion(envs, 1), { tag: 'v1.4.20-rc.2', final: 'v1.4.20', to: 'prod' });
  assert.equal(promotion(envs, 0), null);
  assert.equal(promotion(envs, 2), null);
  assert.equal(promotion([envs[0]!, { ...envs[1]!, version: 'v1.4.20' }, envs[2]!], 1), null);
  assert.equal(promotion([envs[0]!, { ...envs[1]!, split: true }, envs[2]!], 1), null);
  assert.equal(promotion([envs[0]!, envs[1]!, { ...envs[2]!, version: 'v1.4.20' }], 1), null);
});

test('jobTitle', () => {
  const job: ReleaseJob = { kind: 'rollback', subject: 'prod to v1.4.19', state: 'running', error: '', code: '', steps: [], started: '', ended: null };
  assert.equal(jobTitle(job), 'Rolling back prod to v1.4.19');
  assert.equal(jobTitle({ ...job, state: 'done' }), 'Rolled back prod to v1.4.19');
  assert.equal(jobTitle({ ...job, kind: 'cut', subject: '', state: 'failed' }), 'The cut of a release stopped');
});
