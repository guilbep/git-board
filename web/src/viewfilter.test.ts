import { test } from 'node:test';
import assert from 'node:assert/strict';
import { composeFilter } from './viewfilter.ts';

test('composeFilter saves what the page shows', () => {
  assert.deepEqual(composeFilter('is:open', {}, ''), { filter: 'is:open', skipped: [] });
  assert.deepEqual(composeFilter('is:open', { status: ['ready', 'qa'] }, '-status:validated travel'), {
    filter: 'is:open status:ready,qa -status:validated travel',
    skipped: [],
  });
  assert.deepEqual(composeFilter('is:open', { assignee: ['(none)'], label: ['nice to have'], is: ['open', 'closed'] }, ''), {
    filter: 'is:open no:assignee label:"nice to have" is:open,closed',
    skipped: [],
  });
  // "not set" with values: the values left out, negated (all but validated)
  const options = { status: ['backlog', 'ready', 'qa', 'validated', '(none)'] };
  assert.deepEqual(composeFilter('is:open', { status: ['backlog', 'ready', 'qa', '(none)'] }, '', options), {
    filter: 'is:open -status:validated',
    skipped: [],
  });
  assert.deepEqual(composeFilter('is:open', { status: ['ready', '(none)'] }, '', options), {
    filter: 'is:open -status:backlog,qa,validated',
    skipped: [],
  });
  // without the facet's values to negate, it can't be said
  assert.deepEqual(composeFilter('is:open', { status: ['(none)', 'ready'] }, ''), { filter: 'is:open', skipped: ['status'] });
  // a condition already there isn't added twice
  assert.deepEqual(composeFilter('is:open status:ready', { status: ['ready'] }, '  '), { filter: 'is:open status:ready', skipped: [] });
});
