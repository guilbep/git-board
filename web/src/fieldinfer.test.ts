import assert from 'node:assert/strict';
import { test } from 'node:test';
import { allNumbers, inferType } from './fieldinfer.ts';

test('every value a number is the numbers mode', () => {
  assert.ok(allNumbers(['0.5', '1', '2', '4', '8']));
  assert.ok(allNumbers(['1e3']));
  assert.ok(!allNumbers(['1', 'two']));
  assert.ok(!allNumbers([]));
});

test('the inference follows the server TypeOf: numeric, name, values', () => {
  assert.equal(inferType('est', ['1', '2'], false), 'number');
  assert.equal(inferType('size', [], true), 'number');
  assert.equal(inferType('tech-lead', [], false, ['tech-lead']), 'person');
  assert.equal(inferType('assignee', [], false), 'person');
  assert.equal(inferType('due', ['2026-01-02'], false), 'date');
  assert.equal(inferType('due', [], false), 'date');
  assert.equal(inferType('blocked-by', ['#12'], false), 'issue');
  assert.equal(inferType('status', ['todo', 'done'], false), 'select');
  // an explicit numeric wins over what the values would say
  assert.equal(inferType('rank', ['a', 'b'], true), 'number');
});
