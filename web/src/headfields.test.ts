import { test } from 'node:test';
import assert from 'node:assert/strict';
import { dueNote, fieldLine, labelLine } from './headfields.ts';

const names = ['assignee', 'blocked-by', 'due', 'kind', 'prio', 'sprint', 'status', 'team'];

test('the top fields set, the unset ones to set, the others as more', () => {
  const l = fieldLine(names, ['status:in-review', 'assignee:person-a', 'assignee:person-b', 'prio:p1', 'team:web', 'due:2026-11-02', 'blocked-by:#2802']);
  assert.deepEqual(l.top, [{ name: 'assignee', values: ['person-a', 'person-b'] }, { name: 'prio', values: ['p1'] }]);
  assert.deepEqual(l.unset, ['sprint', 'kind']);
  assert.deepEqual(l.more, ['due', 'team']);
  assert.deepEqual(l.blockedBy, ['#2802']);
  assert.deepEqual(l.order, ['assignee', 'prio', 'sprint', 'kind']);
});

test('a project without the usual fields shows its first four', () => {
  const l = fieldLine(['area', 'budget', 'client', 'phase', 'status', 'zone'], ['client:x', 'zone:y']);
  assert.deepEqual(l.top, [{ name: 'client', values: ['x'] }]);
  assert.deepEqual(l.unset, ['area', 'budget', 'phase']);
  assert.deepEqual(l.more, ['zone']);
});

test('no fields: an empty line', () => {
  assert.deepEqual(fieldLine([], ['status:ready']), { order: [], top: [], unset: [], more: [], blockedBy: [] });
});

test('labels: the list picked, in its order; the rest outside it', () => {
  assert.deepEqual(labelLine(['wontfix', 'status:ready', 'security', 'kind:bug', 'needs-design'], ['needs-design', 'regression', 'security']), {
    picked: ['needs-design', 'security'],
    outside: ['wontfix'],
  });
  assert.deepEqual(labelLine(['a', 'b'], []), { picked: [], outside: ['a', 'b'] });
  assert.deepEqual(labelLine(['prio:p1'], ['x']), { picked: [], outside: [] });
});

test('a date against today: late, today, in n days', () => {
  const today = new Date('2026-10-30T09:00:00');
  assert.equal(dueNote('2026-10-28', today), '2 days late');
  assert.equal(dueNote('2026-10-29', today), '1 day late');
  assert.equal(dueNote('2026-10-30', today), 'Today');
  assert.equal(dueNote('2026-10-31', today), 'In 1 day');
  assert.equal(dueNote('2026-11-14', today), 'In 15 days');
  assert.equal(dueNote('not a date', today), '');
  // days that don't exist: no "In NaN days", no March 2
  assert.equal(dueNote('2026-13-45', today), '');
  assert.equal(dueNote('2026-02-30', today), '');
  assert.equal(dueNote('2026-02-29', today), '');
  assert.equal(dueNote('2028-02-29', new Date(2028, 1, 28, 9)), 'In 1 day');
});
