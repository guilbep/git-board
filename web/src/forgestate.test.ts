import { test } from 'node:test';
import assert from 'node:assert/strict';
import { forgeKind, forgeState } from './forgestate.ts';

const gitlab = { name: 'GitLab', url: 'https://gitlab.example.org/team/app/-/issues/1', number: '1' };
const github = { name: 'GitHub', url: 'https://github.com/team/app/issues/2771', number: '2771' };

test('the shape from the forge', () => {
  assert.equal(forgeKind(github), 'github');
  assert.equal(forgeKind(gitlab), 'gitlab');
  assert.equal(forgeKind({ name: 'git.example.org', url: 'https://git.example.org/x' }), 'other');
});

test('nothing waiting: the bare icon, in sync', () => {
  assert.deepEqual(forgeState(github, 0), { kind: 'github', badge: null, n: 0, label: 'GitHub #2771: in sync' });
});

test('changes to send: the count, said', () => {
  assert.deepEqual(forgeState(gitlab, 6), { kind: 'gitlab', badge: 'count', n: 6, label: 'GitLab #1: 6 changes wait to be sent' });
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
