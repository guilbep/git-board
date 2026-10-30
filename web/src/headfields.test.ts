import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fieldLine, labelLine } from './headfields.ts';

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
