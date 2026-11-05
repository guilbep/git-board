import { test } from 'node:test';
import assert from 'node:assert/strict';
import { byCheck, changeText, deleteConfirm, hexRules, lintLines } from './labels.ts';

test('a change says what Send to GitHub does, as lint --create-labels does', () => {
  assert.equal(changeText({ name: 'feat', color: '0e8a16', description: 'A new feature' }), 'Create feat #0e8a16 "A new feature"');
  assert.equal(changeText({ name: 'hold:paused', color: 'd99100', description: '' }), 'Create hold:paused #d99100');
  const was = { name: 'bug', color: 'd73a4a', description: 'Broken' };
  assert.equal(changeText({ ...was, color: 'b60205', was }), 'Update bug: color #d73a4a → #b60205');
  assert.equal(changeText({ ...was, description: 'It breaks', was }), 'Update bug: description "Broken" → "It breaks"');
});

test("lint's findings are grouped by check, in the server's order", () => {
  const f = (check: string, issue_id: string) => ({ check, issue_id });
  assert.deepEqual(byCheck([f('double-single', 'a'), f('double-single', 'b'), f('two-kinds', 'a')]), [
    { check: 'double-single', findings: [f('double-single', 'a'), f('double-single', 'b')] },
    { check: 'two-kinds', findings: [f('two-kinds', 'a')] },
  ]);
  assert.deepEqual(byCheck([]), []);
});

test('the delete confirm says how many issues lose the label', () => {
  assert.equal(deleteConfirm('o/r', 'wontfix', 0), 'Delete wontfix from o/r? No issue carries it.');
  assert.equal(deleteConfirm('o/r', 'wontfix', 1), 'Delete wontfix from o/r? 1 issue loses it.');
  assert.equal(deleteConfirm('o/r', 'wontfix', 3), 'Delete wontfix from o/r? 3 issues lose it.');
});

test('a hex color paints the dot only: one rule per color, the palette names left to gitboard.css', () => {
  const css = hexRules({ kind: { bug: '#d73a4a', feat: 'teal', docs: '#d73a4a' }, status: { ready: '#ffffff' } });
  assert.equal(css, '.gb-c-xd73a4a{--c-bg:var(--bg-subtle);--c-fg:var(--fg);--c-dot:#d73a4a}\n.gb-c-xffffff{--c-bg:var(--bg-subtle);--c-fg:var(--fg);--c-dot:#ffffff}');
  assert.equal(hexRules({}), '');
});

test('lintLines: a missing value and a dead option a line per field, the rest a line per finding', () => {
  const f = (check: string, message: string, issue_id = '', title = '') => ({ check, message, issue_id, title });
  const lines = lintLines([
    f('unknown-value', 'moscow:could has no matching [fields.moscow] option', 'a', 'A'),
    f('missing-value', 'no est value', 'a', 'A'),
    f('missing-value', 'no moscow value', 'a', 'A'),
    f('missing-value', 'no est value', 'b', 'B'),
    f('dead-option', 'est:2: no issue carries it'),
    f('dead-option', 'est:4: no issue carries it'),
    f('dead-option', 'moscow:musthave: no issue carries it'),
  ]);
  assert.deepEqual(lines, [
    { check: 'unknown-value', items: [{ text: 'moscow:could has no matching [fields.moscow] option', issue: { id: 'a', title: 'A' } }] },
    {
      check: 'missing-value',
      items: [
        { text: 'est: 2 open issues have none', filter: 'is:open no:est' },
        { text: 'moscow: 1 open issue has none', filter: 'is:open no:moscow' },
      ],
    },
    { check: 'dead-option', items: [{ text: 'est: no issue carries 2, 4' }, { text: 'moscow: no issue carries musthave' }] },
  ]);
});
