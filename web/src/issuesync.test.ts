import { test } from 'node:test';
import assert from 'node:assert/strict';
import { forgeLabel, forgeNumbers, tallyWords } from './issuesync.ts';

const dir = (o: Partial<Record<'issues' | 'changed' | 'comments' | 'edits' | 'changes', number>> = {}) => ({ issues: 0, changed: 0, comments: 0, edits: 0, changes: 0, ...o });
const tally = (to = dir(), from = dir(), errors = 0) => ({ to, from, linked: 0, stays_local: 0, conflicts: 0, assignable: 0, warnings: 0, errors, held: 0, lines: [] });

test('tallyWords says what one issue sync did on a forge', () => {
  assert.equal(tallyWords(tally(dir({ comments: 1 })), true), 'would send 1 comment');
  assert.equal(tallyWords(tally(dir({ issues: 1, comments: 2 })), false), 'created the issue with 2 comments');
  assert.equal(tallyWords(tally(dir(), dir({ changes: 3 })), false), 'got 3 changes');
  assert.equal(tallyWords(tally(), false), 'up to date');
  assert.equal(tallyWords(tally(dir(), dir(), 1), false), '1 error');
});

test('forgeLabel names a bridge by its forge', () => {
  assert.equal(forgeLabel({ name: 'gl', target: 'gitlab' }), 'GitLab (gl)');
  assert.equal(forgeLabel({ name: 'x', target: 'jira' }), 'jira (x)');
});

test('forgeNumbers: the issue’s numbers on the forges', () => {
  const forges = [
    { name: 'GitHub', url: 'https://github.com/o/r/issues/2518', number: '2518' },
    { name: 'gitea.example.com', url: 'https://gitea.example.com/o/r/issues/1' },
    { name: 'GitLab', url: 'https://gitlab.example.com/g/r/-/issues/77', number: '77' },
  ];
  assert.deepEqual(forgeNumbers({ forges }), ['2518', '77']);
  assert.deepEqual(forgeNumbers({}), []);
});
