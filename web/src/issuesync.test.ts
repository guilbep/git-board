import { test } from 'node:test';
import assert from 'node:assert/strict';
import { tallyWords } from './issuesync.ts';

const dir = (o: Partial<Record<'issues' | 'changed' | 'comments' | 'edits' | 'changes', number>> = {}) => ({ issues: 0, changed: 0, comments: 0, edits: 0, changes: 0, ...o });
const tally = (to = dir(), from = dir(), errors = 0) => ({ to, from, linked: 0, stays_local: 0, warnings: 0, errors, lines: [] });

test('tallyWords says what one issue sync did on a forge', () => {
  assert.equal(tallyWords(tally(dir({ comments: 1 })), true), 'would send 1 comment');
  assert.equal(tallyWords(tally(dir({ issues: 1, comments: 2 })), false), 'created the issue with 2 comments');
  assert.equal(tallyWords(tally(dir(), dir({ changes: 3 })), false), 'got 3 changes');
  assert.equal(tallyWords(tally(), false), 'up to date');
  assert.equal(tallyWords(tally(dir(), dir(), 1), false), '1 error');
});
