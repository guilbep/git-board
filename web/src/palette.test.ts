import { test } from 'node:test';
import assert from 'node:assert/strict';
import { rank } from './palette.ts';

const items = [
  { key: 'i1', label: 'Fix CSV upload on S3', id: 'b9de9e14aa', number: '2528' },
  { key: 'i2', label: 'Travel: car-sharing factor', id: '5a3b1c22bb', number: '2115' },
  { key: 'i3', label: 'Results: totals per unit', id: 'c0ffee11cc', number: '25' },
  { key: 'v1', label: 'View: backlog' },
  { key: 'a1', label: 'New issue' },
];

test('rank finds by number, id prefix and title', () => {
  assert.equal(rank('#2528', items)[0]!.key, 'i1');
  assert.equal(rank('25', items)[0]!.key, 'i3'); // the exact number before a prefix of another
  assert.equal(rank('b9de9e', items)[0]!.key, 'i1');
  assert.equal(rank('car', items)[0]!.key, 'i2');
  assert.equal(rank('csv s3', items)[0]!.key, 'i1'); // every word
  assert.equal(rank('trvl', items)[0]!.key, 'i2'); // letters in order
  assert.equal(rank('backlog', items)[0]!.key, 'v1');
  assert.deepEqual(rank('zzz', items), []);
  assert.equal(rank('', items).length, items.length); // nothing typed: all, in order
});
