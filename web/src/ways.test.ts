import { test } from 'node:test';
import assert from 'node:assert/strict';
import { aheadWords, checkWords, doneWords, faviconDot, pendingWords, runWords, stepLine, stepWords, waitWords, waiting } from './ways.ts';

const dir = (o: Partial<Record<'issues' | 'changed' | 'comments' | 'edits' | 'changes', number>> = {}) => ({ issues: 0, changed: 0, comments: 0, edits: 0, changes: 0, ...o });
const tally = (to = dir(), from = dir()) => ({ to, from, linked: 0, stays_local: 0, conflicts: 0, assignable: 0, warnings: 0, errors: 0, lines: [] });
const remote = (send: number, receive: number, people = { send: 0, receive: 0 }, tracked = true) => ({
  name: 'origin', url: '/srv/r.git', push_urls: ['/srv/r.git'], tracked, issues: { send, receive }, people,
});

test('aheadWords says what a remote would move', () => {
  assert.equal(aheadWords({ send: 3, receive: 0 }, 'issue'), '3 issues to send · 0 to receive');
  assert.equal(aheadWords({ send: 1, receive: 2 }, 'person', 'people'), '1 person to send · 2 to receive');
});

test('checkWords sums a check over the forges', () => {
  const bridges = { gh: tally(dir({ issues: 1 }), dir({ changed: 2 })), gl: tally(dir({ changed: 1 }), dir({ issues: 1 })) };
  assert.equal(checkWords(bridges), '3 issues to get, 2 to send');
});

test('runWords says one forge’s part of a sync', () => {
  const t = tally(dir({ issues: 2, comments: 3, edits: 1 }), dir({ issues: 1, changed: 1 }));
  assert.equal(runWords(t), 'create 2 issues, post 3 comments, push 1 edit or change, pull 2 issues');
});

test('stepLine says a step of the whole sync and how it went', () => {
  assert.equal(stepLine({ name: 'pull', state: 'done' }, 'origin', true), 'git fetch from origin: done');
  assert.equal(stepLine({ name: 'pull', state: 'failed', note: 'refs/board/seq has numbers the remote hasn’t' }, 'origin', false), 'git pull from origin: failed, refs/board/seq has numbers the remote hasn’t');
  assert.equal(stepLine({ name: 'project', state: 'skipped', note: 'forges failed' }, 'origin', false), 'the GitHub Project: skipped, forges failed');
  assert.equal(stepLine({ name: 'push', state: 'done', note: 'numbered 2 issues' }, 'origin', false), 'git push to origin: done, numbered 2 issues');
});

test('pendingWords leaves out what is zero', () => {
  const r = { checked_at: '', conflicts: [], label_changes: 2, project_writes: 0, warnings: 0, unmatched: 0 };
  assert.equal(pendingWords(r), 'update the labels of 2 issues');
  assert.equal(pendingWords({ ...r, label_changes: 0, project_writes: 1 }), 'write 1 Project field');
});

const job = (o: object) => ({ dry_run: true, mode: 'sync' as const, state: 'done' as const, started: '2026-10-18T09:00:00Z', names: [], bridges: {}, errors: 0, ...o });
const forge = (j: object | null) => ({ bridges: [], job: j && job(j) });

test('waiting: nothing known waits, no dot', () => {
  const w = waiting(null, null, null);
  assert.equal(waitWords(w), 'Nothing waits');
  assert.equal(faviconDot(w), null);
  assert.equal(w.checked, undefined);
});

test('waiting: a remote ahead is something to send', () => {
  const w = waiting([{ ...remote(3, 4), checked: '2026-10-18T08:00:00Z' }], null, null);
  assert.equal(waitWords(w), '3 to send · 4 to get');
  assert.equal(w.send, 3); // the header's Sync count
  assert.equal(faviconDot(w), 'send');
  assert.equal(w.checked, '2026-10-18T08:00:00Z');
});

test('waiting: what a remote holds counts only once it was fetched', () => {
  assert.equal(waitWords(waiting([remote(0, 4)], null, null)), 'Nothing waits');
});

test('waiting: a done check of the forges adds its tallies and its time', () => {
  const bridges = { gh: { ...tally(dir(), dir({ issues: 1, changed: 1 })), conflicts: 1 } };
  const w = waiting([remote(0, 0)], forge({ bridges, ended: '2026-10-18T09:05:00Z' }), null);
  assert.equal(waitWords(w), '2 to get · 1 conflict');
  assert.equal(faviconDot(w), 'get');
  assert.equal(w.checked, '2026-10-18T09:05:00Z');
});

test('waiting: a real run’s tallies are done, not waiting', () => {
  const bridges = { gh: tally(dir({ issues: 5 })) };
  assert.equal(waitWords(waiting(null, forge({ dry_run: false, bridges, ended: '2026-10-18T09:05:00Z' }), null)), 'Nothing waits');
});

test('waiting: the Project’s held conflicts count, and its check’s time', () => {
  const last = { checked_at: '2026-10-18T10:00:00Z', conflicts: [{}, {}], label_changes: 0, project_writes: 0, warnings: 0, unmatched: 0 };
  const w = waiting(null, forge({ ended: '2026-10-18T09:05:00Z' }), { project: 'o/1', write: true, last } as never);
  assert.equal(waitWords(w), '2 conflicts');
  assert.equal(faviconDot(w), null);
  assert.equal(w.checked, '2026-10-18T10:00:00Z');
});

test('waiting: a run going on says so, whatever waits', () => {
  assert.equal(waiting([remote(3, 0)], forge({ state: 'running' }), null).running, true);
});

test('stepWords says where a run is, with a total when the way has one', () => {
  const bridges = [{ name: 'gh', target: 'github' }];
  assert.equal(stepWords({ bridge: 'gh', way: 'push', done: 12, total: 40, issue: '#43 import: users lose their work' }, bridges), 'GitHub (gh) · pushing 12 of 40 · #43 import: users lose their work');
  assert.equal(stepWords({ bridge: 'gh', way: 'pull', done: 3, issue: 'CO2-7 a title' }, bridges), 'GitHub (gh) · pulling 3 · CO2-7 a title');
  assert.equal(stepWords({ bridge: 'gl', way: 'pull', done: 1 }, bridges), 'gl · pulling 1');
});

test('doneWords is a real run’s one line, in its mode', () => {
  const bridges = { gh: tally(dir({ issues: 1 }), dir({ issues: 1, changed: 1 })), gl: tally(dir({ changed: 2 }), dir()) };
  const job = (mode: 'sync' | 'send' | 'all', errors = 0) => ({ dry_run: false, mode, state: 'done' as const, started: '', names: [], bridges, errors });
  assert.equal(doneWords(job('sync')), 'pulled 2 issues, pushed 3');
  assert.equal(doneWords(job('all')), 'pulled 2 issues, pushed 3');
  assert.equal(doneWords(job('send', 1)), 'pushed 3 issues, 1 error');
});
