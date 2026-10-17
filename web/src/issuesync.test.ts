import { test } from 'node:test';
import assert from 'node:assert/strict';
import { forgeLabel, linkable, tallyWords } from './issuesync.ts';

const dir = (o: Partial<Record<'issues' | 'changed' | 'comments' | 'edits' | 'changes', number>> = {}) => ({ issues: 0, changed: 0, comments: 0, edits: 0, changes: 0, ...o });
const tally = (to = dir(), from = dir(), errors = 0) => ({ to, from, linked: 0, stays_local: 0, warnings: 0, errors, lines: [] });

test('tallyWords says what one issue sync did on a forge', () => {
  assert.equal(tallyWords(tally(dir({ comments: 1 })), true), 'would send 1 comment');
  assert.equal(tallyWords(tally(dir({ issues: 1, comments: 2 })), false), 'created the issue with 2 comments');
  assert.equal(tallyWords(tally(dir(), dir({ changes: 3 })), false), 'got 3 changes');
  assert.equal(tallyWords(tally(), false), 'up to date');
  assert.equal(tallyWords(tally(dir(), dir(), 1), false), '1 error');
});

test('linkable: the sync bridges an issue is not on, nor was', () => {
  const bridges = [{ name: 'gh', target: 'github' }, { name: 'gl', target: 'gitlab' }, { name: 'gl2', target: 'gitlab' }];
  const forges = [
    { name: 'GitHub', url: 'https://github.com/o/r/issues/7', bridge: 'gh' },
    { name: 'GitLab', url: 'https://gitlab.example.com/g/r/-/issues/3', bridge: 'gl', gone: 'unlinked' as const },
    { name: 'gitea.example.com', url: 'https://gitea.example.com/o/r/issues/1' },
  ];
  assert.deepEqual(linkable(bridges, forges).map((b) => b.name), ['gl2']);
  assert.deepEqual(linkable(bridges, []).map((b) => b.name), ['gh', 'gl', 'gl2']);
});

test('forgeLabel names a bridge by its forge', () => {
  assert.equal(forgeLabel({ name: 'gl', target: 'gitlab' }), 'GitLab (gl)');
  assert.equal(forgeLabel({ name: 'x', target: 'jira' }), 'jira (x)');
});
