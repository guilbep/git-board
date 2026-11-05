import { test } from 'node:test';
import assert from 'node:assert/strict';
import { lineDiff } from './linediff.ts';

test('lineDiff keeps, removes and adds lines', () => {
  const ops = (a: string, b: string) => lineDiff(a, b).map((l) => l.op + l.text);
  assert.deepEqual(ops('a\nb\nc', 'a\nc\nd'), [' a', '-b', ' c', '+d']);
  assert.deepEqual(ops('', 'x'), ['+x']);
  assert.deepEqual(ops('x', ''), ['-x']);
  assert.deepEqual(ops('same', 'same'), [' same']);
});
