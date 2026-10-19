import { test } from 'node:test';
import assert from 'node:assert/strict';
import { channels, forgeSummary, remoteSummary, settingsTab, settingsTabs } from './settingstabs.ts';

test('settingsTab takes the address, the Overview without one or for an unknown link', () => {
  assert.equal(settingsTab('forges'), 'forges');
  assert.equal(settingsTab('project-file'), 'project-file');
  assert.equal(settingsTab('github-project'), 'github-project');
  assert.equal(settingsTab('numbers'), 'numbers');
  assert.equal(settingsTab(''), 'overview');
  assert.equal(settingsTab(undefined), 'overview');
  assert.equal(settingsTab('bridges'), 'overview');
  assert.equal(settingsTab(['forges']), 'overview');
  // General held who you write as: its old links open You
  assert.equal(settingsTab('general'), 'you');
});

test('the tabs, in the menu order: the Overview, the three channels numbered, then this board', () => {
  assert.deepEqual(settingsTabs.map((t) => t.id), ['overview', 'remotes', 'forges', 'github-project', 'project-file', 'numbers', 'attachments', 'worktrees', 'you']);
  assert.deepEqual(channels.map((c) => `${c.label} (${c.via})`), ['Remotes (git)', 'Forges (API)', 'GitHub Project (fields)']);
});

test('the Overview says in one line where the forges and remotes stand', () => {
  assert.equal(forgeSummary([]), 'No forge yet.');
  assert.equal(forgeSummary([{ sync: true }]), '1 forge, sync on.');
  assert.equal(forgeSummary([{ sync: false }, { sync: false }]), '2 forges, sync off.');
  assert.equal(forgeSummary([{ sync: true }, { sync: false }]), '2 forges, 1 with sync on.');
  assert.equal(remoteSummary([]), 'No remote yet: the issues are only on this computer.');
  assert.equal(remoteSummary([{ name: 'origin' }]), 'origin.');
  assert.equal(remoteSummary([{ name: 'origin' }, { name: 'alice' }]), 'origin, alice.');
});
