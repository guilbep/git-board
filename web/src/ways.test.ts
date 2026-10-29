import { test } from 'node:test';
import assert from 'node:assert/strict';
import { aheadWords, checkWords, cutWords, doneWords, faviconDot, longPull, pendingWords, readProject, runWords, stepLine, stepWords, timeLeft, waitWords, waiting } from './ways.ts';

const dir = (o: Partial<Record<'issues' | 'changed' | 'comments' | 'edits' | 'changes', number>> = {}) => ({ issues: 0, changed: 0, comments: 0, edits: 0, changes: 0, ...o });
const tally = (to = dir(), from = dir()) => ({ to, from, linked: 0, stays_local: 0, conflicts: 0, assignable: 0, warnings: 0, errors: 0, held: 0, lines: [] });
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
  const r = { checked_at: '', title: '', conflicts: [], label_changes: 2, project_writes: 0, warnings: 0, unmatched: 0 };
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
  const job = (mode: 'sync' | 'send' | 'get' | 'all', errors = 0) => ({ dry_run: false, mode, state: 'done' as const, started: '', names: [], bridges, errors });
  assert.equal(doneWords(job('sync')), 'pulled 2 issues, pushed 3');
  assert.equal(doneWords(job('all')), 'pulled 2 issues, pushed 3');
  assert.equal(doneWords(job('send', 1)), 'pushed 3 issues, 1 error');
  assert.equal(doneWords(job('get')), 'pulled 2 issues');
});

test('readProject reads a number, as the forge owner\'s Project, or a Project\'s address', () => {
  const p = (owner: string, number: number) => ({ owner, number });
  assert.deepEqual(readProject('40', 'EPFL-ENAC'), p('EPFL-ENAC', 40));
  assert.deepEqual(readProject(' 40 ', 'EPFL-ENAC'), p('EPFL-ENAC', 40));
  assert.deepEqual(readProject('https://github.com/orgs/EPFL-ENAC/projects/40', ''), p('EPFL-ENAC', 40));
  assert.deepEqual(readProject('https://github.com/orgs/EPFL-ENAC/projects/40/views/2', 'other'), p('EPFL-ENAC', 40));
  assert.deepEqual(readProject('https://github.com/users/me/projects/3/', ''), p('me', 3));
  assert.deepEqual(readProject('https://github.com/users/me/projects/3/views/1?filterQuery=is%3Aopen', ''), p('me', 3));

  assert.equal(readProject('', 'EPFL-ENAC'), null);
  assert.equal(readProject('40', ''), null); // no GitHub forge to take the owner from
  for (const bad of ['0', '-1', '4.5', '40a', '1e3']) assert.equal(readProject(bad, 'EPFL-ENAC'), null, bad);
  assert.equal(readProject('https://github.com/EPFL-ENAC/wiki', ''), null); // a repository
  assert.equal(readProject('https://github.com/orgs/EPFL-ENAC/projects/', ''), null);
  assert.equal(readProject('https://gitlab.com/orgs/EPFL-ENAC/projects/40', ''), null);
  assert.equal(readProject('https://github.com/orgs/-bad/projects/40', ''), null); // not a GitHub name
  assert.equal(readProject('https://github.com/orgs/a--b/projects/40', ''), null);
});

test('timeLeft says nothing before 20 issues, then the rest at the rate so far', () => {
  const first = { at: 0, done: 1 };
  assert.equal(timeLeft(first, { at: 5_000, done: 15 }, 1034), '');
  assert.equal(timeLeft(first, { at: 21_000, done: 22 }, 1034), 'about 17 min left'); // 1 s an issue, 1012 left
  assert.equal(timeLeft(first, { at: 21_000, done: 22 }, 100), 'about a minute left'); // 78 s
  assert.equal(timeLeft(first, { at: 21_000, done: 22 }, 60), 'less than a minute left');
  assert.equal(timeLeft(first, { at: 21_000, done: 60 }, 60), '');
});

test('longPull: a pull with 200 issues or more left, or over a minute', () => {
  assert.equal(longPull({ bridge: 'gh', way: 'pull', done: 1, total: 1034 }, 0), true);
  assert.equal(longPull({ bridge: 'gh', way: 'pull', done: 1, total: 150 }, 0), false);
  assert.equal(longPull({ bridge: 'gh', way: 'pull', done: 30, total: 150 }, 90_000), true);
  assert.equal(longPull({ bridge: 'gh', way: 'push', done: 1, total: 1034 }, 0), false);
  assert.equal(longPull({ bridge: 'gh', way: 'pull', done: 3 }, 0), false);
});

test('cutWords says where a cut sync stopped, and that the next goes on', () => {
  const bridges = [{ name: 'gh', target: 'github' }];
  const cut = {
    at: '2026-10-29T14:02:00Z',
    forges: [
      { name: 'gh', key: 'github.com/o/r', read: 612, total: 1034 },
      { key: 'gitlab.example.com/g/p', read: 0, total: 20 },
    ],
  };
  assert.equal(
    cutWords(cut, bridges, 'today 14:02'),
    'The last sync stopped today 14:02, at 612 of 1,034 issues from GitHub (gh), 0 of 20 from gitlab.example.com/g/p. The next sync goes on from there: what it pulled stays.',
  );
});
