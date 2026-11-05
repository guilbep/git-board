// Run with: node --test src/api/error.test.ts
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { ApiError, errorOf, isStale } from './error.ts';

test('the one error shape: its code, message and field', () => {
  assert.deepEqual(errorOf('{"code":"stale","message":"the issue changed since you read it","field":"title"}'), {
    code: 'stale', message: 'the issue changed since you read it', field: 'title',
  });
  assert.deepEqual(errorOf('{"code":"not-found","message":"no such issue"}'), { code: 'not-found', message: 'no such issue', field: '' });
});

test('a body not the shape is shown as it is', () => {
  assert.deepEqual(errorOf('Bad Gateway'), { code: '', message: 'Bad Gateway', field: '' });
  assert.deepEqual(errorOf(''), { code: '', message: '', field: '' });
  assert.deepEqual(errorOf('null'), { code: '', message: 'null', field: '' });
  assert.deepEqual(errorOf('{"ok":false}'), { code: '', message: '{"ok":false}', field: '' });
});

test('stale is a 412', () => {
  assert.equal(isStale(new ApiError('changed', 412, 'stale')), true);
  assert.equal(isStale(new ApiError('gone', 404, 'not-found')), false);
  assert.equal(isStale(new Error('x')), false);
});
