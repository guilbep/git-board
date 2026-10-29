import { test } from 'node:test';
import assert from 'node:assert/strict';
import { viewWords } from './view.ts';

test('the classic page by default, and why', () => {
  assert.equal(viewWords({ view: 'classic', view_from: 'default' }), 'Your issues open on the classic page: the board’s .git-board.toml has no [cycle] table.');
});

test('the cycle’s page from the board’s file', () => {
  assert.equal(viewWords({ view: 'cycle', view_from: '.git-board.toml' }), 'Your issues open on the cycle’s page: the board’s .git-board.toml has a [cycle] table.');
});

test('either page from board.view', () => {
  assert.equal(viewWords({ view: 'cycle', view_from: 'board.view' }), 'Your issues open on the cycle’s page: your git config sets board.view.');
  assert.equal(viewWords({ view: 'classic', view_from: 'board.view' }), 'Your issues open on the classic page: your git config sets board.view.');
});

test('board.view names the level that set it', () => {
  assert.equal(viewWords({ view: 'classic', view_from: 'board.view', view_scope: 'local' }), 'Your issues open on the classic page: this repository’s git config sets board.view.');
  assert.equal(viewWords({ view: 'cycle', view_from: 'board.view', view_scope: 'global' }), 'Your issues open on the cycle’s page: your global git config sets board.view.');
});

test('a board.view that is neither is said to be ignored', () => {
  assert.equal(
    viewWords({ view: 'classic', view_from: 'default', view_ignored: 'kanban' }),
    'Your issues open on the classic page: the board’s .git-board.toml has no [cycle] table. board.view = “kanban” is neither classic nor cycle, so it’s ignored.',
  );
});

test('nothing known yet, nothing said', () => {
  assert.equal(viewWords(null), '');
});
