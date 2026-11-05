import { test } from 'node:test';
import assert from 'node:assert/strict';
import { inContext, keyLabel, lineFor, noteUse, shortcuts, uses } from './keys.ts';

const memory = () => {
  const m = new Map<string, string>();
  return { getItem: (k: string) => m.get(k) ?? null, setItem: (k: string, v: string) => void m.set(k, v) };
};

test('every shortcut has its own id', () => {
  assert.equal(new Set(shortcuts.map((s) => s.id)).size, shortcuts.length);
});

test('inContext: the panel has its keys and anywhere’s, not the filters’', () => {
  const ids = inContext(['board', 'panel']).map((s) => s.id);
  for (const id of ['next', 'prev', 'palette', 'help']) assert.ok(ids.includes(id), id);
  assert.ok(!ids.includes('only'));
  assert.deepEqual(inContext([]).map((s) => s.id), ['palette', 'help']);
});

test('lineFor: the most specific context first, ? last, five at most', () => {
  assert.deepEqual(lineFor(['board']).map((s) => s.id), ['new', 'palette', 'help']);
  assert.deepEqual(lineFor(['board', 'panel']).map((s) => s.id), ['next', 'prev', 'wide', 'esc', 'help']);
  assert.deepEqual(lineFor(['board', 'filters']).map((s) => s.id), ['only', 'range', 'toggle', 'palette', 'help']);
  assert.deepEqual(lineFor([]).map((s) => s.id), ['palette', 'help']);
});

test('keyLabel: Mod and Alt as each keyboard writes them', () => {
  assert.equal(keyLabel('Mod', true), '⌘');
  assert.equal(keyLabel('Mod', false), 'Ctrl');
  assert.equal(keyLabel('Alt', true), '⌥');
  assert.equal(keyLabel('j', false), 'j');
});

test('noteUse counts, per shortcut', () => {
  const s = memory();
  noteUse('palette', s);
  noteUse('palette', s);
  noteUse('next', s);
  assert.deepEqual(uses(s), { palette: 2, next: 1 });
});
