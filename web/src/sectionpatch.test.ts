// Run with: node --test src/sectionpatch.test.ts
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { sectionPatch } from './sectionpatch.ts';

const raw = {
  fields: { prio: { options: ['p0'] }, size: { options: ['s'] } },
  release: { checks: 'make', platforms: ['linux/amd64'], follow: '' },
  roles: { pm: 'a', lead: 'b' },
  cycle: { ready: 'ready', criteria: ['it works'] },
};

test('a field is a whole table; a field dropped, null', () => {
  assert.deepEqual(sectionPatch('fields', { set: { prio: { options: ['p0', 'p1'], unit: '', colors: {} } }, remove: ['size', 'ghost'] }, { raw }), {
    'fields.prio': { options: ['p0', 'p1'] },
    'fields.size': null,
  });
});

test('a section sends its keys alone: one emptied is removed, one the form doesn’t show stays', () => {
  assert.deepEqual(sectionPatch('release', { checks: 'make ci', build: '', follow: '' }, { raw }), {
    'release.checks': 'make ci',
    'release.follow': '',
  });
  assert.deepEqual(sectionPatch('release', { checks: '' }, { raw }), { 'release.checks': null });
});

test('roles: the draft is the table', () => {
  assert.deepEqual(sectionPatch('roles', { pm: 'c' }, { raw }), { 'roles.pm': 'c', 'roles.lead': null });
});

test('the cycle: off removes the table, on writes its keys', () => {
  assert.deepEqual(sectionPatch('cycle', { on: false, cycle: {} }, { raw }), { cycle: null });
  assert.deepEqual(sectionPatch('cycle', { on: true, cycle: { ready: 'go', plans: 'docs' } }, { raw }), { 'cycle.ready': 'go', 'cycle.plans': 'docs' });
  assert.deepEqual(sectionPatch('cycle', { on: true, cycle: { ready: '' } }, { raw: {} }), { cycle: {} });
});

test('a field’s place on a forge: its mapping, and the table of a forge found in the remotes', () => {
  const forges = [{ name: 'github', kind: 'github', project: 'o/r', host: 'github.com' }];
  assert.deepEqual(sectionPatch('fields', { set: {}, remove: [], forges: { github: { set: { prio: { to: 'label', values: null } }, remove: ['est'] } } }, { raw: {}, forges }), {
    'forges.github.kind': 'github',
    'forges.github.project': 'o/r',
    'forges.github.fields.prio': { to: 'label' },
  });
  const raw = { forges: { github: { kind: 'github', fields: { est: { to: 'project' } } } } };
  assert.deepEqual(sectionPatch('fields', { set: {}, remove: [], forges: { github: { set: {}, remove: ['est'] } } }, { raw, forges }), { 'forges.github.fields.est': null });
});
