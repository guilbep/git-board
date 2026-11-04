import { test } from 'node:test';
import assert from 'node:assert/strict';
import { featureOn, toggled, whereFrom } from './experimental.ts';

test('a feature is on only when /api/me lists it', () => {
  assert.equal(featureOn(['cycle'], 'cycle'), true);
  assert.equal(featureOn(['cycle'], 'agents'), false);
  assert.equal(featureOn([], 'cycle'), false);
  assert.equal(featureOn(undefined, 'worktrees'), false);
  assert.equal(featureOn(null, 'worktrees'), false);
});

test('toggled keeps the features in order, and a name the page does not know', () => {
  assert.deepEqual(toggled([], 'worktrees', true), ['worktrees']);
  assert.deepEqual(toggled(['worktrees'], 'cycle', true), ['cycle', 'worktrees']);
  assert.deepEqual(toggled(['cycle', 'worktrees'], 'cycle', false), ['worktrees']);
  assert.deepEqual(toggled(['cycle'], 'cycle', true), ['cycle']);
  assert.deepEqual(toggled(['teleport', 'agents'], 'cycle', true), ['cycle', 'agents', 'teleport']);
});

test('whereFrom names the clone’s own setting when it wins', () => {
  assert.match(whereFrom(null), /\[experimental\] features/);
  assert.match(whereFrom(['cycle']), /board\.experimental \(cycle\) wins/);
  assert.match(whereFrom([]), /\(empty\)/);
});
