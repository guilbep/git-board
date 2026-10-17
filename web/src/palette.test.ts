import { test } from 'node:test';
import assert from 'node:assert/strict';
import { jumps, openFirst, rank } from './palette.ts';

const issues = [
  { key: 'i1', label: 'Fix CSV upload on S3', id: 'b9de9e14aa', numbers: ['2528'] },
  { key: 'i2', label: 'Travel: car-sharing factor', id: '5a3b1c22bb', numbers: ['2115', '12'] },
  { key: 'i3', label: 'Results: totals per unit', id: 'c0ffee11cc', numbers: ['25'] },
];
const keys = (l: { key: string }[]) => l.map((i) => i.key);

test('jumps: an issue by its number or the start of its id, as the filter takes them', () => {
  assert.deepEqual(keys(jumps('#2528', issues)), ['i1']);
  assert.deepEqual(keys(jumps(' 2528 ', issues)), ['i1']);
  assert.deepEqual(keys(jumps('25', issues)), ['i3']); // the number, not a prefix of another
  assert.deepEqual(keys(jumps('#12', issues)), ['i2']); // on another forge
  assert.deepEqual(keys(jumps('B9DE9E1', issues)), ['i1']);
  assert.deepEqual(jumps('b9de9e', issues), []); // under 7 characters: a word
  assert.deepEqual(jumps('car', issues), []); // words are the filter's
  assert.deepEqual(jumps('#2528 csv', issues), []);
});

test('rank: a command by its label, every word, else its letters in order', () => {
  const commands = [{ key: 'v1', label: 'View: backlog' }, { key: 'a1', label: 'New issue' }, { key: 's', label: 'Settings' }];
  assert.equal(rank('backlog', commands)[0]!.key, 'v1');
  assert.equal(rank('nwis', commands)[0]!.key, 'a1');
  assert.deepEqual(rank('zzz', commands), []);
  assert.equal(rank('', commands).length, 3); // nothing typed: all, in order
});

test('openFirst: open issues before closed ones, the latest changed first', () => {
  const list = [
    { id: 'c', status: 'closed', updated_at: '2026-10-16T10:00:00Z' },
    { id: 'o1', status: 'open', updated_at: '2026-10-01T10:00:00Z' },
    { id: 'o2', status: 'open', updated_at: '2026-10-15T10:00:00Z' },
  ];
  assert.deepEqual(openFirst(list).map((i) => i.id), ['o2', 'o1', 'c']);
});
