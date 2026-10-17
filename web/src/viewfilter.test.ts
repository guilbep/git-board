import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { Facet } from './facets.ts';
import { composeFilter, withFacet } from './viewfilter.ts';

test('composeFilter: an older link\'s filter, the view\'s with its ticks and words', () => {
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

test('a tick writes the facet\'s term after the rest of the filter', () => {
  const status: Facet = { key: 'status', options: [['ready', 1], ['qa', 1], ['validated', 0], ['(none)', 2]], selected: [], rest: 'is:open label:"nice to have"' };
  assert.equal(withFacet(status, ['ready', 'qa']), 'is:open label:"nice to have" status:ready,qa');
  assert.equal(withFacet(status, []), 'is:open label:"nice to have"');
  assert.equal(withFacet(status, ['(none)']), 'is:open label:"nice to have" no:status');
  // not set and values: the values left out, negated
  assert.equal(withFacet(status, ['ready', 'qa', '(none)']), 'is:open label:"nice to have" -status:validated');
  assert.equal(withFacet(status, ['ready', 'qa', 'validated', '(none)']), 'is:open label:"nice to have"');
  const label: Facet = { key: 'label', options: [['nice to have', 1], ['(none)', 1]], selected: [], rest: '' };
  assert.equal(withFacet(label, ['nice to have']), 'label:"nice to have"');
  assert.equal(withFacet(label, ['(none)']), 'no:label');
  const is: Facet = { key: 'is', options: [['open', 3], ['closed', 1]], selected: ['open'], rest: 'status:ready' };
  assert.equal(withFacet(is, ['open', 'closed']), 'status:ready is:open,closed');
});
