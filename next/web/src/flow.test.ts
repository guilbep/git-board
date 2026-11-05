import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { CycleStages, IssueEvent } from './api/board.ts';
import { STEPS } from './cycle.ts';
import { bodyThreads, carries, resolvedBy, divider, firstNew, fold, idOf, isResolved, isRoot, items, pick, READING, readingOf, roleOf, rootOf, segments, stepOf, type Segment } from './flow.ts';

const stages: CycleStages = {
  before: ['backlog'], ready: 'ready', in_progress: ['in-progress'], in_review: ['in-review'],
  validation: [], closing: 'done', criteria: [], kinds: {}, guessed: false,
};
const me = { id: 'me', name: 'Me' };
const you = { id: 'you', name: 'You' };
const ev = (id: string, at: number, extra: Partial<IssueEvent> = {}) =>
  ({ id, kind: 'comment', author: you, at: new Date(Date.UTC(2026, 9, 21, at)).toISOString(), ...extra }) as IssueEvent;
const step = (id: string) => ({ meta: { 'cycle:step': id } });
const ids = (segs: Segment[]) => segs.map((s): [string, string[]] => [s.id, s.entries.map((e) => e.id)]);
// a segment of the default list, for the divider and items
const seg = (n: number, entries: IssueEvent[]): Segment => ({ id: '', n, name: STEPS[n]!, entries });

const events = [
  ev('created', 0, { kind: 'created' }),
  ev('c1', 1),
  ev('l1', 2, { kind: 'labels', added: ['status:ready'] }),
  ev('c2', 3, step('risk')),
  ev('l2', 4, { kind: 'labels', added: ['risk:structural'] }),
  ev('c3', 5, step('plan')),
  ev('l3', 6, { kind: 'labels', added: ['status:in-progress'], removed: ['status:ready'] }),
  ev('l4', 7, { kind: 'labels', added: ['status:in-review'], removed: ['status:in-progress'] }),
  ev('r1', 8, { review: { path: 'a.go', line: 3 } }),
  ev('c6', 9),
];

test('segments, one per step opened', () => {
  assert.deepEqual(ids(segments(events, stages, 'review')), [
    ['intent', ['c1']], ['risk', ['l1', 'c2']], ['plan', ['l2', 'c3']], ['tests', ['l3']], ['code', []], ['review', ['l4', 'r1', 'c6']],
  ]);
});

test('a comment keeps the step it carries', () => {
  const segs = segments([...events, ev('c7', 10, step('plan'))], stages, 'review');
  assert.deepEqual(segs.find((s) => s.id === 'plan')!.entries.map((e) => e.id), ['l2', 'c3', 'c7']);
});

test("after the last status move, the issue's step", () => {
  const segs = segments([...events.slice(0, 7), ev('c8', 10)], stages, 'code');
  assert.deepEqual(ids(segs).slice(3), [['tests', ['l3']], ['code', ['c8']]]);
});

test('low risk at ready: 03 is current', () => {
  const low = [
    ev('c1', 1), ev('l1', 2, { kind: 'labels', added: ['status:ready'] }),
    ev('l2', 3, { kind: 'labels', added: ['risk:low'] }), ev('c2', 4), ev('c3', 5, step('plan')),
  ];
  assert.deepEqual(ids(segments(low, stages, 'plan', [3])), [['intent', ['c1']], ['risk', ['l1']], ['plan', ['l2', 'c2', 'c3']]]);
});

test('a cycle:step the list lacks is no step', () => {
  const bad = ['deploy', '0', '9', '3.5'].map((v, i) => ev(`b${i}`, 10 + i, { meta: { 'cycle:step': v } }));
  assert.deepEqual(ids(segments([...events, ...bad], stages, 'review')).slice(-1), [['review', ['l4', 'r1', 'c6', 'b0', 'b1', 'b2', 'b3']]]);
  assert.equal(segments([...events, ...bad], stages, 'review').length, 6);
  // verify is the default list's: a comment carrying it opens its segment
  assert.deepEqual(ids(segments([...events, ev('b', 10, step('verify'))], stages, 'review')).at(-1), ['verify', ['b']]);
});

test('the current segment shows empty', () => {
  assert.deepEqual(ids(segments([events[0]!], stages, 'intent')), [['intent', []]]);
});

test('a doc issue skips 4', () => {
  const segs = segments([...events.slice(0, 7), ev('c9', 10)], stages, 'code', [4]);
  assert.deepEqual(ids(segs).slice(3), [['code', ['l3', 'c9']]]);
});

test('sent back from Verify', () => {
  const back = [...events, ev('c10', 10, step('verify')), ev('l5', 11, { kind: 'labels', added: ['status:in-progress'], removed: ['status:in-review'] })];
  const segs = segments(back, stages, 'tests');
  assert.deepEqual(segs.map((s) => s.n), [1, 2, 3, 4, 5, 6, 7]);
  assert.deepEqual(segs.find((s) => s.id === 'tests')!.entries.map((e) => e.id), ['l3', 'l5']);
  assert.deepEqual(segs.find((s) => s.id === 'verify')!.entries.map((e) => e.id), ['c10']);
});

test('no cycle', () => {
  assert.deepEqual(ids(segments(events, undefined, '')), [['', events.slice(1).map((e) => e.id)]]);
});

test('closing the issue is the last step', () => {
  const segs = segments([...events, ev('s1', 10, { kind: 'status', status: 'closed' })], stages, 'verify');
  assert.deepEqual(ids(segs).at(-1), ['verify', ['s1']]);
});

test('the filters', () => {
  const thread = [
    ev('r2', 10, { review: { path: 'b.go', line: 1, resolved: true } }),
    ev('r3', 11, { review: { path: 'b.go', reply_to: 'r2' } }),
    ev('r4', 12, { review: { path: 'a.go', reply_to: 'r1' }, author: me }),
    ev('a1', 13, { meta: { 'ai:model': 'claude' } }),
  ];
  const all = [...events, ...thread];
  const segs = segments(all, stages, 'review');
  const shown = (r: Partial<typeof READING>) => ids(pick(segs, all, { ...READING, ...r }, 'me', 'review')).filter(([, e]) => e.length);
  assert.deepEqual(shown({ show: 'step' }), [['review', ['l4', 'r1', 'c6', 'r2', 'r3', 'r4', 'a1']]]);
  assert.deepEqual(ids(pick(segs, all, { ...READING, show: 'step' }, 'me', 'review')).length, 1);
  assert.deepEqual(shown({ show: 'open' }), [['review', ['r1', 'r4']]]);
  assert.deepEqual(shown({ mine: true }), [['review', ['r4']]]);
  assert.deepEqual(shown({ agent: true }), [['review', ['a1']]]);
  assert.deepEqual(shown({ milestones: true }), [['risk', ['l1']], ['plan', ['l2']], ['tests', ['l3']], ['review', ['l4']]]);
  assert.deepEqual(shown({ mine: true, agent: true }), [['review', ['r4', 'a1']]]);
  assert.deepEqual(ids(pick(segs, all, { ...READING, newest: true }, 'me', 'review')).slice(0, 2), [
    ['review', ['a1', 'r4', 'r3', 'r2', 'c6', 'r1', 'l4']], ['code', []],
  ]);
  assert.equal(pick(segs, all, READING, 'me', 'review').length, 6);
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
  const first = pick(segments(all, stages, 'review'), all, { ...READING, newest: true }, 'me', 'review')[0]!;
  assert.deepEqual(items(first, true).map((i) => [i.event.id, i.replies.map((r) => r.id)]), [['c6', []], ['r1', ['r5', 'r4']], ['l4', []]]);
});

test('the divider', () => {
  const days = new Intl.DateTimeFormat(undefined, { day: 'numeric', month: 'short' });
  const d = (h: number) => new Date(Date.UTC(2026, 9, 21, h));
  assert.equal(divider(seg(1, [ev('a', 1), ev('b', 30)]), false), `01 Intent · ${days.formatRange(d(1), d(30))} · 2 entries`);
  assert.equal(divider(seg(2, [ev('a', 1)]), false), `02 Risk · ${days.format(d(1))} · 1 entry`);
  assert.equal(divider(seg(7, [ev('a', 1)]), true), `07 Verify · ${days.format(d(1))} · current`);
  assert.equal(divider(seg(7, []), true), '07 Verify · current');
  assert.equal(divider(seg(5, []), false), '05 Code');
  assert.equal(divider({ id: 'review', n: 2, name: 'Review', entries: [] }, false), '02 Review');
});

test('new since the last visit', () => {
  const segs = segments(events, stages, 'review');
  const at = (id: string) => Date.parse(events.find((e) => e.id === id)!.at);
  assert.equal(firstNew(segs, null, 'me'), undefined);
  assert.equal(firstNew(segs, at('l3'), 'me'), 'l4');
  assert.equal(firstNew(segs, at('l3'), 'you'), undefined);
  assert.equal(firstNew(segs, at('c6'), 'me'), undefined);
});

test('new in a folded thread: the line goes above the thread', () => {
  const reply = ev('r6', 10, { review: { path: 'a.go', reply_to: 'r1' } });
  const segs = segments([...events, reply], stages, 'review');
  const at = (id: string) => Date.parse(events.find((e) => e.id === id)!.at);
  assert.equal(firstNew(segs, at('c6'), 'me'), 'r6');
  assert.equal(firstNew(segs, at('c6'), 'me', true), 'r1');
  assert.equal(firstNew(segs, at('l4'), 'me', true), 'r1');
});

test('the step a new comment carries', () => {
  assert.equal(carries(seg(7, [])), '07 Verify');
  assert.equal(carries({ n: 2, name: 'Review' }), '02 Review');
  assert.equal(carries(undefined), '');
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
const open = (all: IssueEvent[]) => ids(pick(segments(all, stages, 'review'), all, { ...READING, show: 'open' }, 'me', 'review')).filter(([, e]) => e.length);

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
  assert.deepEqual(items(seg(1, [c7, events[8]!, c8, r4]), true, true).map((i) => [i.event.id, i.replies.length]), [['c7', 0], ['r1', 1], ['c8', 0]]);
  assert.deepEqual(items(seg(1, [c7, c8]), true).map((i) => i.replies.length), [1]);
});

test('a body reply without its root stays', () => {
  assert.deepEqual(fold([c8]), [{ event: c8, replies: [] }]);
});

test('Open threads counts a body thread', () => {
  assert.deepEqual(open([...events, c7, c8]), [['review', ['r1', 'c7', 'c8']]]);
});

test('a resolved body thread is not open', () => {
  assert.ok(isResolved(resolved('v2')));
  assert.deepEqual(open([...events, resolved('v2'), c8]), [['review', ['r1']]]);
});

test('an emptied thread:resolved is open', () => {
  assert.ok(!isResolved(resolved('')));
  assert.deepEqual(open([...events, resolved(''), c8]), [['review', ['r1', 'c7', 'c8']]]);
});

test('pick keeps a thread with no section', () => {
  const gone = ev('c9', 12, { meta: { 'thread:anchor': 'body@v1/renamed' } });
  assert.deepEqual(open([...events, gone]), [['review', ['r1', 'c9']]]);
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
  const on = (v: number, from: number, to?: number) => ids(pick(segments(all, undefined, ''), all, { ...READING, show: 'version' }, 'me', '', { v, from: at(from), ...(to ? { to: at(to) } : {}) }));
  assert.deepEqual(on(2, 3), [['', ['version-2', 'c2']]]);
  assert.deepEqual(on(1, 0, 3), [['', ['c1', 'c3', 'c4']]]);
});

test('one version reads as all', () => {
  const segs = segments(events, stages, 'review');
  assert.deepEqual(pick(segs, events, { ...READING, show: 'version' }, 'me', 'review'), pick(segs, events, READING, 'me', 'review'));
});

test('readingOf takes version', () => {
  assert.equal(readingOf('{"show":"version"}').show, 'version');
});

const at = (h: number) => new Date(Date.UTC(2026, 9, 21, h)).toISOString();
test('segments carry when each step opened', () => {
  assert.deepEqual(segments(events, stages, 'review').map((s) => [s.id, s.at]), [
    ['intent', undefined], ['risk', at(2)], ['plan', at(4)], ['tests', at(6)], ['code', undefined], ['review', at(7)],
  ]);
});

test('a late carried comment keeps the bars', () => {
  const segs = segments([...events.slice(0, 7), ev('c9', 8, step('intent'))], stages, 'tests');
  assert.deepEqual(segs.map((s) => [s.id, s.at]), [['intent', undefined], ['risk', at(2)], ['plan', at(4)], ['tests', at(6)]]);
  // a carried step above the running one opens it
  assert.equal(segments([...events.slice(0, 2), ev('c9', 3, step('risk'))], stages, 'risk')[1]!.at, at(3));
});

test('readingOf takes changes', () => {
  assert.equal(readingOf(JSON.stringify({ tab: 'changes' })).tab, 'changes');
});

// the project's list ([cycle] steps)
const listed = (...ids: string[]): CycleStages => ({
  ...stages, validation: ['qa'],
  steps: ids.map((id) => (id === 'qa' ? { id, kind: 'signoff', name: 'QA', status: 'qa' } : { id, kind: id === 'verify' ? 'signoff' : id, name: id })),
});
const two = listed('code', 'review');

test('"1" to "7" read as the default ids', () => {
  const segs = segments([...events.slice(0, 7), ev('c7', 10, step('6')), ev('c8', 11, step('2'))], stages, 'code');
  assert.deepEqual(segs.find((s) => s.id === 'review')!.entries.map((e) => e.id), ['c7']);
  assert.deepEqual(segs.find((s) => s.id === 'risk')!.entries.map((e) => e.id), ['l1', 'c2', 'c8']);
});

test('two steps, two segments', () => {
  const segs = segments(events, two, 'review');
  assert.deepEqual(ids(segs), [['code', ['c1', 'l1', 'c2', 'l2', 'c3', 'l3']], ['review', ['l4', 'r1', 'c6']]]);
  assert.deepEqual(segs.map((s) => [s.n, s.name, s.at]), [[1, 'code', at(6)], [2, 'review', at(7)]]);
});

test('in progress opens code when tests is not a step', () => {
  assert.deepEqual(ids(segments(events.slice(0, 7), listed('intent', 'code', 'review'), 'code')), [['intent', ['c1', 'l1', 'c2', 'l2', 'c3']], ['code', ['l3']]]);
});

test('an id the list lacks stays where it was written', () => {
  const list = listed('intent', 'risk', 'plan', 'tests', 'code', 'review', 'qa');
  assert.deepEqual(ids(segments([...events, ev('c7', 10, step('7'))], list, 'review')).at(-1), ['review', ['l4', 'r1', 'c6', 'c7']]);
});

test('before the first step, the first segment', () => {
  const early = events.slice(0, 6);
  const segs = segments(early, two, '');
  assert.deepEqual(ids(segs), [['code', ['c1', 'l1', 'c2', 'l2', 'c3']]]);
  assert.deepEqual(ids(pick(segs, early, { ...READING, show: 'step' }, 'me', '')), ids(segs));
});

test("a move to qa opens qa's segment", () => {
  const list = listed('intent', 'risk', 'plan', 'tests', 'code', 'review', 'qa');
  const segs = segments([...events, ev('l5', 10, { kind: 'labels', added: ['status:qa'], removed: ['status:in-review'] }), ev('c7', 11)], list, 'qa');
  assert.deepEqual(ids(segs).at(-1), ['qa', ['l5', 'c7']]);
  assert.equal(segs.at(-1)!.at, at(10));
});

test('idOf', () => {
  assert.deepEqual(['1', '6', '7', 'qa', '0', '9', ''].map(idOf), ['intent', 'review', 'verify', 'qa', '0', '9', '']);
  assert.equal(stepOf(ev('a', 1, step('3'))), 'plan');
  assert.equal(stepOf(ev('a', 1)), '');
});
