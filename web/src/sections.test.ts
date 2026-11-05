import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { IssueEvent } from './api/types.ts';
import {
  anchorLabel, anchorOf, criteriaOf, isTick, sectionsOf, slugs, stateOf, version, versions,
} from './sections.ts';

// 6a's description, as Markdown and as the server renders it
const md = [
  'Seen in the clone of the git-bug fork while trying to cut v0.11.1-sync.9.',
  '',
  '## Releases page',
  '',
  '- The header reads ". · published on GitHub", with no repository name.',
  '- "All releases" lists only upstream\'s tags.',
  '- The cut stopped with git-bug\'s CLI text about a missing identity.',
  '',
  '## Expected',
  '',
  '- [ ] The authority remote is chosen, not assumed.',
  '- [ ] Pre-release tags count.',
  '- [ ] A missing identity sends the user to Settings › identity, not to a CLI command.',
  '- [ ] The header names the repository.',
  '',
  '## Workaround',
  '',
  'Not tried yet: [release] remote = "fork".',
].join('\n');
const html = '<p>Seen in the clone.</p>\n<h2>Releases page</h2>\n<ul>\n<li>a</li>\n<li>b</li>\n<li>c</li>\n</ul>\n' +
  '<h2>Expected</h2>\n<ul>\n<li>1</li>\n<li>2</li>\n<li>3</li>\n<li>4</li>\n</ul>\n<h2>Workaround</h2>\n<p>Not tried yet.</p>\n';
const secs = sectionsOf(html);
const at = (s: string) => anchorOf(s)!;

test('no heading is one section', () => {
  const s = sectionsOf('<p>a</p><p>b</p>');
  assert.deepEqual(s.map((x) => [x.n, x.slug, x.heading, x.html]), [[1, 'intro', '', '<p>a</p><p>b</p>']]);
});

test('each heading opens a section', () => {
  assert.deepEqual(secs.map((s) => [s.n, s.slug, s.heading]), [
    [1, 'intro', ''], [2, 'releases-page', 'Releases page'], [3, 'expected', 'Expected'], [4, 'workaround', 'Workaround'],
  ]);
  assert.ok(secs[2]!.html.startsWith('<h2>Expected</h2>'));
  assert.deepEqual(sectionsOf('<h1>Only</h1><p>x</p>').map((s) => [s.n, s.slug]), [[1, 'only']]);
});

test('a repeated heading gets an index', () => {
  assert.deepEqual(slugs(['Expected', 'Expected', 'Releases page', 'Expected']), ['expected', 'expected-2', 'releases-page', 'expected-3']);
});

test('anchorOf parses the grammar', () => {
  assert.deepEqual(anchorOf('body@v1/expected#3'), { text: 'body', v: 1, slug: 'expected', item: 3 });
  assert.deepEqual(anchorOf('body@v2/intro'), { text: 'body', v: 2, slug: 'intro' });
  assert.equal(anchorOf('plan@a1f0/steps#4'), undefined);
  assert.equal(anchorOf('body@v0/x'), undefined);
  assert.equal(anchorOf(undefined), undefined);
});

test('anchorLabel', () => {
  assert.equal(anchorLabel(at('body@v1/expected#3'), secs), '§3 › 3');
  assert.equal(anchorLabel(at('body@v1/expected'), secs), '§3');
  // an unknown slug keeps its name (the heading was renamed since)
  assert.equal(anchorLabel(at('body@v1/gone#2'), secs), '§ gone › 2');
});

test('a tick is not a version', () => {
  const h = (...bodies: string[]) => ({ kind: 'created', history: bodies.map((body) => ({ author: { id: 'a', name: 'A' }, at: '', body })) }) as IssueEvent;
  assert.equal(version({ kind: 'created' } as IssueEvent), 1);
  assert.equal(version(h('- [ ] a')), 1);
  assert.equal(version(h('- [ ] a\n- [ ] b', '- [x] a\n- [ ] b', '- [x] a\n- [ ] c')), 2);
  assert.ok(isTick('- [ ] a', '- [X] a'));
  assert.ok(!isTick('- [ ] a', '- [x] b'));
  assert.ok(!isTick('a', 'a'));
});

const ver = (body: string, html: string, id: string) => ({ id, author: { id: 'a', name: 'Person A' }, at: id, body, html });
const v1md = '## Expected\n\n- [ ] a\n- [ ] b\n- [ ] c\n\n## Workaround\n\nNone.';
const v1html = '<h2>Expected</h2>\n<ul>\n<li><input disabled="" type="checkbox"> a</li>\n<li><input disabled="" type="checkbox"> b</li>\n<li><input disabled="" type="checkbox"> c</li>\n</ul>\n<h2>Workaround</h2>\n<p>None.</p>\n';
const tickhtml = v1html.replace('<input disabled="" type="checkbox"> a', '<input checked="" disabled="" type="checkbox"> a');
const v2html = tickhtml.replace('> c</li>', '> C</li>').replace('None.', 'Some.');
const created = (...h: ReturnType<typeof ver>[]) => ({ id: 'c', kind: 'created', author: h[0]!.author, at: '1', body: h.at(-1)!.body, html: h.at(-1)!.html, history: h }) as IssueEvent;
const three = created(ver(v1md, v1html, '1'), ver(v1md.replace('[ ] a', '[x] a'), tickhtml, '2'), ver(v1md.replace('[ ] a', '[x] a').replace('- [ ] c', '- [ ] C').replace('None.', 'Some.'), v2html, '3'));

test('versions counts no tick', () => {
  const vs = versions(three);
  assert.deepEqual(vs.map((v) => [v.n, v.id, v.ticks]), [[1, '1', 1], [2, '3', 0]]);
  assert.match(vs[0]!.body, /\[x\] a/);
  assert.equal(vs[1]!.html, v2html);
});

test('versions without history is v1', () => {
  const vs = versions({ id: 'c', kind: 'created', author: { id: 'a', name: 'A' }, at: '1', body: 'x', html: '<p>x</p>' } as IssueEvent);
  assert.deepEqual(vs.map((v) => [v.n, v.id, v.html]), [[1, 'c', '<p>x</p>']]);
});

test('a thread state against the current version', () => {
  const vs = versions(three);
  const root = (anchor: string, resolved?: string) => ({ kind: 'comment', meta: { 'thread:anchor': anchor, ...(resolved !== undefined ? { 'thread:resolved': resolved } : {}) } }) as unknown as IssueEvent;
  assert.deepEqual(stateOf(root('body@v1/expected', 'v2'), vs), { state: 'resolved' });
  assert.deepEqual(stateOf(root('body@v1/expected', ''), vs), { state: 'outdated', since: 2 });
  assert.deepEqual(stateOf(root('body@v2/expected'), vs), { state: 'current' });
  assert.deepEqual(stateOf(root('body@v1/workaround'), vs), { state: 'outdated', since: 2 });
  assert.deepEqual(stateOf(root('body@v1/gone'), vs), { state: 'outdated', since: 2 });
  const carried = created(ver(v1md, v1html, '1'), ver(v1md, v1html.replace('None.', 'Some.'), '2'));
  carried.history![1]!.body = v1md.replace('None.', 'Some.');
  assert.deepEqual(stateOf(root('body@v1/expected'), versions(carried)), { state: 'carried' });
});

test('criteria are the checkbox lines', () => {
  assert.deepEqual(criteriaOf(md), { n: 4, ticked: 0 });
  assert.deepEqual(criteriaOf(md.replace('- [ ] Pre-release', '- [x] Pre-release')), { n: 4, ticked: 1 });
  assert.deepEqual(criteriaOf(md + '\n\n```\n- [ ] not one\n```'), { n: 4, ticked: 0 });
});
