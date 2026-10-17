import { test } from 'node:test';
import assert from 'node:assert/strict';
import { actThenRead } from './gate.ts';

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
