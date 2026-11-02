import { test } from 'node:test';
import assert from 'node:assert/strict';
import { byId, noteUse } from './keys.ts';
import type { Shortcut } from './keys.ts';
import { dismissTip, pickTip, resetTips, retired } from './tips.ts';

const memory = () => {
  const m = new Map<string, string>();
  return { getItem: (k: string) => m.get(k) ?? null, setItem: (k: string, v: string) => void m.set(k, v), removeItem: (k: string) => void m.delete(k) };
};
const pool = ['next', 'prev', 'wide', 'esc'].map((id) => byId(id)!) as Shortcut[];

test('pickTip goes round, skipping the retired and the tipless', () => {
  assert.equal(pickTip(pool, new Set())?.id, 'next');
  assert.equal(pickTip(pool, new Set(), 'next')?.id, 'prev');
  assert.equal(pickTip(pool, new Set(), 'wide')?.id, 'next'); // esc has no tip: round again
  assert.equal(pickTip(pool, new Set(['prev']), 'next')?.id, 'wide');
  assert.equal(pickTip(pool, new Set(['next', 'prev', 'wide'])), null);
});

test('a shortcut used three times retires; two, not yet', () => {
  const s = memory();
  noteUse('next', s);
  noteUse('next', s);
  assert.ok(!retired(s).has('next'));
  noteUse('next', s);
  assert.ok(retired(s).has('next'));
});

test('dismissed tips retire; resetTips brings every one back', () => {
  const s = memory();
  dismissTip('wide', s);
  for (let i = 0; i < 3; i++) noteUse('prev', s);
  assert.deepEqual([...retired(s)].sort(), ['prev', 'wide']);
  resetTips(s);
  assert.equal(retired(s).size, 0);
});
