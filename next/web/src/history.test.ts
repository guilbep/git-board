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

import { waitingChanges } from './history.ts';

const who = (name: string) => ({ id: name, name });

test('waiting changes: what each is, newest first, with the forges it waits for', () => {
  const events = [
    { kind: 'created' as const, id: 'c0', author: who('ann'), at: '2026-10-14T09:00:00+02:00', body: 'desc' },
    { kind: 'comment' as const, id: 'c1', author: who('ann'), at: '2026-10-14T10:00:00+02:00', body: 'old', pending: ['GitLab'] },
    {
      kind: 'comment' as const, id: 'c2', author: who('ann'), at: '2026-10-14T11:00:00+02:00', body: 'new text', edit_pending: ['GitHub', 'GitLab'], edit_was: 'old text',
      history: [{ author: who('ann'), at: '2026-10-14T11:00:00+02:00', body: 'old text' }, { author: who('bob'), at: '2026-10-15T08:00:00Z', body: 'new text' }],
    },
    { kind: 'title' as const, id: 't', author: who('bob'), at: '2026-10-15T07:00:00Z', title: 'B', was: 'A', pending: ['GitHub'] },
    { kind: 'status' as const, id: 's', author: who('bob'), at: '2026-10-15T06:00:00Z', status: 'closed' as const, pending: ['GitHub'] },
    { kind: 'labels' as const, id: 'l', author: who('bob'), at: '2026-10-14T12:00:00+02:00', added: ['x'], removed: ['y'] }, // there everywhere
  ];
  const got = waitingChanges(events).map((c) => [c.what, c.event.id, c.author.name, c.waiting.join('+')]);
  assert.deepEqual(got, [
    ['Comment edited', 'c2', 'bob', 'GitHub+GitLab'], // the edit's author and time, 08:00Z
    ['Title changed', 't', 'bob', 'GitHub'],
    ['Closed', 's', 'bob', 'GitHub'],
    ['Comment added', 'c1', 'ann', 'GitLab'],
  ]);
});
