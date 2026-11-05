import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { IssueEvent } from './api/board.ts';
import type { BodyVersion } from './sections.ts';
import { cardFor } from './cycle.ts';
import { mayGive, specOf, verdicts } from './spec.ts';

const b = { id: 'b', name: 'Person B', login: 'bob' };
const verdict = (id: string, at: string, v: string, on: number) =>
  ({ id, kind: 'comment', author: b, at, body: '', meta: { 'spec:verdict': v, 'spec:on': `body@v${on}` } }) as unknown as IssueEvent;
const vs = (n: number) => Array.from({ length: n }, (_, i) => ({ n: i + 1 })) as BodyVersion[];
const thread = (id: string, resolved?: string) =>
  ({ id, kind: 'comment', author: b, at: '0', body: '', meta: { 'thread:anchor': 'body@v1/expected', ...(resolved ? { 'thread:resolved': resolved } : {}) } }) as unknown as IssueEvent;

test('verdicts read spec:verdict on a version', () => {
  const plain = { id: 'p', kind: 'comment', author: b, at: '1', body: '' } as IssueEvent;
  assert.deepEqual(verdicts([verdict('2', '2', 'approve', 1), plain, verdict('1', '1', 'request-changes', 1), verdict('x', '3', 'maybe', 1)]).map((x) => x.event.id), ['1', '2']);
});

test('the last verdict on vN, and a verdict on v1 is before', () => {
  const s = specOf([verdict('1', '1', 'request-changes', 1), thread('t1', 'v2'), thread('t2'), thread('t3')], vs(2));
  assert.equal(s.v, 2);
  assert.equal(s.on, undefined);
  assert.equal(s.before?.verdict, 'request-changes');
  assert.equal(s.approved, undefined);
  assert.equal(s.open, 2);
  assert.equal(s.resolvedBy, 1);
  assert.equal(specOf([verdict('1', '1', 'request-changes', 2), verdict('2', '2', 'approve', 2)], vs(2)).on?.verdict, 'approve');
});

test('approved v2, v3 awaits', () => {
  const s = specOf([verdict('1', '1', 'approve', 2)], vs(3));
  assert.equal(s.approved?.v, 2);
  assert.equal(s.on, undefined);
});

test('mayGive: the spec holder, else anyone but the author', () => {
  assert.ok(mayGive({ spec: 'bob' }, 'alice', 'Bob'));
  assert.ok(!mayGive({ spec: 'bob' }, 'alice', 'carol'));
  assert.ok(!mayGive({ spec: '' }, 'alice', 'alice'));
  assert.ok(mayGive(undefined, 'alice', 'carol'));
  assert.ok(!mayGive(undefined, 'alice', undefined));
});

test('cardFor leaves spec-verdict to the verdict bar', () => {
  assert.equal(cardFor({ mine: true, actions: ['spec-verdict'] } as never, 'backlog'), null);
});
