import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { IssueEvent } from './api/board.ts';
import {
  anchorLabel, anchorOf, anchorString, applySuggestion, isTick, lineAt, mdSections, sectionsOf, slugs, suggestionOf, version,
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
  assert.equal(anchorString(at('body@v1/expected#3')), 'body@v1/expected#3');
  assert.equal(anchorString(at('body@v2/intro')), 'body@v2/intro');
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

test('a fence is not a heading', () => {
  const fenced = md.replace('## Workaround', '```\n# not one\n```\n\n## Workaround');
  assert.deepEqual(mdSections(fenced, secs)!.map((s) => s.slug), ['intro', 'releases-page', 'expected', 'workaround']);
});

test("a heading's slug is its rendered text", () => {
  const h = sectionsOf('<h2><a href="https://x">Releases</a> page</h2><p>a</p><h2>A &amp; B</h2><p>b</p>');
  assert.deepEqual(h.map((s) => [s.slug, s.heading]), [['releases-page', 'Releases page'], ['a-b', 'A & B']]);
  const m = mdSections('## [Releases](https://x) page\n\na\n\n## A & B\n\nb', h)!;
  assert.deepEqual(m.map((s) => s.slug), ['releases-page', 'a-b']);
});

test('a setext heading opens a section', () => {
  const s = sectionsOf('<p>p</p><h2>Expected</h2><ul><li>x</li><li>y</li></ul>');
  const text = 'p\n\nExpected\n---\n\n- x\n- y';
  assert.deepEqual(mdSections(text, s)!.map((x) => [x.slug, x.start, x.body, x.end]), [['intro', 0, 0, 2], ['expected', 2, 4, 7]]);
  assert.equal(lineAt(text, s, at('body@v1/expected#2')), 'y');
});

test('a quoted heading opens none', () => {
  const s = sectionsOf('<blockquote><h1>Note</h1></blockquote><h2>Real</h2><p>x</p>');
  assert.deepEqual(s.map((x) => x.slug), ['intro', 'real']);
  assert.deepEqual(mdSections('> # Note\n\n## Real\n\nx', s)!.map((x) => x.slug), ['intro', 'real']);
});

test('mismatched headings refuse', () => {
  const extra = `${md}\n\n## Later`;
  assert.equal(mdSections(extra, secs), undefined);
  assert.equal(applySuggestion(extra, secs, at('body@v1/expected#3'), 'x'), undefined);
});

test('lineAt', () => {
  assert.equal(lineAt(md, secs, at('body@v1/expected#3')), 'A missing identity sends the user to Settings › identity, not to a CLI command.');
  assert.equal(lineAt(md, secs, at('body@v1/workaround')), 'Not tried yet: [release] remote = "fork".');
  assert.equal(lineAt(md, secs, at('body@v1/expected#9')), undefined);
  assert.equal(lineAt(md, secs, at('body@v1/gone')), undefined);
});

test('suggestionOf', () => {
  const body = 'Then say what replaces the CLI text.\n\n```suggestion\nA missing identity sends the user to Settings › identity; the CLI text is never shown.\n```';
  assert.equal(suggestionOf(body), 'A missing identity sends the user to Settings › identity; the CLI text is never shown.');
  assert.equal(suggestionOf('no block'), undefined);
  assert.equal(suggestionOf('```suggestion\n```'), '');
});

test('applySuggestion replaces the item and keeps its marker', () => {
  const out = applySuggestion(md, secs, at('body@v1/expected#3'), 'A missing identity sends the user to Settings › identity; the CLI text is never shown.')!;
  assert.ok(out.includes('\n- [ ] A missing identity sends the user to Settings › identity; the CLI text is never shown.\n'));
  assert.ok(!out.includes('not to a CLI command'));
  assert.equal(out.split('\n').length, md.split('\n').length);
});

test('applySuggestion without an item replaces the section', () => {
  const out = applySuggestion(md, secs, at('body@v1/workaround'), 'Tried: it works.')!;
  assert.ok(out.endsWith('## Workaround\n\nTried: it works.'));
  const intro = applySuggestion(md, secs, at('body@v1/intro'), 'New intro.')!;
  assert.ok(intro.startsWith('New intro.\n\n## Releases page'));
});

test('applySuggestion on a gone section is undefined', () => {
  assert.equal(applySuggestion(md, secs, at('body@v1/gone'), 'x'), undefined);
  assert.equal(applySuggestion(md, secs, at('body@v1/expected#9'), 'x'), undefined);
});
