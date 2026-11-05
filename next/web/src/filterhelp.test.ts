import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { FieldConfig } from './api/types.ts';
import { examples, nextTrick } from './filterhelp.ts';

const f = (type?: string): FieldConfig => ({ single: true, options: null, numeric: false, ...(type ? { type } : {}) });

test('examples: this board’s own fields, @me only with a login, five at most', () => {
  const fields = { 'lead-dev': f('person'), sprint: f('iteration'), status: f('select'), kind: f('select') };
  assert.deepEqual(examples(fields, 'guilbep').map((e) => e.term), ['lead-dev:@me', 'sprint:@current', 'no:status', 'is:closed', 'involves:@me']);
  assert.deepEqual(examples(fields, '').map((e) => e.term), ['sprint:@current', 'no:status', 'is:closed']);
  assert.equal(examples({ assignee: f('person'), owner: f('person') }, 'x')[0]!.term, 'assignee:@me');
  assert.deepEqual(examples({}, '').map((e) => e.term), ['is:closed']);
});

test('nextTrick: the trick for the word being typed', () => {
  assert.match(nextTrick('status:r')!, /-status: .*status:a,b/);
  assert.match(nextTrick('travel -status:')!, /no:status/);
  assert.match(nextTrick('car')!, /quotes/);
  assert.equal(nextTrick(''), null);
  assert.equal(nextTrick('is:cl'), null);
  assert.match(nextTrick('created:')!, /@today-7d/);
  assert.match(nextTrick('-closed:>2')!, /\.\./);
  assert.match(nextTrick('comments:')!, /2\.\.5/);
  assert.match(nextTrick('crash in:')!, /title,body/);
});
