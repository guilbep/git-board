// Run with: node --test web/src/apierror.test.ts
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { errorOf } from './apierror.ts';

test('writeError and writeCoded: the error, and its code', () => {
  assert.deepEqual(errorOf('{"error":"no such issue"}'), { message: 'no such issue', code: '' });
  assert.deepEqual(errorOf('{"error":"a forge sync runs","code":"syncing"}'), { message: 'a forge sync runs', code: 'syncing' });
});

test('a config check refused for its syntax: the message at its line and column', () => {
  const body = '{"ok":false,"error":{"line":3,"col":7,"message":"expected a value"},"warnings":[],"summary":{"fields":null,"views":null,"sprints":0}}';
  assert.deepEqual(errorOf(body), { message: 'line 3, column 7: expected a value', code: '' });
});

test('a config check refused for the rest: its message, not the JSON', () => {
  const body = '{"ok":false,"message":"fields.prio: an option twice","warnings":[],"summary":{"fields":null,"views":null,"sprints":0}}';
  assert.deepEqual(errorOf(body), { message: 'fields.prio: an option twice', code: '' });
});

test('a body not JSON, or JSON with nothing to say, is shown as it is', () => {
  assert.deepEqual(errorOf('Bad Gateway'), { message: 'Bad Gateway', code: '' });
  assert.deepEqual(errorOf(''), { message: '', code: '' });
  assert.deepEqual(errorOf('null'), { message: 'null', code: '' });
  assert.deepEqual(errorOf('{"ok":false}'), { message: '{"ok":false}', code: '' });
});
