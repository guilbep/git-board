import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  autoOpens,
  commitsAgo,
  diffCommand,
  editable,
  groupOf,
  lastReviewBy,
  lineIn,
  parseUnified,
  reviewsOf,
  shownRange,
  threadsOf,
  verbOf,
  viewedCount,
} from './review.ts';
import type { FileStat, IssueEvent, Review } from './api/board.ts';

const file = (path: string, added: number, deleted = 0, extra: Partial<FileStat> = {}): FileStat => ({
  path,
  added,
  deleted,
  blob: 'b-' + path,
  ...extra,
});

test('groupOf: generated first, then large over 3000 changed lines', () => {
  assert.equal(groupOf(file('a', 3000)), 'review');
  assert.equal(groupOf(file('a', 3000, 1)), 'large');
  assert.equal(groupOf(file('a', 1, 0, { generated: true })), 'generated');
  assert.equal(groupOf(file('a', 5000, 0, { generated: true })), 'generated');
});

test('autoOpens: files to review under 1000 changed lines', () => {
  assert.equal(autoOpens(file('a', 999)), true);
  assert.equal(autoOpens(file('a', 500, 500)), false);
  assert.equal(autoOpens(file('a', 1, 0, { generated: true })), false);
  assert.equal(autoOpens(file('a', 1, 0, { binary: true })), false);
});

test('diffCommand names the commits compared, and quotes what the shell would split', () => {
  const from = 'a'.repeat(40), to = 'b'.repeat(40);
  assert.equal(diffCommand(from, to, 'a/b.ts'), `git diff ${'a'.repeat(12)} ${'b'.repeat(12)} -- a/b.ts`);
  assert.equal(diffCommand(from, to, "my file's.ts"), `git diff ${'a'.repeat(12)} ${'b'.repeat(12)} -- 'my file'\\''s.ts'`);
});

test('parseUnified numbers both sides, keeps hunk headers, skips the file header', () => {
  const diff = [
    'diff --git a/a.txt b/a.txt',
    'index 1..2 100644',
    '--- a/a.txt',
    '+++ b/a.txt',
    '@@ -2,3 +2,3 @@ func x',
    ' 2',
    '-3',
    '+three',
    ' 4',
    '@@ -9 +9,2 @@',
    ' 9',
    '+10',
    '\\ No newline at end of file',
    '',
  ].join('\n');
  assert.deepEqual(parseUnified(diff), [
    { op: '@', text: '@@ -2,3 +2,3 @@ func x' },
    { op: ' ', old: 2, new: 2, text: '2' },
    { op: '-', old: 3, text: '3' },
    { op: '+', new: 3, text: 'three' },
    { op: ' ', old: 4, new: 4, text: '4' },
    { op: '@', text: '@@ -9 +9,2 @@' },
    { op: ' ', old: 9, new: 9, text: '9' },
    { op: '+', new: 10, text: '10' },
  ]);
  assert.deepEqual(parseUnified('diff --git a/x b/x\nsimilarity index 100%\nrename from x\nrename to y\n'), []);
});

const person = (id: string) => ({ id, name: id });
const ev = (id: string, who: string, review?: Review, body = ''): IssueEvent => ({
  kind: 'comment',
  id,
  author: person(who),
  at: '2026-10-18T10:00:00Z',
  body,
  ...(review && { review }),
});

test('threadsOf puts replies under their first comment, resolved from it', () => {
  const anchor = { path: 'a.txt', line: 3, side: 'new' as const, commit: 'c1' };
  const events = [
    ev('r1', 'ann', { ...anchor, resolved: true }),
    ev('plain', 'bob'),
    ev('s1', 'bob', { state: 'APPROVE', commit: 'c1' }),
    ev('r2', 'bob', { ...anchor, reply_to: 'r1' }),
    ev('lost', 'bob', { ...anchor, reply_to: 'gone' }),
  ];
  const th = threadsOf(events);
  assert.equal(th.length, 2);
  assert.equal(th[0]!.root.id, 'r1');
  assert.deepEqual(th[0]!.replies.map((e) => e.id), ['r2']);
  assert.equal(th[0]!.resolved, true);
  // a reply whose first comment isn't here stands alone
  assert.equal(th[1]!.root.id, 'lost');
  assert.deepEqual(reviewsOf(events).map((e) => e.id), ['s1']);
  assert.equal(lastReviewBy(events, 'bob')?.id, 's1');
  assert.equal(lastReviewBy(events, 'ann'), undefined);
});

test('lineIn: the head line when the view ends at head, the written line at its commit, else none', () => {
  const r: Review = { path: 'a', line: 3, side: 'new', commit: 'c1', head_line: 5 };
  assert.equal(lineIn(r, 'head', 'head'), 5);
  assert.equal(lineIn({ ...r, outdated: true }, 'head', 'head'), null);
  assert.equal(lineIn({ ...r, head_line: 0 }, 'head', 'head'), null); // not annotated: the commit isn't here
  assert.equal(lineIn(r, 'c1', 'head'), 3);
  assert.equal(lineIn(r, 'c0', 'head'), null);
});

test('commitsAgo counts in the newest-first list', () => {
  const commits = [{ sha: 'c3' }, { sha: 'c2' }, { sha: 'c1' }];
  assert.equal(commitsAgo('c3', commits), 0);
  assert.equal(commitsAgo('c1', commits), 2);
  assert.equal(commitsAgo('c0', commits), null);
});

test('viewedCount: a mark counts while the file is the blob it was viewed at; generated left out', () => {
  const files = [file('a', 1), file('b', 1), file('g', 1, 0, { generated: true })];
  assert.deepEqual(viewedCount(files, {}), [0, 2]);
  assert.deepEqual(viewedCount(files, { a: 'b-a', b: 'old-blob', g: 'b-g' }), [1, 2]);
});

test('threadsOf, reviewsOf and lastReviewBy keep a branch’s own, and what names none', () => {
  const anchor = { path: 'a.txt', line: 3, side: 'new' as const, commit: 'c1' };
  const events = [
    ev('here', 'ann', { ...anchor, branch: 'fix/1-x' }),
    ev('there', 'ann', { ...anchor, branch: '1-other' }),
    ev('nowhere', 'ann', anchor),
    ev('ok-here', 'bob', { state: 'APPROVE', commit: 'c1', branch: 'fix/1-x' }),
    ev('ok-there', 'bob', { state: 'APPROVE', commit: 'o1', branch: '1-other' }),
  ];
  assert.deepEqual(threadsOf(events, 'fix/1-x').map((t) => t.root.id), ['here', 'nowhere']);
  assert.deepEqual(reviewsOf(events, 'fix/1-x').map((e) => e.id), ['ok-here']);
  assert.equal(lastReviewBy(events, 'bob', 'fix/1-x')?.id, 'ok-here');
  assert.equal(threadsOf(events).length, 3);
});

test('editable: a review comment goes to the pull request, its edits never do', () => {
  assert.equal(editable(ev('plain', 'ann')), true);
  assert.equal(editable(ev('line', 'ann', { path: 'a', line: 1, side: 'new', commit: 'c1' })), false);
  assert.equal(editable(ev('pr', 'ann', { pr: 'github.com/o/r#26' })), false);
});

test('verbOf names a pull request’s own comment', () => {
  assert.equal(verbOf(ev('plain', 'ann')), 'commented');
  assert.equal(verbOf(ev('pr', 'ann', { pr: 'github.com/o/r#26' })), 'commented on pull request #26');
  assert.equal(verbOf(ev('ok', 'ann', { state: 'APPROVE', commit: 'c1', pr: 'github.com/o/r#26' })), 'approved');
  assert.equal(verbOf({ ...ev('d', 'ann'), kind: 'created' }), 'opened');
});

test('shownRange: one commit against its parent, the list holding a merge’s other side too', () => {
  const c = { base: 'b', head: 'm', commits: [{ sha: 'm' }, { sha: 'd' }, { sha: 'c1' }] };
  assert.deepEqual(shownRange(c, 'all'), { from: 'b', to: 'm' });
  assert.deepEqual(shownRange(c, 'mine', 'c1'), { from: 'c1', to: 'm' });
  assert.deepEqual(shownRange(c, 'mine'), { from: 'b', to: 'm' });
  assert.deepEqual(shownRange(c, 'm'), { from: 'm^', to: 'm' });
});
