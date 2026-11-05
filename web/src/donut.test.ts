import { test } from 'node:test';
import assert from 'node:assert/strict';
import { donutSlices } from './donut.ts';

test('donutSlices share the circle by count', () => {
  const s = donutSlices([{ key: 'done', n: 3 }, { key: 'ready', n: 1 }, { key: 'none', n: 0 }], 50, 30);
  assert.deepEqual(s.map((x) => [x.key, x.pct]), [['done', 75], ['ready', 25]]);
  assert.ok(s.every((x) => x.d.startsWith('M')));
  const whole = donutSlices([{ key: 'done', n: 2 }], 50, 30);
  assert.equal(whole.length, 1);
  assert.equal(whole[0]!.pct, 100);
  assert.match(whole[0]!.d, /A.*A/); // a full ring: two half arcs
  assert.deepEqual(donutSlices([], 50, 30), []);
});
