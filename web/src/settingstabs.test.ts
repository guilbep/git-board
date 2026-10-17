import { test } from 'node:test';
import assert from 'node:assert/strict';
import { channels, settingsTab, settingsTabs } from './settingstabs.ts';

test('settingsTab takes the address, General without one or for an old link', () => {
  assert.equal(settingsTab('forges'), 'forges');
  assert.equal(settingsTab('project-file'), 'project-file');
  assert.equal(settingsTab('github-project'), 'github-project');
  assert.equal(settingsTab(''), 'general');
  assert.equal(settingsTab(undefined), 'general');
  assert.equal(settingsTab('bridges'), 'general');
  assert.equal(settingsTab(['forges']), 'general');
});

test('the tabs, in the menu order: the three channels last, numbered', () => {
  assert.deepEqual(settingsTabs.map((t) => t.id), ['general', 'project-file', 'attachments', 'remotes', 'forges', 'github-project']);
  assert.deepEqual(channels.map((c) => `${c.label} (${c.via})`), ['Remotes (git)', 'Forges (API)', 'GitHub Project (fields)']);
});
