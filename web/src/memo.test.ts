// Run with: node --test web/src/memo.test.ts
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { memo } from './memo.ts';

test('a text asked twice is rendered once', async () => {
  let calls = 0;
  const render = memo(async (s: string) => (calls++, `<p>${s}</p>`));
  assert.equal(await render('a'), '<p>a</p>');
  assert.equal(await render('a'), '<p>a</p>');
  assert.equal(await render('b'), '<p>b</p>');
  assert.equal(calls, 2);
});

test('a refusal is not kept: the next ask goes again', async () => {
  let calls = 0;
  const render = memo(async (s: string) => {
    if (++calls === 1) throw new Error('server down');
    return s;
  });
  await assert.rejects(render('a'), /server down/);
  assert.equal(await render('a'), 'a');
  assert.equal(calls, 2);
});

test('past max keys, the oldest goes', async () => {
  let calls = 0;
  const render = memo(async (s: string) => (calls++, s), 2);
  await render('a');
  await render('b');
  await render('c'); // a goes
  await render('b');
  assert.equal(calls, 3);
  await render('a');
  assert.equal(calls, 4);
});
