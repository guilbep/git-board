import { test } from 'node:test';
import assert from 'node:assert/strict';
import { settingsGroups, settingsTab, settingsTabs, syncSummary } from './settingstabs.ts';

test('settingsTab takes the address, the Overview without one or for an unknown link', () => {
  assert.equal(settingsTab('sync'), 'sync');
  assert.equal(settingsTab('fields'), 'fields');
  assert.equal(settingsTab('advanced'), 'advanced');
  assert.equal(settingsTab('numbers'), 'numbers');
  assert.equal(settingsTab('labels'), 'labels');
  assert.equal(settingsTab(''), 'overview');
  assert.equal(settingsTab(undefined), 'overview');
  assert.equal(settingsTab('bridges'), 'overview');
  assert.equal(settingsTab(['sync']), 'overview');
  // the old tabs' links: General held who you write as; the three ways are
  // one Sync; the Project file is a tab per section, the fields first
  assert.equal(settingsTab('general'), 'you');
  assert.equal(settingsTab('remotes'), 'sync');
  assert.equal(settingsTab('forges'), 'sync');
  assert.equal(settingsTab('github-project'), 'sync');
  assert.equal(settingsTab('project-file'), 'fields');
});

test('the tabs, in the menu order: the Overview, the Sync, the project file per section, You', () => {
  assert.deepEqual(
    settingsTabs.map((t) => t.id),
    ['overview', 'sync', 'fields', 'labels', 'views', 'sprints', 'roles', 'branches', 'cycle', 'release', 'repos', 'attachments', 'numbers', 'worktrees', 'advanced', 'you'],
  );
});

test('the sidebar groups: the project file\u2019s sections together, the rest without a heading', () => {
  const groups = settingsGroups();
  assert.deepEqual(
    groups.map((g) => g.group),
    ['', 'The project file', ''],
  );
  const byId = (i: number) => groups[i]?.tabs.map((t) => t.id) ?? [];
  assert.deepEqual(byId(1), ['fields', 'labels', 'views', 'sprints', 'roles', 'branches', 'cycle', 'release', 'repos', 'attachments', 'numbers', 'worktrees', 'advanced']);
  assert.deepEqual(byId(0), ['overview', 'sync']);
  assert.deepEqual(byId(2), ['you']);
});

test('the Overview says in one line where the sync stands', () => {
  const none = { project: '', write: false, last: null };
  assert.equal(syncSummary({ remote: null, forges: [], project: none }), 'No origin: the issues are only on this computer, no forge with sync on.');
  assert.equal(syncSummary({ remote: { name: 'origin' }, forges: [{ sync: true }, { sync: false }], project: none }), 'origin, 1 forge with sync on.');
  assert.equal(syncSummary({ remote: { name: 'origin' }, forges: [{ sync: true }, { sync: true }], project: { ...none, project: 'EPFL-ENAC/40' } }), 'origin, 2 forges with sync on, the GitHub Project EPFL-ENAC/40.');
});
