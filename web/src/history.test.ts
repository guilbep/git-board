// Run with: node --test web/src/history.test.ts
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { describeLabels } from './history.ts';

test('a field swap reads as setting it', () => {
  assert.deepEqual(describeLabels(['status:qa'], ['status:ready']), [{ verb: 'set', field: 'status', values: ['qa'] }]);
});

test('multi-valued fields add and remove, plain labels are labels', () => {
  assert.deepEqual(describeLabels(['assignee:bob', 'bug'], ['assignee:alice', 'nice to have']), [
    { verb: 'added', field: 'assignee', values: ['bob'] },
    { verb: 'removed', field: 'assignee', values: ['alice'] },
    { verb: 'added', field: 'label', values: ['bug'] },
    { verb: 'removed', field: 'label', values: ['nice to have'] },
  ]);
});

test('a field only removed is cleared', () => {
  assert.deepEqual(describeLabels([], ['prio:p1']), [{ verb: 'cleared', field: 'prio', values: ['p1'] }]);
});

test('a field set from nothing is set', () => {
  assert.deepEqual(describeLabels(['sprint:hypercare'], []), [{ verb: 'set', field: 'sprint', values: ['hypercare'] }]);
});
