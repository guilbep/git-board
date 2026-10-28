import { test } from 'node:test';
import assert from 'node:assert/strict';
import { canConfirm, cardsFor, resultLine } from './startover.ts';

const none = { switched: true, git_bug: { refs: 0, cache: false, keys: 0 }, remote: false };

test('without origin, only this clone’s board can start over', () => {
  assert.deepEqual(cardsFor(none).map((c) => c.id), ['reset']);
});

test('with origin, the three resets, in the order of starting over', () => {
  const cards = cardsFor({ ...none, remote: true });
  assert.deepEqual(cards.map((c) => c.id), ['reset', 'push-prune', 'take']);
  assert.deepEqual(cards.map((c) => c.command), ['git board reset --hard', 'git board push --prune', 'git board reset --hard origin']);
});

test('git-bug’s data adds its card, with a button once switched', () => {
  const cards = cardsFor({ ...none, remote: true, git_bug: { refs: 53, cache: true, keys: 7 } });
  const prune = cards.at(-1)!;
  assert.equal(prune.id, 'prune');
  assert.equal(prune.command, 'git board prune');
  assert.equal(prune.button, 'Remove git-bug’s data');
});

test('unswitched, git-bug’s data is the only copy: no button, switch first', () => {
  const prune = cardsFor({ ...none, switched: false, git_bug: { refs: 3, cache: false, keys: 0 } }).at(-1)!;
  assert.equal(prune.id, 'prune');
  assert.equal(prune.button, '');
  assert.match(prune.does, /^git board switch first/);
});

test('the name is typed exactly: spaces at the ends aside', () => {
  assert.equal(canConfirm('git-board', 'git-board'), true);
  assert.equal(canConfirm(' git-board ', 'git-board'), true);
  assert.equal(canConfirm('Git-Board', 'git-board'), false);
  assert.equal(canConfirm('git', 'git-board'), false);
  assert.equal(canConfirm('', ''), false);
});

test('a result says what went and where it is kept', () => {
  assert.equal(resultLine({ deleted: 0, backup: '', me: false }), 'Nothing to delete');
  assert.equal(resultLine({ deleted: 12, backup: '/r/.git/b.bundle', me: false }), '12 refs deleted, backed up in /r/.git/b.bundle');
  assert.equal(resultLine({ deleted: 3, backup: '', me: false }), '3 refs deleted');
  assert.equal(resultLine({ deleted: 1, backup: '/b', me: true }), '1 refs deleted, backed up in /b; board.me is unset: its person is gone');
});
