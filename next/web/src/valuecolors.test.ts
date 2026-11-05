// Run with: node --test src/valuecolors.test.ts
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { hexRules, valueColors } from './valuecolors.ts';

test('the file’s color, then a word’s, then the spread; numbers none', () => {
  const got = valueColors(
    { status: { type: 'select', colors: { Ready: 'yellow' } }, est: { type: 'number' } },
    { status: ['ready', 'done', 'zeta', 'alpha', '3'], est: ['1', '2'] },
  );
  assert.deepEqual(got.status, { ready: 'amber', done: 'green', alpha: 'blue', zeta: 'purple' });
  assert.deepEqual(got.est, {});
});

test('a date field has only the colors the file sets', () => {
  assert.deepEqual(valueColors({ due: { type: 'date', colors: { '2026-01-01': 'red' } } }, { due: ['2026-01-01', 'done'] }).due, { '2026-01-01': 'red' });
});

test('a #rrggbb color paints the dot alone, once per color', () => {
  const css = hexRules({ kind: { bug: '#d73a4a', feat: 'teal', docs: '#d73a4a' }, status: { ready: '#ffffff' } });
  assert.equal(css, '.gb-c-xd73a4a{--c-bg:var(--bg-subtle);--c-fg:var(--fg);--c-dot:#d73a4a}\n.gb-c-xffffff{--c-bg:var(--bg-subtle);--c-fg:var(--fg);--c-dot:#ffffff}');
  assert.equal(hexRules({}), '');
});
