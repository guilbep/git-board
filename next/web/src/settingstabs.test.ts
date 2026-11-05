import { test } from 'node:test';
import assert from 'node:assert/strict';
import { settingsGroups, settingsTab, settingsTabs, syncSummary, visibleTabs } from './settingstabs.ts';

test('settingsTab takes the address, the Sync without one or for an unknown link', () => {
  assert.equal(settingsTab('sync'), 'sync');
  assert.equal(settingsTab('fields'), 'fields');
  assert.equal(settingsTab('advanced'), 'advanced');
  assert.equal(settingsTab('numbers'), 'numbers');
  assert.equal(settingsTab('labels'), 'labels');
  assert.equal(settingsTab(''), 'sync');
  assert.equal(settingsTab(undefined), 'sync');
  assert.equal(settingsTab('bridges'), 'sync');
  assert.equal(settingsTab(['sync']), 'sync');
  // the old tabs' links: General held who you write as; the three ways are
  // one Sync; the Project file is a tab per section, the fields first
  assert.equal(settingsTab('overview'), 'sync');
  assert.equal(settingsTab('general'), 'you');
  assert.equal(settingsTab('remotes'), 'sync');
  assert.equal(settingsTab('forges'), 'sync');
  assert.equal(settingsTab('github-project'), 'sync');
  assert.equal(settingsTab('project-file'), 'fields');
  assert.equal(settingsTab('forge-fields'), 'fields'); // edited on each field now
});

test('the tabs, in the menu order: the project file with its Sync, then what is kept elsewhere', () => {
  assert.deepEqual(
    settingsTabs.map((t) => t.id),
    ['sync', 'fields', 'labels', 'views', 'sprints', 'roles', 'branches', 'cycle', 'release', 'repos', 'attachments', 'experimental', 'advanced', 'datastore', 'numbers', 'you'],
  );
});

test('the sidebar groups by where a tab is kept: the project file, the Sync in it, then elsewhere', () => {
  const groups = settingsGroups();
  assert.deepEqual(
    groups.map((g) => g.group),
    ['The project file', 'Kept elsewhere'],
  );
  const byId = (i: number) => groups[i]?.tabs.map((t) => t.id) ?? [];
  assert.deepEqual(byId(0), ['sync', 'fields', 'labels', 'views', 'sprints', 'roles', 'branches', 'cycle', 'release', 'repos', 'attachments', 'experimental', 'advanced']);
  assert.deepEqual(byId(1), ['datastore', 'numbers', 'you']);
});

test('the Sync says in one line where it stands', () => {
  assert.equal(syncSummary({ remotes: [], forges: [] }), 'No remote: the issues are only on this computer, no forge with sync on.');
  assert.equal(syncSummary({ remotes: ['upstream', 'origin'], forges: [{ sync: true }, { sync: false }] }), 'origin, 1 forge with sync on.');
  assert.equal(syncSummary({ remotes: ['upstream'], forges: [{ sync: true }, { sync: true }] }), 'upstream, 2 forges with sync on.');
});

test('an experimental feature’s tab shows while it is on; its link is the Sync while off', () => {
  const none = () => false;
  const ids = visibleTabs(none).map((t) => t.id);
  assert.ok(!ids.includes('cycle'));
  assert.ok(ids.includes('experimental'));
  assert.equal(settingsTab('cycle', none), 'sync');
  assert.equal(settingsTab('worktrees', none), 'sync'); // dropped: wtx's own binary
  assert.equal(settingsTab('experimental', none), 'experimental');
  const cycleOnly = (f: string) => f === 'cycle';
  assert.equal(settingsTab('cycle', cycleOnly), 'cycle');
  assert.deepEqual(settingsGroups(cycleOnly)[1]?.tabs.map((t) => t.id), ['datastore', 'numbers', 'you']);
  assert.ok(settingsGroups(cycleOnly)[0]?.tabs.some((t) => t.id === 'cycle'));
});
