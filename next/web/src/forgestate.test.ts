// Run with: node --test src/forgestate.test.ts
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { forgeKind, forgeState } from './forgestate.ts';

const gitlab = { name: 'GitLab', url: 'https://gitlab.example.org/team/app/-/issues/1', number: '1' };
const github = { name: 'GitHub', url: 'https://github.com/team/app/issues/2771', number: '2771' };

test('the shape from the forge', () => {
  assert.equal(forgeKind(github), 'github');
  assert.equal(forgeKind(gitlab), 'gitlab');
  assert.equal(forgeKind({ name: 'git.example.org', url: 'https://git.example.org/x' }), 'other');
});

test('nothing waiting: the bare icon, in sync', () => {
  assert.deepEqual(forgeState(github, 0), { kind: 'github', badge: null, n: 0, incoming: 0, label: 'GitHub #2771: in sync' });
});

test('changes to send: the count, said', () => {
  assert.deepEqual(forgeState(gitlab, 6), { kind: 'gitlab', badge: 'count', n: 6, incoming: 0, label: 'GitLab #1: 6 changes wait to be sent' });
  assert.equal(forgeState(gitlab, 1).label, 'GitLab #1: 1 change waits to be sent');
});

test('a run going, out of reach, changed on both sides', () => {
  assert.equal(forgeState(gitlab, 6, { running: true }).badge, 'syncing');
  assert.equal(forgeState(gitlab, 0, { left: 'no token' }).label, 'GitLab #1: out of reach (no token)');
  assert.equal(forgeState(gitlab, 0, { conflict: true }).badge, 'conflict');
});

test('out of reach wins over a count and a run', () => {
  assert.equal(forgeState(gitlab, 6, { running: true, left: 'timeout', conflict: true }).badge, 'unreachable');
  assert.equal(forgeState(gitlab, 6, { running: true, conflict: true }).badge, 'conflict');
});

// as the server tallies (forges.go add): an issue pulled is changed once,
// changes counts its operations; one sent is changed and its changes
const way = (changed = 0, changes = 0) => ({ issues: 0, changed, comments: 0, edits: 0, changes });
const tally = (o: { to?: number; from?: number; conflicts?: number; left?: string } = {}) => ({
  to: way(o.to ? 1 : 0, o.to), from: way(o.from ? 1 : 0, o.from), linked: 1, stays_local: 0, conflicts: o.conflicts ?? 0, assignable: 0, warnings: 0, errors: 0, held: 0, lines: [], ...(o.left ? { left: o.left } : {}),
});
const bridges = [{ name: 'gh', target: 'github', host: 'github.com', path: 'team/app' }, { name: 'gl', target: 'gitlab', host: 'gitlab.example.org' }];
const job = (o: { state?: 'running' | 'done' | 'failed' | undefined; dry?: boolean | undefined; ended?: string | undefined; bridges?: Record<string, ReturnType<typeof tally>> } = {}) => ({
  dry_run: o.dry ?? false, mode: 'sync' as const, state: o.state ?? 'done', started: '2026-10-30T10:00:00Z', names: ['gh', 'gl'], bridges: o.bridges ?? {}, errors: 0, ...(o.ended ? { ended: o.ended } : {}),
});
const check = (bridges: Record<string, ReturnType<typeof tally>>, o: { dry?: boolean; ended?: string } = {}) => ({ dry_run: o.dry ?? true, writes: 0, errors: 0, bridges, ended: o.ended ?? '2026-10-30T10:05:00Z' });

test('new there since the last sync: on its own, and beside a count', () => {
  assert.deepEqual(forgeState(gitlab, 0, { incoming: 3 }), { kind: 'gitlab', badge: 'count', n: 0, incoming: 3, label: 'GitLab #1: 3 new there since the last sync' });
  assert.equal(forgeState(gitlab, 6, { incoming: 1 }).label, 'GitLab #1: 6 changes wait to be sent, 1 new there since the last sync');
  assert.equal(forgeState(gitlab, 6, { incoming: 1, running: true }).badge, 'syncing');
});
