import { test } from 'node:test';
import assert from 'node:assert/strict';
import { jumps, latest, rank } from './palette.ts';

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

test('jumps: once the project numbers its issues, a bare number and CO2-N are its own, #N a forge\'s', () => {
  const counted = [
    { key: 'x', label: 'Bouncer pool', id: 'aaaaaaaaaa', numbers: ['3071'], project: 'CO2-3060', counted: true },
    { key: 'y', label: 'CSV export', id: 'bbbbbbbbbb', numbers: ['3060'], project: 'CO2-3075', counted: true },
    { key: 'w', label: 'New one', id: 'cccccccccc', numbers: ['3080'], counted: true },
  ];
  assert.deepEqual(keys(jumps('3060', counted)), ['x']);
  assert.deepEqual(keys(jumps('co2-3060', counted)), ['x']);
  assert.deepEqual(keys(jumps('#3060', counted)), ['y']);
  assert.deepEqual(keys(jumps('3080', counted)), []); // not numbered yet: a bare number is a project number
  assert.deepEqual(keys(jumps('#3080', counted)), ['w']);
});

test('rank: a command by its label, every word, else its letters in order', () => {
  const commands = [{ key: 'v1', label: 'View: backlog' }, { key: 'a1', label: 'New issue' }, { key: 's', label: 'Settings' }];
  assert.equal(rank('backlog', commands)[0]!.key, 'v1');
  assert.equal(rank('nwis', commands)[0]!.key, 'a1');
  assert.deepEqual(rank('zzz', commands), []);
  assert.equal(rank('', commands).length, 3); // nothing typed: all, in order
});

test('latest: the latest edited first, the latest created breaking a tie, open or closed', () => {
  const synced = '2026-10-15T10:00:00Z'; // one sync stamped both
  const list = [
    { id: 'old', status: 'open', updated_at: synced, created_at: '2026-01-01T10:00:00Z' },
    { id: 'new', status: 'open', updated_at: synced, created_at: '2026-09-01T10:00:00Z' },
    { id: 'closed', status: 'closed', updated_at: '2026-10-16T10:00:00Z', created_at: '2025-01-01T10:00:00Z' },
  ];
  assert.deepEqual(latest(list).map((i) => i.id), ['closed', 'new', 'old']);
});
