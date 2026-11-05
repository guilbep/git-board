import { test } from 'node:test';
import assert from 'node:assert/strict';
import { valueIcon } from './valueicons.ts';

test('valueIcon picks a status, priority or kind icon from the value', () => {
  const cases: [string, string, string][] = [
    ['status', 'backlog', 'st-backlog'],
    ['status', 'draft', 'st-backlog'],
    ['status', 'ready', 'st-ready'],
    ['status', 'in-progress', 'st-progress'],
    ['status', 'in-review', 'st-review'],
    ['status', 'ready-for-validation', 'st-review'],
    ['status', 'tests-metier', 'st-test'],
    ['status', 'validated', 'st-done'],
    ['status', 'blocked', 'st-blocked'],
    ['status', 'comite', 'st-waiting'],
    ['prio', 'p0', 'prio-urgent'],
    ['prio', 'p1', 'prio-high'],
    ['prio', 'p2', 'prio-medium'],
    ['prio', 'p3', 'prio-low'],
    ['kind', 'feat', 'kind-feat'],
    ['cat', 'bug', 'kind-bug'],
    ['kind', 'doc', 'kind-doc'],
    ['kind', 'chore', 'kind-chore'],
    ['sprint', 'delivery', ''], // a sprint is a name, no icon
    ['assignee', 'guilbep', ''],
  ];
  for (const [field, value, want] of cases) assert.equal(valueIcon(field, value), want, `${field}:${value}`);
});
