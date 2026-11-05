// Run with: node --test src/experimental.test.ts
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { featureOn, toggled, whereFrom } from './experimental.ts';

test('a feature is on when GET /api/board lists it; none listed, off', () => {
  assert.equal(featureOn(['cycle'], 'cycle'), true);
  assert.equal(featureOn([], 'cycle'), false);
  assert.equal(featureOn(null, 'cycle'), false);
  assert.equal(featureOn(undefined, 'cycle'), false);
});

test('a toggle keeps the names the page doesn’t know', () => {
  assert.deepEqual(toggled([], 'cycle', true), ['cycle']);
  assert.deepEqual(toggled(['cycle', 'beta'], 'cycle', false), ['beta']);
  assert.deepEqual(toggled(['agents'], 'cycle', true), ['cycle', 'agents']);
  assert.deepEqual(toggled(['cycle'], 'cycle', true), ['cycle']);
});

test('where the features come from: the file, unless the clone sets its own', () => {
  assert.match(whereFrom(null), /\.git-board\.toml’s \[experimental\]/);
  assert.match(whereFrom(['cycle']), /board\.experimental \(cycle\) wins/);
  assert.match(whereFrom([]), /\(empty\)/);
});
