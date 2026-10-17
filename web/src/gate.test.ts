import { test } from 'node:test';
import assert from 'node:assert/strict';
import { actThenRead, needsPush } from './gate.ts';

// A refused Merge or Open keeps GitHub's message on the page through the
// re-read that follows it.
test('the action’s error outlives a clean re-read', async () => {
  let read = 0;
  const shown = await actThenRead(
    () => Promise.reject(new Error('GitHub: Head branch was modified. Review and try the merge again.')),
    async () => (read++, ''),
  );
  assert.equal(read, 1);
  assert.equal(shown, 'GitHub: Head branch was modified. Review and try the merge again.');
});

test('a done action shows the re-read’s error, or none', async () => {
  assert.equal(await actThenRead(async () => {}, async () => 'GitHub: Bad credentials'), 'GitHub: Bad credentials');
  assert.equal(await actThenRead(async () => {}, async () => ''), '');
});

const branch = (name: string, remote = '', in_base = false) => ({ name, remote, in_base });

// A branch only here is told to be pushed for a PR, unless its work is in
// the base already: after a merge and a pruned fetch, pushing it would
// recreate a merged branch on GitHub.
test('only unmerged branches only here are told to push', () => {
  const all = [branch('fix/1-a'), branch('fix/1-a', 'origin'), branch('fix/1-b'), branch('fix/1-c', '', true)];
  assert.deepEqual(all.filter((b) => needsPush(b, all, 'o/r')).map((b) => b.name), ['fix/1-b']);
  assert.equal(needsPush(branch('fix/1-b'), all, ''), false, 'no GitHub origin, no PR to open');
});
