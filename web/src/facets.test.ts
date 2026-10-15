// Run with: node --test web/src/facets.test.ts (Node strips the types).
import assert from 'node:assert/strict';
import { test } from 'node:test';
import type { Issue } from './api/board';
import type { Fields } from './fields.ts';
import { NONE, facets, matches, parseSelection, parseSort, pick, sortIssues, sortOptions } from './facets.ts';

const issue = (id: string, title: string, labels: string[], status = 'open'): Issue => ({ id, title, status, labels });

const fields: Fields = {
  status: { single: true, options: ['backlog', 'ready', 'in-progress'], numeric: false },
  est: { single: true, options: ['1', '2', '4'], numeric: true },
  assignee: { single: false, options: null, numeric: false },
};

const issues = [
  issue('a1', 'Login page', ['status:ready', 'est:4', 'assignee:ann', 'sprint:s3', 'bug']),
  issue('b2', 'Export CSV', ['status:backlog', 'est:3.5', 'assignee:bob', 'assignee:ann']),
  issue('c3', 'Fix login', ['status:in-progress', 'sprint:s4'], 'closed'),
  issue('d4', 'Docs', ['est:1']),
];
const ids = (list: Issue[]) => list.map((i) => i.id).join(' ');

test('OR within a facet, AND across facets', () => {
  const sel = { status: ['ready', 'backlog'], assignee: ['bob'] };
  assert.equal(ids(issues.filter((i) => matches(i, sel, ''))), 'b2');
  assert.equal(ids(issues.filter((i) => matches(i, { status: ['ready', 'backlog'] }, ''))), 'a1 b2');
});

test('a multi-valued field matches when any value is selected', () => {
  assert.equal(ids(issues.filter((i) => matches(i, { assignee: ['ann'] }, ''))), 'a1 b2');
});

test('"no value" selects issues without the field', () => {
  assert.equal(ids(issues.filter((i) => matches(i, { sprint: [NONE] }, ''))), 'b2 d4');
  assert.equal(ids(issues.filter((i) => matches(i, { label: [NONE] }, ''))), 'b2 c3 d4');
});

test('search matches the title and the start of the id', () => {
  assert.equal(ids(issues.filter((i) => matches(i, {}, 'LOGIN'))), 'a1 c3');
  assert.equal(ids(issues.filter((i) => matches(i, {}, 'd4'))), 'd4');
});

test('facets: state, configured fields, other namespaces, then plain labels', () => {
  const list = facets(issues, {}, '', fields);
  assert.deepEqual(list.map((f) => f.key), ['is', 'assignee', 'est', 'status', 'sprint', 'label']);
  // configured option order, values outside the options after, "no value" last
  assert.deepEqual(list.find((f) => f.key === 'status')!.options, [
    ['backlog', 1], ['ready', 1], ['in-progress', 1], [NONE, 1],
  ]);
});

test('facet counts follow the other facets, not their own', () => {
  const list = facets(issues, { status: ['ready'], is: ['open'] }, '', fields);
  const count = (key: string) => Object.fromEntries(list.find((f) => f.key === key)!.options);
  assert.deepEqual(count('status'), { backlog: 1, ready: 1, 'in-progress': 0, [NONE]: 1 });
  assert.deepEqual(count('is'), { open: 1, closed: 0 });
});

test('a facet with one value is hidden unless something is selected in it', () => {
  const open = issues.filter((i) => i.status === 'open');
  assert.ok(!facets(open, {}, '', fields).some((f) => f.key === 'is'));
  assert.ok(facets(open, { is: ['open'] }, '', fields).some((f) => f.key === 'is'));
});

test('numeric fields sort as numbers, missing last both ways', () => {
  assert.equal(ids(sortIssues(issues, { key: 'est', desc: false }, fields)), 'd4 b2 a1 c3');
  assert.equal(ids(sortIssues(issues, { key: 'est', desc: true }, fields)), 'a1 b2 d4 c3');
});

test('option fields sort by their configured order, missing last both ways', () => {
  assert.equal(ids(sortIssues(issues, { key: 'status', desc: false }, fields)), 'b2 a1 c3 d4');
  assert.equal(ids(sortIssues(issues, { key: 'status', desc: true }, fields)), 'c3 a1 b2 d4');
});

test('title, id and state sort too', () => {
  assert.equal(ids(sortIssues(issues, { key: 'title', desc: false }, fields)), 'd4 b2 c3 a1');
  assert.equal(ids(sortIssues(issues, { key: 'is', desc: true }, fields)), 'a1 b2 d4 c3');
});

test('sort specs: the URL form and the view form', () => {
  assert.deepEqual(parseSort('-prio'), { key: 'prio', desc: true });
  assert.deepEqual(parseSort('prio desc'), { key: 'prio', desc: true });
  assert.deepEqual(parseSort('prio asc'), { key: 'prio', desc: false });
  assert.equal(parseSort(''), null);
});

test('the selection lives under f.<facet> query keys, one per value', () => {
  assert.deepEqual(parseSelection({ view: 'x', 'f.status': ['ready', 'qa'], 'f.is': 'open', sort: 'prio' }), {
    status: ['ready', 'qa'],
    is: ['open'],
  });
});

test('a section sorts its options by count, alphabetically or in workflow order, (none) last', () => {
  const opts: [string, number][] = [['ready', 2], [NONE, 9], ['backlog', 5], ['qa', 5]];
  const show = (mode: 'count' | 'alpha' | 'workflow') => sortOptions(opts, mode, ['backlog', 'ready', 'qa']).map(([v]) => v).join(' ');
  assert.equal(show('count'), `backlog qa ready ${NONE}`);
  assert.equal(show('alpha'), `backlog qa ready ${NONE}`);
  assert.equal(show('workflow'), `backlog ready qa ${NONE}`);
  assert.equal(sortOptions([['b', 1], ['a10', 1], ['a9', 1]], 'alpha', []).map(([v]) => v).join(' '), 'a9 a10 b');
});

test('pick: a click toggles, Alt keeps only one then all but it, Shift a range', () => {
  const order = ['a', 'b', 'c', 'd', 'e'];
  assert.deepEqual(pick(['a'], 'c', order, {}), ['a', 'c']);
  assert.deepEqual(pick(['a', 'c'], 'c', order, {}), ['a']);
  assert.deepEqual(pick(['a', 'c'], 'd', order, { alt: true }), ['d']);
  assert.deepEqual(pick(['d'], 'd', order, { alt: true }), ['a', 'b', 'c', 'e']);
  // shift from the last clicked: the range takes the clicked box's new state
  assert.deepEqual(pick(['a'], 'd', order, { shift: true, anchor: 'b' }), ['a', 'b', 'c', 'd']);
  assert.deepEqual(pick(['a', 'b', 'c', 'd'], 'c', order, { shift: true, anchor: 'a' }), ['d']);
  assert.deepEqual(pick([], 'c', order, { shift: true }), ['c']); // no anchor: a plain toggle
});

test('sortIssues sorts by forge number, issues without one last', () => {
  const withForge = (id: string, n?: string): Issue => ({ ...issue(id, id, []), forges: n ? [{ name: 'GitHub', url: `https://github.com/o/r/issues/${n}`, number: n }] : [] });
  const sorted = sortIssues([withForge('a', '1436'), withForge('b'), withForge('c', '87'), withForge('d', '2528')], { key: 'links', desc: false }, {});
  assert.deepEqual(sorted.map((i) => i.id), ['c', 'a', 'd', 'b']);
  const desc = sortIssues([withForge('a', '1436'), withForge('b'), withForge('c', '87')], { key: 'links', desc: true }, {});
  assert.deepEqual(desc.map((i) => i.id), ['a', 'c', 'b']);
});
