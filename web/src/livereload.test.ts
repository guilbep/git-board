// Run with: node --test src/livereload.test.ts
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { boardReload } from './livereload.ts';

const me = 'tab-1';

test('a write to the issues reloads them, this tab’s own too', () => {
  assert.equal(boardReload({ kind: 'issue', by: 'tab-2' }, me), 'issues');
  assert.equal(boardReload({ kind: 'issue', by: me }, me), 'issues');
  assert.equal(boardReload({ kind: 'issue' }, me), 'issues'); // a command's, a sync's
  assert.equal(boardReload({ kind: 'resync' }, me), 'issues'); // a reconnect's
});

test('a config change from elsewhere reloads the config, this tab’s own does not', () => {
  assert.equal(boardReload({ kind: 'config', by: 'tab-2' }, me), 'config');
  assert.equal(boardReload({ kind: 'config', by: me }, me), null);
});

test('a sync from elsewhere reloads the issues', () => {
  assert.equal(boardReload({ kind: 'sync' }, me), 'issues');
  assert.equal(boardReload({ kind: 'sync', by: me }, me), null);
});

test('a run’s progress, the inbox, a person, who writes and a release reload nothing', () => {
  for (const kind of ['progress', 'inbox', 'person', 'me', 'release']) {
    assert.equal(boardReload({ kind }, me), null, kind);
    assert.equal(boardReload({ kind, by: 'tab-2' }, me), null, kind);
  }
});
