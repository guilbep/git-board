// Run with: node --test src/interaction.test.ts
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { begin, finished, onSlow, routeName, started, traceparent } from './interaction.ts';

test('an interaction: its requests carry its trace, it ends after the last, a slow one is told', async () => {
  const slow: string[] = [];
  onSlow((i, ms) => slow.push(`${i.name} ${ms > 200}`));
  const i = begin('open issue');
  assert.match(traceparent(), new RegExp(`^00-${i.trace}-${i.span}-01$`));
  started();
  await new Promise((r) => setTimeout(r, 220));
  finished();
  await new Promise((r) => setTimeout(r, 20));
  assert.deepEqual(slow, ['open issue true']);
  // over: a request now is a trace of its own
  assert.doesNotMatch(traceparent(), new RegExp(i.trace));
});

test('a quick one is not told', async () => {
  const slow: string[] = [];
  onSlow((i) => slow.push(i.name));
  begin('filter');
  started();
  finished();
  await new Promise((r) => setTimeout(r, 20));
  assert.deepEqual(slow, []);
});

test('routeName: ids become :id', () => {
  assert.equal(routeName('/issues/3b32a91'), '/issues/:id');
  assert.equal(routeName('/issues/VM-12'), '/issues/:id');
  assert.equal(routeName('/settings/sync'), '/settings/sync');
});

test('an act that asks nothing ends at its first frame, once QUIET_MS showed it asks nothing', async () => {
  const { settled, QUIET_MS } = await import('./interaction.ts');
  const before = settled.length;
  begin('nothing asked');
  await new Promise((r) => setTimeout(r, QUIET_MS / 2));
  assert.equal(settled.length, before, 'ended before its first request could come');
  await new Promise((r) => setTimeout(r, QUIET_MS));
  const last = settled.at(-1)!;
  assert.equal(last.name, 'nothing asked');
  assert.ok(last.ms < QUIET_MS, `timed to its first frame, not to the wait: ${last.ms} ms`);
});
