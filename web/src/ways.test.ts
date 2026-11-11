// Run with: node --test src/ways.test.ts
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { aheadWords, remoteWords, reportWords, runWords, stepWords, tallyGroups, tallyWords } from './ways.ts';

test('what a fetch found waiting, and what a pull or a push did', () => {
  assert.equal(aheadWords({ send: 3, receive: 0 }, 'issue'), '3 issues to send · 0 to receive');
  assert.equal(remoteWords({ remote: 'origin', action: 'fetch', issues: { send: 1, receive: 2 }, people: { send: 0, receive: 1 } }), '1 issue to send · 2 to receive; 0 people to send · 1 to receive');
  assert.equal(remoteWords({ remote: 'origin', action: 'fetch' }), 'Up to date');
  assert.equal(remoteWords({ remote: 'origin', action: 'push', note: 'numbered CO2-12' }), 'Pushed: numbered CO2-12');
  assert.equal(remoteWords({ remote: 'origin', action: 'pull' }), 'Pulled');
});

test('a run’s steps and counts', () => {
  assert.equal(stepWords({ name: 'pull', state: 'done' }), 'git pull: done');
  assert.equal(stepWords({ forge: 'github', way: 'push', what: 'created', issue: '#12' }), 'github push: created #12');
  assert.equal(stepWords({ forge: 'github', way: 'pull', line: 'reading', done: 2, total: 40 }), 'github pull: reading 2 of 40');
  assert.equal(reportWords({ created: 2, updated: 0, unchanged: 5, posted: 1, pushed: 0 }), '2 created, 1 posted');
  assert.equal(reportWords({ created: 0, updated: 0, unchanged: 5, posted: 0, pushed: 0 }), 'nothing to change');
  assert.equal(reportWords({ created: 0, updated: 0, unchanged: 0, posted: 0, pushed: 1, restored: 2 }), '1 pushed, 2 put back on a backup');
});

test('what the Sync button says', () => {
  const run = { id: '1', scope: 'all' as const, way: 'sync' as const, dry_run: false, started_at: '', state: 'done' as const, steps: [], tallies: [] };
  assert.equal(runWords(null), 'not synced from here yet');
  assert.equal(runWords({ ...run, state: 'running' }), 'syncing now');
  assert.equal(runWords({ ...run, state: 'failed', error: 'origin refused' }), 'the last sync failed: origin refused');
  assert.equal(runWords(run), 'synced');
  assert.equal(runWords({ ...run, scope: 'forges', report: { created: 1, updated: 0, unchanged: 0, posted: 0, pushed: 0 } }), 'synced the forges: 1 created');
});

test('tallies: in words, by forge and way, warnings apart', () => {
  assert.equal(tallyWords({ what: 'create-comment', n: 1200, examples: [] }), `${(1200).toLocaleString()} comments posted`);
  assert.equal(tallyWords({ what: 'posted', n: 1, examples: [] }), '1 issue created there');
  assert.equal(tallyWords({ what: 'warning: refused "…": no account there', n: 3, examples: [] }), '3 times: refused "…": no account there');
  const g = tallyGroups([
    { forge: 'gitlab', way: 'push', what: 'set-status', n: 2, examples: [] },
    { forge: 'gitlab', way: 'push', what: 'create-comment', n: 9, examples: [] },
    { forge: 'gitlab', way: 'pull', what: 'updated', n: 1, examples: [] },
    { forge: 'gitlab', what: 'warning: x', n: 4, examples: [] },
    { forge: 'github', way: 'pull', what: 'created', n: 1, examples: [] },
  ]);
  assert.deepEqual(g.map((x) => x.forge), ['gitlab', 'github']);
  assert.deepEqual(g[0]!.ways.map((w) => w.way), ['pull', 'push']);
  assert.deepEqual(g[0]!.ways[1]!.tallies.map((t) => t.what), ['create-comment', 'set-status']);
  assert.equal(g[0]!.warnings.length, 1);
});
