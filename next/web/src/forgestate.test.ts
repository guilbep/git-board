import { test } from 'node:test';
import assert from 'node:assert/strict';
import { bridgeFor, checkHolds, forgeKind, forgeState, forgeStates, incomingOf } from './forgestate.ts';

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

test('a forge link is the bridge on its host, in its project', () => {
  assert.equal(bridgeFor(gitlab, bridges)?.name, 'gl');
  assert.equal(bridgeFor(github, bridges)?.name, 'gh');
  // GitHub spells the project as it likes; a link keeps the user's case
  assert.equal(bridgeFor({ name: 'GitHub', url: 'https://github.com/Team/App/issues/3' }, bridges)?.name, 'gh');
  // another project on the host, or one whose name only starts the same
  assert.equal(bridgeFor({ name: 'GitHub', url: 'https://github.com/team/app-old/issues/3' }, bridges), undefined);
  assert.equal(bridgeFor({ name: 'git.example.org', url: 'https://git.example.org/x' }, bridges), undefined);
  // a GitLab with no "gitlab" in its name or URL: its host says
  const epfl = { name: 'git.epfl.ch', url: 'https://git.epfl.ch/team/app/-/issues/9', number: '9' };
  assert.equal(bridgeFor(epfl, [{ name: 'EPFL', target: 'gitlab', host: 'git.epfl.ch' }])?.name, 'EPFL');
  // two GitLabs: on two hosts, each its own; on one, the one in its path
  const two = [{ name: 'gl', target: 'gitlab', host: 'gitlab.example.org' }, { name: 'epfl', target: 'gitlab', host: 'git.epfl.ch', path: 'team/app' }];
  assert.equal(bridgeFor(gitlab, two)?.name, 'gl');
  assert.equal(bridgeFor(epfl, two)?.name, 'epfl');
  const oneHost = [{ name: 'a', target: 'gitlab', host: 'git.epfl.ch', path: 'team/app' }, { name: 'b', target: 'gitlab', host: 'git.epfl.ch' }];
  assert.equal(bridgeFor(epfl, oneHost)?.name, 'a');
  // two by id on one host: neither, no badge rather than the wrong one
  assert.equal(bridgeFor(epfl, [{ name: 'a', target: 'gitlab', host: 'git.epfl.ch' }, { name: 'b', target: 'gitlab', host: 'git.epfl.ch' }]), undefined);
});

test('what a check would bring in, per forge: each operation once', () => {
  assert.equal(incomingOf(tally({ from: 3 })), 3); // one issue, three operations
  assert.equal(incomingOf(tally({ to: 2 })), 0);
  assert.equal(incomingOf(undefined), 0);
});

test('new there since the last sync: on its own, and beside a count', () => {
  assert.deepEqual(forgeState(gitlab, 0, { incoming: 3 }), { kind: 'gitlab', badge: 'count', n: 0, incoming: 3, label: 'GitLab #1: 3 new there since the last sync' });
  assert.equal(forgeState(gitlab, 6, { incoming: 1 }).label, 'GitLab #1: 6 changes wait to be sent, 1 new there since the last sync');
  assert.equal(forgeState(gitlab, 6, { incoming: 1, running: true }).badge, 'syncing');
});

test('the states wired: out of reach from the last run, both sides and new from the issue’s check', () => {
  const st = (o: Parameters<typeof forgeStates>[2]) => forgeStates([github, gitlab], () => 0, o).map((x) => x.state);
  const none = { bridges, job: null, check: null };
  assert.deepEqual(st(none).map((s) => s.badge), [null, null]);
  // out of reach: the last run's left, on that forge only
  assert.deepEqual(st({ ...none, job: job({ bridges: { gl: tally({ left: 'timeout' }) } }) }).map((s) => s.badge), [null, 'unreachable']);
  // a real run going is syncing; a dry run is not
  assert.deepEqual(st({ ...none, job: job({ state: 'running' }) }).map((s) => s.badge), ['syncing', 'syncing']);
  assert.deepEqual(st({ ...none, job: job({ state: 'running', dry: true }) }).map((s) => s.badge), [null, null]);
  // the issue's own check: both sides on gh, three new on gl
  const s = st({ ...none, check: check({ gh: tally({ conflicts: 1 }), gl: tally({ from: 3 }) }) });
  assert.equal(s[0]!.badge, 'conflict');
  assert.equal(s[1]!.incoming, 3);
});

test('out of reach: the newer of the issue’s own run and the board’s', () => {
  const st = (o: Parameters<typeof forgeStates>[2]) => forgeStates([gitlab], () => 0, o)[0]!.state;
  const old = job({ ended: '2026-10-30T10:01:00Z', bridges: { gl: tally({ left: 'timeout' }) } });
  // the issue's Refresh reached it since: what it found shows
  const reached = st({ bridges, job: old, check: check({ gl: tally({ from: 2, conflicts: 1 }) }) });
  assert.equal(reached.badge, 'conflict');
  assert.equal(reached.incoming, 2);
  // its own real run reached it: no badge, what was new is in
  assert.deepEqual(st({ bridges, job: old, check: check({ gl: tally() }, { dry: false }) }).badge, null);
  // the issue's run didn't reach it: out of reach
  assert.equal(st({ bridges, job: null, check: check({ gl: tally({ left: 'no token' }) }) }).label, 'GitLab #1: out of reach (no token)');
  // a check of the board's after it, which didn't reach it, is the newer
  const later = job({ dry: true, ended: '2026-10-30T10:09:00Z', bridges: { gl: tally({ left: 'timeout' }) } });
  assert.equal(st({ bridges, job: later, check: check({ gl: tally() }) }).badge, 'unreachable');
});

test('the icon is its bridge’s kind, a GitLab of any name', () => {
  const epfl = { name: 'git.epfl.ch', url: 'https://git.epfl.ch/team/app/-/issues/9', number: '9' };
  const [s] = forgeStates([epfl], () => 0, { bridges: [{ name: 'EPFL', target: 'gitlab', host: 'git.epfl.ch' }], job: null, check: check({ EPFL: tally({ from: 1 }) }) });
  assert.equal(s!.state.kind, 'gitlab');
  assert.equal(s!.state.incoming, 1);
  assert.equal(forgeStates([epfl], () => 0, { bridges, job: null, check: null })[0]!.state.kind, 'other');
});

test('a check holds until a real run of the board’s is done after it, by the server’s clock', () => {
  const at = { ended: '2026-10-30T10:05:00Z' };
  const ended = (when: string, o: { dry?: boolean | undefined; state?: 'running' | 'done' | 'failed' | undefined } = {}) => job({ dry: o.dry, state: o.state, ended: when });
  assert.equal(checkHolds(undefined, null), false);
  assert.equal(checkHolds(at, null), true);
  assert.equal(checkHolds(at, ended('2026-10-30T10:00:00Z')), true); // a run before it
  assert.equal(checkHolds(at, ended('2026-10-30T10:10:00Z')), false); // a whole sync after it
  assert.equal(checkHolds(at, ended('2026-10-30T10:10:00Z', { dry: true })), true); // a check of the whole board writes nothing
  assert.equal(checkHolds(at, ended('2026-10-30T10:10:00Z', { state: 'failed' })), true); // part of the way, or none
  assert.equal(checkHolds(at, job({ state: 'running' })), true);
});
