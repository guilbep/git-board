// Run with: node --test src/configcheck.test.ts
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { checkOf } from './configcheck.ts';

test('a text that isn’t TOML: its line and column', () => {
  const c = checkOf('[fields.a]\nsingle = \n', '');
  assert.equal(c.ok, false);
  assert.equal(c.error?.line, 2);
});

test('TOML the server refused: its message', () => {
  assert.deepEqual(checkOf('[nope]\nx = 1\n', 'nope.x: git-board doesn’t know this key'), {
    ok: false, message: 'nope.x: git-board doesn’t know this key', warnings: [], summary: { fields: [], views: [], sprints: 0 },
  });
});

test('a good file: what it holds', () => {
  const c = checkOf('[fields.a]\n[fields.b]\n[views.x]\nfilter = ""\n[[sprints.list]]\nslug = "s"\n', '');
  assert.deepEqual(c.summary, { fields: ['a', 'b'], views: ['x'], sprints: 1 });
});
