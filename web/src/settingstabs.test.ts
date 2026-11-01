import { test } from 'node:test';
import assert from 'node:assert/strict';
import { settingsGroups, settingsTab, settingsTabs, syncSummary } from './settingstabs.ts';

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
  assert.equal(settingsTab('forge-fields'), 'forge-fields');
});

test('the tabs, in the menu order: the project file with its Sync, then what is kept elsewhere', () => {
  assert.deepEqual(
    settingsTabs.map((t) => t.id),
    ['sync', 'forge-fields', 'fields', 'labels', 'views', 'sprints', 'roles', 'branches', 'cycle', 'release', 'repos', 'attachments', 'advanced', 'numbers', 'worktrees', 'you'],
  );
});

test('the sidebar groups by where a tab is kept: the project file, the Sync in it, then elsewhere', () => {
  const groups = settingsGroups();
  assert.deepEqual(
    groups.map((g) => g.group),
    ['The project file', 'Kept elsewhere'],
  );
  const byId = (i: number) => groups[i]?.tabs.map((t) => t.id) ?? [];
  assert.deepEqual(byId(0), ['sync', 'forge-fields', 'fields', 'labels', 'views', 'sprints', 'roles', 'branches', 'cycle', 'release', 'repos', 'attachments', 'advanced']);
  assert.deepEqual(byId(1), ['numbers', 'worktrees', 'you']);
});

test('the Sync says in one line where it stands', () => {
  const none = { project: '', write: false, last: null };
  assert.equal(syncSummary({ remote: null, forges: [], project: none }), 'No origin: the issues are only on this computer, no forge with sync on.');
  assert.equal(syncSummary({ remote: { name: 'origin' }, forges: [{ sync: true }, { sync: false }], project: none }), 'origin, 1 forge with sync on.');
  assert.equal(syncSummary({ remote: { name: 'origin' }, forges: [{ sync: true }, { sync: true }], project: { ...none, project: 'EPFL-ENAC/40' } }), 'origin, 2 forges with sync on, the GitHub Project EPFL-ENAC/40.');
});
