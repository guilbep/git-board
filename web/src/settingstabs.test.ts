import { test } from 'node:test';
import assert from 'node:assert/strict';
import { settingsTab, settingsTabs, syncSummary } from './settingstabs.ts';

test('settingsTab takes the address, the Overview without one or for an unknown link', () => {
  assert.equal(settingsTab('sync'), 'sync');
  assert.equal(settingsTab('project-file'), 'project-file');
  assert.equal(settingsTab('numbers'), 'numbers');
  assert.equal(settingsTab('labels'), 'labels');
  assert.equal(settingsTab(''), 'overview');
  assert.equal(settingsTab(undefined), 'overview');
  assert.equal(settingsTab('bridges'), 'overview');
  assert.equal(settingsTab(['sync']), 'overview');
  // the old tabs' links: General held who you write as; the three ways are one Sync
  assert.equal(settingsTab('general'), 'you');
  assert.equal(settingsTab('remotes'), 'sync');
  assert.equal(settingsTab('forges'), 'sync');
  assert.equal(settingsTab('github-project'), 'sync');
});

test('the tabs, in the menu order: the Overview, the Sync, then this board', () => {
  assert.deepEqual(settingsTabs.map((t) => t.id), ['overview', 'sync', 'project-file', 'labels', 'numbers', 'attachments', 'worktrees', 'you']);
});

test('the Overview says in one line where the sync stands', () => {
  const none = { project: '', write: false, last: null };
  assert.equal(syncSummary({ remote: null, forges: [], project: none }), 'No origin: the issues are only on this computer, no forge with sync on.');
  assert.equal(syncSummary({ remote: { name: 'origin' }, forges: [{ sync: true }, { sync: false }], project: none }), 'origin, 1 forge with sync on.');
  assert.equal(syncSummary({ remote: { name: 'origin' }, forges: [{ sync: true }, { sync: true }], project: { ...none, project: 'EPFL-ENAC/40' } }), 'origin, 2 forges with sync on, the GitHub Project EPFL-ENAC/40.');
});
