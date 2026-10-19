import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { Draft, EnvState, ReleaseJob, ReleaseRow, ReleaseStep, TargetState } from './api/board.ts';
import { behindText, bump, checkIcon, cutHint, cutLabel, groupReleases, jobTitle, markClass, nextTag, older, olderFinals, plural, promotion, registryName, releaseBadge, releasePath, repoName, repoNames, sha7, splitParts, splitText, stepLabel, stepState, takenOf } from './releases.ts';

function row(name: string): ReleaseRow {
  const [, major, minor, patch, rc] = /^v(\d+)\.(\d+)\.(\d+)(?:-rc\.(\d+))?$/.exec(name)!.map(Number);
  return {
    tag: { name, commit: 'c-' + name, message: '', date: '2026-10-01T10:00:00Z', version: { major: major!, minor: minor!, patch: patch!, rc: rc || 0 } },
    names: [name],
    record: null,
    runsIn: [],
    beforeGitBoard: false,
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
  // the trunk as the server names it, when it isn't the environment's name
  assert.equal(behindText(env({ name: 'dev', behind: ['a'] }), 'main'), 'Runs 1 merge behind the main branch');
});

test('splitText names each target', () => {
  const split = env({ split: true, targets: [target('enack8s', 'v1.4.20'), target('openshift', 'v1.4.19')] });
  assert.equal(splitText(split), 'prod runs two releases. enack8s is on v1.4.20, openshift still on v1.4.19.');
  // the highest version leads, whatever the config's order: deploys only
  // move forward, so the lagging target is behind even when edited last
  const moved = (t: TargetState, date: string): TargetState => ({ ...t, since: { commit: 'x', author: 'a', version: t.deployed.version, date } });
  const reversed = env({ split: true, targets: [moved(target('openshift', 'v1.4.19'), '2026-10-17T16:02:00+02:00'), moved(target('enack8s', 'v1.4.20'), '2026-09-28T10:00:00Z')] });
  assert.equal(splitText(reversed), 'prod runs two releases. enack8s is on v1.4.20, openshift still on v1.4.19.');
  assert.deepEqual(splitParts(reversed), ['prod runs two releases.', 'enack8s is on v1.4.20, openshift still on v1.4.19.']);
  // a final is past its candidates
  const final = env({ split: true, targets: [target('enack8s', 'v1.4.20-rc.2'), target('openshift', 'v1.4.20')] });
  assert.equal(splitText(final), 'prod runs two releases. openshift is on v1.4.20, enack8s still on v1.4.20-rc.2.');
  // an overlay not adopted has no version: its commit, else none
  const unadopted = env({ split: true, targets: [target('enack8s', 'v1.4.20'), target('openshift', '', '')] });
  assert.equal(splitText(unadopted), 'prod runs two releases. enack8s is on v1.4.20, openshift still on no known commit.');
});

test('stepState', () => {
  const step = (name: string, state: 'running' | 'done' | 'failed') => ({ name, detail: '', state });
  const running: ReleaseJob = { kind: 'deploy', subject: 'dev', state: 'running', error: '', code: '', tag: '', steps: [step('frontend', 'done'), step('backend', 'running')], started: '', ended: null };
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
  const job: ReleaseJob = { kind: 'rollback', subject: 'prod to v1.4.19', state: 'running', error: '', code: '', tag: '', steps: [], started: '', ended: null };
  assert.equal(jobTitle(job), 'Rolling back prod to v1.4.19');
  assert.equal(jobTitle({ ...job, state: 'done' }), 'Rolled back prod to v1.4.19');
  assert.equal(jobTitle({ ...job, kind: 'cut', subject: '', state: 'failed' }), 'The cut of a release stopped');
});

test('stepLabel says what each step did', () => {
  const s = (name: string, detail = '', state: ReleaseStep['state'] = 'done'): ReleaseStep => ({ name, detail, state });
  const names = { 'git@github.com:EPFL-ENAC/enack8s-app-config.git': 'enack8s' };
  assert.equal(stepLabel(s('frontend', 'built sha256:aa'), names), 'Image frontend built and pushed');
  assert.equal(stepLabel(s('backend', 'reused sha256:bb'), names), 'Image backend ready');
  assert.equal(stepLabel(s('backend', '', 'running'), names), 'Image backend');
  assert.equal(stepLabel(s('chart', 'pushed v1.4.20 sha256:cc'), names), 'Chart packaged');
  assert.equal(stepLabel(s('chart', 'helm: no Chart.yaml', 'failed'), names), 'Chart');
  assert.equal(stepLabel(s('tag', 'pushed v1.4.20-rc.3'), names), 'Tag v1.4.20-rc.3 pushed');
  assert.equal(stepLabel(s('record', 'created abc1234'), names), 'Release record written');
  assert.equal(stepLabel(s('dev on git@github.com:EPFL-ENAC/enack8s-app-config.git', 'pushed 1a2b3c4'), names), 'Pointing dev at it (enack8s)');
  assert.equal(stepLabel(s('dev on /tmp/other.git', '', 'running'), names), 'Pointing dev at it (/tmp/other.git)');
  assert.equal(stepLabel(s('a step this page doesn’t know'), names), 'a step this page doesn’t know');
});

test('cutLabel says the tag, and where it goes when it deploys', () => {
  const d = { tag: 'v1.4.20-rc.3' } as Draft;
  assert.equal(cutLabel(d, true, 'stage'), 'Cut v1.4.20-rc.3 and deploy to stage');
  assert.equal(cutLabel(d, false, 'stage'), 'Cut v1.4.20-rc.3');
});

test('checkIcon: held, failed, or left to the action', () => {
  assert.deepEqual(checkIcon(true), { name: 'check', cls: 'gb-state-ok' });
  assert.deepEqual(checkIcon(false), { name: 'error', cls: 'gb-state-warn' });
  assert.deepEqual(checkIcon(undefined), { name: 'info', cls: '' });
});

test('nextTag: the final, or the next candidate past the taken ones', () => {
  const rows = [row('v1.4.20-rc.2'), row('v1.4.20-rc.1'), row('v1.4.19'), row('v1.4.19-rc.4')];
  assert.equal(nextTag('1.4.20', false, rows), 'v1.4.20-rc.3');
  assert.equal(nextTag('v1.4.20', true, rows), 'v1.4.20');
  assert.equal(nextTag('1.5.0', false, rows), 'v1.5.0-rc.1');
  assert.equal(nextTag('1.4', false, rows), '');
  assert.equal(nextTag('1.4.20-rc.1', false, rows), '');
});

test('takenOf: what of a version is cut already', () => {
  const rows = [row('v1.4.20-rc.2'), row('v1.4.20-rc.1'), row('v1.4.19'), row('v1.4.19-rc.1')];
  assert.deepEqual(takenOf('1.4.20', false, rows), ['rc.1', 'rc.2']);
  assert.deepEqual(takenOf('1.4.21', false, rows), []);
  // a final taken is the final itself
  assert.deepEqual(takenOf('1.4.19', true, rows), ['v1.4.19']);
  assert.deepEqual(takenOf('v1.4.20', true, rows), []);
});

test('cutHint: the proposal, then the tag and what is taken', () => {
  const v = (s: string) => row(s).tag;
  const issue = (kind: string) => ({ number: '#1', title: 't', kind, merge: 'm' });
  const d = { previous: v('v1.4.19'), proposed: { major: 1, minor: 4, patch: 20, rc: 0 }, issues: [1, 2, 3, 4, 5].map(() => issue('fix')) } as Draft;
  assert.deepEqual(cutHint(d, 'v1.4.20-rc.3', ['rc.1', 'rc.2'], false), ['Proposed: a patch, because none of the 5 issues is a feat. The tag will be', 'v1.4.20-rc.3', ': rc.1 and rc.2 are taken.']);
  assert.deepEqual(cutHint({ ...d, issues: [issue('fix')] }, 'v1.4.20-rc.1', [], false), ['Proposed: a patch, because its one issue isn’t a feat. The tag will be', 'v1.4.20-rc.1', '.']);
  assert.equal(cutHint({ ...d, issues: [] }, 'v1.4.20', [], true)[0], 'Proposed: a patch, since no merged branch names an issue. The tag will be');
  assert.equal(cutHint({ ...d, proposed: { major: 1, minor: 5, patch: 0, rc: 0 } }, 'v1.5.0-rc.1', [], false)[0], 'Proposed: a minor, because one of the issues is a feat. The tag will be');
  assert.equal(cutHint({ ...d, proposed: { major: 2, minor: 0, patch: 0, rc: 0 } }, 'v2.0.0-rc.1', ['rc.1'], false)[2], ': rc.1 is taken.');
  assert.equal(cutHint({ ...d, proposed: { major: 2, minor: 0, patch: 0, rc: 0 } }, 'v2.0.0-rc.1', [], false)[0], 'Proposed: a major, because an issue is labelled breaking. The tag will be');
  assert.equal(cutHint({ ...d, previous: null, proposed: { major: 0, minor: 1, patch: 0, rc: 0 } }, 'v0.1.0', [], true)[0], 'Proposed: 0.1.0, the first release. The tag will be');
  // a final cut already, a version that isn't one
  assert.deepEqual(cutHint(d, 'v1.4.19', ['v1.4.19'], true), ['Proposed: a patch, because none of the 5 issues is a feat. The tag', 'v1.4.19', ' is cut already: choose another version.']);
  assert.deepEqual(cutHint(d, '', [], false), ['Proposed: a patch, because none of the 5 issues is a feat. A version is three numbers, like 1.4.20.', '', '']);
});

test('releaseBadge says what it is and where it runs', () => {
  const r = (name: string, runsIn: string[], yanked = '') => ({ ...row(name), runsIn, record: yanked ? ({ yanked } as ReleaseRow['record']) : null });
  assert.deepEqual(releaseBadge(r('v1.4.20-rc.2', ['stage'])), { text: 'Candidate · in stage', cls: 'epfl-badge-tertiary' });
  assert.deepEqual(releaseBadge(r('v1.4.20', ['stage', 'prod'])), { text: 'Release · in stage and prod', cls: 'epfl-badge-dark' });
  assert.deepEqual(releaseBadge(r('v1.4.20-rc.1', [])), { text: 'Candidate', cls: 'epfl-badge-tertiary' });
  assert.deepEqual(releaseBadge(r('v1.4.20-rc.1', ['stage'], 'broken')), { text: 'Yanked', cls: 'gb-rel-badge-yanked' });
});

test('the small helpers', () => {
  assert.equal(sha7('8c41d2e0a1b2'), '8c41d2e');
  assert.equal(plural(1, 'issue'), '1 issue');
  assert.equal(plural(4, 'issue'), '4 issues');
  assert.equal(releasePath('v1.4.20-rc.2'), '/releases/v1.4.20-rc.2');
  assert.deepEqual(markClass('dev'), ['gb-envmark', '']);
  assert.deepEqual(markClass('prod'), ['gb-envmark', 'gb-envmark-prod']);
  assert.deepEqual(repoNames([env({ targets: [target('enack8s', 'v1'), target('openshift', 'v1')] })]), { enack8s: 'enack8s', openshift: 'openshift' });
});

test('olderFinals: the finals before what the environment runs, not yanked', () => {
  const yanked = { ...row('v1.4.18'), record: { yanked: 'broken' } as ReleaseRow['record'] };
  const rows = [row('v1.4.21-rc.1'), row('v1.4.20'), row('v1.4.20-rc.2'), row('v1.4.19'), yanked, row('v1.4.17')];
  const prod = env({ version: 'v1.4.20', targets: [target('enack8s', 'v1.4.20')] });
  assert.deepEqual(olderFinals(prod, rows).map((r) => r.tag.name), ['v1.4.19', 'v1.4.17']);
  // split: older than the target ahead
  const split = env({ split: true, version: '', targets: [target('enack8s', 'v1.4.19'), target('openshift', 'v1.4.20')] });
  assert.deepEqual(olderFinals(split, rows).map((r) => r.tag.name), ['v1.4.19', 'v1.4.17']);
  // not a release tag: every final but none
  const dev = env({ version: '1.4.21-dev.3.gabc1234', targets: [target('enack8s', '1.4.21-dev.3.gabc1234')] });
  assert.deepEqual(olderFinals(dev, rows).map((r) => r.tag.name), ['v1.4.20', 'v1.4.19', 'v1.4.17']);
});

test('older: SemVer order, a final after its candidates, equal not older', () => {
  const v = (s: string) => row(s).tag.version;
  assert.equal(older(v('v1.4.19'), v('v1.4.20')), true);
  assert.equal(older(v('v1.4.20-rc.2'), v('v1.4.20')), true);
  assert.equal(older(v('v1.4.20-rc.1'), v('v1.4.20-rc.2')), true);
  assert.equal(older(v('v1.4.20'), v('v1.4.20-rc.2')), false);
  assert.equal(older(v('v1.4.20'), v('v1.4.20')), false);
  assert.equal(older(v('v1.4.20-rc.2'), v('v1.4.20-rc.2')), false);
});

test('bump: what a version is to the final before it', () => {
  const v = (s: string) => row(s).tag.version;
  assert.equal(bump(v('v1.4.19'), v('v1.4.20-rc.2')), 'patch');
  assert.equal(bump(v('v1.4.19'), v('v1.5.0')), 'minor');
  assert.equal(bump(v('v1.4.19'), v('v2.0.0')), 'major');
  assert.equal(bump(undefined, v('v0.1.0')), '');
});

test('registryName and repoName say them short', () => {
  assert.equal(registryName('ghcr.io/epfl-enac/co2-calculator'), 'ghcr');
  assert.equal(registryName('quay.test/app'), 'quay');
  assert.equal(registryName('localhost:5000/app'), 'localhost:5000');
  assert.equal(repoName('git@github.com:EPFL-ENAC/enack8s-app-config.git'), 'enack8s-app-config');
  assert.equal(repoName('https://github.com/EPFL-ENAC/co2-calculator/'), 'co2-calculator');
});
