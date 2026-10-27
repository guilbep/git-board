import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { CycleStages, IssueEvent } from './api/board.ts';
import { bodyThreads, carries, resolvedBy, divider, firstNew, fold, isResolved, isRoot, items, pick, READING, readingOf, roleOf, rootOf, segments, type Segment } from './flow.ts';

const stages: CycleStages = {
  before: ['backlog'], ready: 'ready', in_progress: ['in-progress'], in_review: ['in-review'],
  validation: [], closing: 'done', criteria: [], kinds: {}, guessed: false,
};
const me = { id: 'me', name: 'Me' };
const you = { id: 'you', name: 'You' };
const ev = (id: string, at: number, extra: Partial<IssueEvent> = {}) =>
  ({ id, kind: 'comment', author: you, at: new Date(Date.UTC(2026, 9, 21, at)).toISOString(), ...extra }) as IssueEvent;
const step = (n: number) => ({ meta: { 'cycle:step': String(n) } });
const ids = (segs: Segment[]) => segs.map((s): [number, string[]] => [s.n, s.entries.map((e) => e.id)]);

const events = [
  ev('created', 0, { kind: 'created' }),
  ev('c1', 1),
  ev('l1', 2, { kind: 'labels', added: ['status:ready'] }),
  ev('c2', 3, step(2)),
  ev('l2', 4, { kind: 'labels', added: ['risk:structural'] }),
  ev('c3', 5, step(3)),
  ev('l3', 6, { kind: 'labels', added: ['status:in-progress'], removed: ['status:ready'] }),
  ev('l4', 7, { kind: 'labels', added: ['status:in-review'], removed: ['status:in-progress'] }),
  ev('r1', 8, { review: { path: 'a.go', line: 3 } }),
  ev('c6', 9),
];

test('segments, one per step opened', () => {
  assert.deepEqual(ids(segments(events, stages, 6)), [
    [1, ['c1']], [2, ['l1', 'c2']], [3, ['l2', 'c3']], [4, ['l3']], [5, []], [6, ['l4', 'r1', 'c6']],
  ]);
});

test('a comment keeps the step it carries', () => {
  const segs = segments([...events, ev('c7', 10, step(3))], stages, 6);
  assert.deepEqual(segs.find((s) => s.n === 3)!.entries.map((e) => e.id), ['l2', 'c3', 'c7']);
});

test("after the last status move, the issue's step", () => {
  const segs = segments([...events.slice(0, 7), ev('c8', 10)], stages, 5);
  assert.deepEqual(ids(segs).slice(3), [[4, ['l3']], [5, ['c8']]]);
});

test('low risk at ready: 03 is current', () => {
  const low = [
    ev('c1', 1), ev('l1', 2, { kind: 'labels', added: ['status:ready'] }),
    ev('l2', 3, { kind: 'labels', added: ['risk:low'] }), ev('c2', 4), ev('c3', 5, step(3)),
  ];
  assert.deepEqual(ids(segments(low, stages, 3, [3])), [[1, ['c1']], [2, ['l1']], [3, ['l2', 'c2', 'c3']]]);
});

test('a cycle:step outside 1 to 7 is no step', () => {
  const bad = ['verify', '0', '9', '3.5'].map((v, i) => ev(`b${i}`, 10 + i, { meta: { 'cycle:step': v } }));
  assert.deepEqual(ids(segments([...events, ...bad], stages, 6)).slice(-1), [[6, ['l4', 'r1', 'c6', 'b0', 'b1', 'b2', 'b3']]]);
  assert.equal(segments([...events, ...bad], stages, 6).length, 6);
});

test('the current segment shows empty', () => {
  assert.deepEqual(ids(segments([events[0]!], stages, 1)), [[1, []]]);
});

test('a doc issue skips 4', () => {
  const segs = segments([...events.slice(0, 7), ev('c9', 10)], stages, 5, [4]);
  assert.deepEqual(ids(segs).slice(3), [[5, ['l3', 'c9']]]);
});

test('sent back from Verify', () => {
  const back = [...events, ev('c10', 10, step(7)), ev('l5', 11, { kind: 'labels', added: ['status:in-progress'], removed: ['status:in-review'] })];
  const segs = segments(back, stages, 4);
  assert.deepEqual(segs.map((s) => s.n), [1, 2, 3, 4, 5, 6, 7]);
  assert.deepEqual(segs.find((s) => s.n === 4)!.entries.map((e) => e.id), ['l3', 'l5']);
  assert.deepEqual(segs.find((s) => s.n === 7)!.entries.map((e) => e.id), ['c10']);
});

test('no cycle', () => {
  assert.deepEqual(ids(segments(events, undefined, 0)), [[0, events.slice(1).map((e) => e.id)]]);
});

test('closing the issue is step 7', () => {
  const segs = segments([...events, ev('s1', 10, { kind: 'status', status: 'closed' })], stages, 7);
  assert.deepEqual(ids(segs).at(-1), [7, ['s1']]);
});

test('the filters', () => {
  const thread = [
    ev('r2', 10, { review: { path: 'b.go', line: 1, resolved: true } }),
    ev('r3', 11, { review: { path: 'b.go', reply_to: 'r2' } }),
    ev('r4', 12, { review: { path: 'a.go', reply_to: 'r1' }, author: me }),
    ev('a1', 13, { meta: { 'ai:model': 'claude' } }),
  ];
  const all = [...events, ...thread];
  const segs = segments(all, stages, 6);
  const shown = (r: Partial<typeof READING>) => ids(pick(segs, all, { ...READING, ...r }, 'me', 6)).filter(([, e]) => e.length);
  assert.deepEqual(shown({ show: 'step' }), [[6, ['l4', 'r1', 'c6', 'r2', 'r3', 'r4', 'a1']]]);
  assert.deepEqual(ids(pick(segs, all, { ...READING, show: 'step' }, 'me', 6)).length, 1);
  assert.deepEqual(shown({ show: 'open' }), [[6, ['r1', 'r4']]]);
  assert.deepEqual(shown({ mine: true }), [[6, ['r4']]]);
  assert.deepEqual(shown({ agent: true }), [[6, ['a1']]]);
  assert.deepEqual(shown({ milestones: true }), [[2, ['l1']], [3, ['l2']], [4, ['l3']], [6, ['l4']]]);
  assert.deepEqual(shown({ mine: true, agent: true }), [[6, ['r4', 'a1']]]);
  assert.deepEqual(ids(pick(segs, all, { ...READING, newest: true }, 'me', 6)).slice(0, 2), [
    [6, ['a1', 'r4', 'r3', 'r2', 'c6', 'r1', 'l4']], [5, []],
  ]);
  assert.equal(pick(segs, all, READING, 'me', 6).length, 6);
});

test('a reply without its root stays', () => {
  const reply = ev('r4', 12, { review: { path: 'a.go', reply_to: 'r1' } });
  assert.deepEqual(fold([reply]), [{ event: reply, replies: [] }]);
});

test('threads fold their replies', () => {
  const replies = [ev('r4', 12, { review: { path: 'a.go', reply_to: 'r1' } }), ev('r5', 13, { review: { path: 'a.go', reply_to: 'r1' } })];
  const items = fold([events[8]!, events[9]!, ...replies]);
  assert.deepEqual(items.map((i) => [i.event.id, i.replies.map((r) => r.id), i.resolved]), [['r1', ['r4', 'r5'], false], ['c6', [], undefined]]);
});

test('Newest first keeps the threads', () => {
  const all = [...events, ev('r4', 12, { review: { path: 'a.go', reply_to: 'r1' } }), ev('r5', 13, { review: { path: 'a.go', reply_to: 'r1' } })];
  const seg = pick(segments(all, stages, 6), all, { ...READING, newest: true }, 'me', 6)[0]!;
  assert.deepEqual(items(seg, true).map((i) => [i.event.id, i.replies.map((r) => r.id)]), [['c6', []], ['r1', ['r5', 'r4']], ['l4', []]]);
});

test('the divider', () => {
  const days = new Intl.DateTimeFormat(undefined, { day: 'numeric', month: 'short' });
  const d = (h: number) => new Date(Date.UTC(2026, 9, 21, h));
  const two = { n: 1, entries: [ev('a', 1), ev('b', 30)] };
  assert.equal(divider(two, false), `01 Intent · ${days.formatRange(d(1), d(30))} · 2 entries`);
  assert.equal(divider({ n: 2, entries: [ev('a', 1)] }, false), `02 Risk · ${days.format(d(1))} · 1 entry`);
  assert.equal(divider({ n: 7, entries: [ev('a', 1)] }, true), `07 Verify · ${days.format(d(1))} · current`);
  assert.equal(divider({ n: 7, entries: [] }, true), '07 Verify · current');
  assert.equal(divider({ n: 5, entries: [] }, false), '05 Code');
});

test('new since the last visit', () => {
  const segs = segments(events, stages, 6);
  const at = (id: string) => Date.parse(events.find((e) => e.id === id)!.at);
  assert.equal(firstNew(segs, null, 'me'), undefined);
  assert.equal(firstNew(segs, at('l3'), 'me'), 'l4');
  assert.equal(firstNew(segs, at('l3'), 'you'), undefined);
  assert.equal(firstNew(segs, at('c6'), 'me'), undefined);
});

test('new in a folded thread: the line goes above the thread', () => {
  const reply = ev('r6', 10, { review: { path: 'a.go', reply_to: 'r1' } });
  const segs = segments([...events, reply], stages, 6);
  const at = (id: string) => Date.parse(events.find((e) => e.id === id)!.at);
  assert.equal(firstNew(segs, at('c6'), 'me'), 'r6');
  assert.equal(firstNew(segs, at('c6'), 'me', true), 'r1');
  assert.equal(firstNew(segs, at('l4'), 'me', true), 'r1');
});

test('the step a new comment carries', () => {
  assert.equal(carries(7), '07 Verify');
  assert.equal(carries(3), '03 Plan');
  assert.equal(carries(0), '');
});

test('the reading kept', () => {
  assert.deepEqual(readingOf(null), READING);
  assert.deepEqual(readingOf('{'), READING);
  assert.deepEqual(readingOf('[]'), READING);
  assert.deepEqual(readingOf('{"show":"open","newest":true,"tab":3}'), { ...READING, show: 'open', newest: true });
  assert.deepEqual(readingOf('{"show":"bogus","threads":"no","mine":true}'), { ...READING, mine: true });
});

// body threads (thread:*): a question on §3 › 3 and its reply
const c7 = ev('c7', 10, { meta: { 'thread:anchor': 'body@v1/expected#3' } });
const c8 = ev('c8', 11, { meta: { 'thread:anchor': 'body@v1/expected#3', 'thread:reply-to': 'c7' }, author: me });
const resolved = (v: string) => ({ ...c7, meta: { ...c7.meta, 'thread:resolved': v } });
const open = (all: IssueEvent[]) => ids(pick(segments(all, stages, 6), all, { ...READING, show: 'open' }, 'me', 6)).filter(([, e]) => e.length);

test('a body reply folds under its root', () => {
  assert.deepEqual(fold([c7, c8]).map((i) => [i.event.id, i.replies.map((r) => r.id), i.resolved]), [['c7', ['c8'], false]]);
  assert.equal(rootOf(c8), 'c7');
  assert.equal(rootOf(c7), undefined);
  assert.ok(isRoot(c7) && !isRoot(c8) && isRoot(events[8]!) && !isRoot(events[9]!));
  assert.deepEqual(bodyThreads([...events, c7, c8]).map((i) => [i.event.id, i.replies.map((r) => r.id)]), [['c7', ['c8']]]);
});

test('pointer lines keep body comments apart', () => {
  // under their section, the flow lists each body comment on its own, in
  // time order; review threads still fold
  const r4 = ev('r4', 12, { review: { path: 'a.go', reply_to: 'r1' } });
  assert.deepEqual(items({ n: 1, entries: [c7, events[8]!, c8, r4] }, true, true).map((i) => [i.event.id, i.replies.length]), [['c7', 0], ['r1', 1], ['c8', 0]]);
  assert.deepEqual(items({ n: 1, entries: [c7, c8] }, true).map((i) => i.replies.length), [1]);
});

test('a body reply without its root stays', () => {
  assert.deepEqual(fold([c8]), [{ event: c8, replies: [] }]);
});

test('Open threads counts a body thread', () => {
  assert.deepEqual(open([...events, c7, c8]), [[6, ['r1', 'c7', 'c8']]]);
});

test('a resolved body thread is not open', () => {
  assert.ok(isResolved(resolved('v2')));
  assert.deepEqual(open([...events, resolved('v2'), c8]), [[6, ['r1']]]);
});

test('an emptied thread:resolved is open', () => {
  assert.ok(!isResolved(resolved('')));
  assert.deepEqual(open([...events, resolved(''), c8]), [[6, ['r1', 'c7', 'c8']]]);
});

test('pick keeps a thread with no section', () => {
  const gone = ev('c9', 12, { meta: { 'thread:anchor': 'body@v1/renamed' } });
  assert.deepEqual(open([...events, gone]), [[6, ['r1', 'c9']]]);
});

test('who resolved a thread', () => {
  const t = (v: string) => fold([resolved(v), c8])[0]!;
  assert.equal(resolvedBy(t('v2'), [c7, c8]), 'resolved by v2');
  assert.equal(resolvedBy(t('me'), [c7, c8]), 'resolved by Me');
  assert.equal(resolvedBy(t('someone'), [c7, c8]), 'resolved');
  assert.equal(resolvedBy(t(''), [c7, c8]), '');
});

test('readingOf takes where', () => {
  assert.equal(READING.where, 'section');
  assert.equal(readingOf('{"where":"flow"}').where, 'flow');
  assert.equal(readingOf('{"where":"side"}').where, 'section');
});

test('the role a person holds', () => {
  const fields = { po: { single: false, options: null, numeric: false, type: 'person', inherits: 'alice' }, risk: { single: true, options: [], numeric: false } };
  assert.equal(roleOf(fields, 'alice'), 'po');
  assert.equal(roleOf(fields, 'bob'), undefined);
  assert.equal(roleOf(fields, undefined), undefined);
});

// On vN: v2 published at 3; an anchor or spec:on names its version, else the time does
test('On vN keeps its entries and milestone', () => {
  const all = [
    ev('created', 0, { kind: 'created' }),
    ev('c1', 1),
    ev('version-2', 3, { kind: 'version' }),
    ev('version-3', 3, { kind: 'version' }), // v3 in the same second: its own
    ev('c2', 4),
    ev('c3', 5, { meta: { 'thread:anchor': 'body@v1/expected' } }),
    ev('c4', 6, { meta: { 'spec:on': 'body@v1', 'spec:verdict': 'approve' } }),
  ];
  const at = (h: number) => new Date(Date.UTC(2026, 9, 21, h)).toISOString();
  const on = (v: number, from: number, to?: number) => ids(pick(segments(all, undefined, 0), all, { ...READING, show: 'version' }, 'me', 0, { v, from: at(from), ...(to ? { to: at(to) } : {}) }));
  assert.deepEqual(on(2, 3), [[0, ['version-2', 'c2']]]);
  assert.deepEqual(on(1, 0, 3), [[0, ['c1', 'c3', 'c4']]]);
});

test('one version reads as all', () => {
  const segs = segments(events, stages, 6);
  assert.deepEqual(pick(segs, events, { ...READING, show: 'version' }, 'me', 6), pick(segs, events, READING, 'me', 6));
});

test('readingOf takes version', () => {
  assert.equal(readingOf('{"show":"version"}').show, 'version');
});

const at = (h: number) => new Date(Date.UTC(2026, 9, 21, h)).toISOString();
test('segments carry when each step opened', () => {
  assert.deepEqual(segments(events, stages, 6).map((s) => [s.n, s.at]), [
    [1, undefined], [2, at(2)], [3, at(4)], [4, at(6)], [5, undefined], [6, at(7)],
  ]);
});

test('a late carried comment keeps the bars', () => {
  const segs = segments([...events.slice(0, 7), ev('c9', 8, step(1))], stages, 4);
  assert.deepEqual(segs.map((s) => [s.n, s.at]), [[1, undefined], [2, at(2)], [3, at(4)], [4, at(6)]]);
  // a carried step above the running one opens it
  assert.equal(segments([...events.slice(0, 2), ev('c9', 3, step(2))], stages, 2)[1]!.at, at(3));
});

test('readingOf takes changes', () => {
  assert.equal(readingOf(JSON.stringify({ tab: 'changes' })).tab, 'changes');
});
