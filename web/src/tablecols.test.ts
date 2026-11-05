import { test } from 'node:test';
import assert from 'node:assert/strict';
import { chooserColumns, defaultColumns, moveColumn, rangeIds, savedColumns, shownColumns, toggleColumn } from './tablecols.ts';

const names = ['prio', 'status'];

test('a view without columns shows the default ones, the forge after the id', () => {
  assert.deepEqual(defaultColumns(names), ['id', 'links', 'title', 'is', 'prio', 'status', 'other']);
  assert.deepEqual(shownColumns(undefined, names), defaultColumns(names));
  assert.deepEqual(chooserColumns(undefined, names), ['id', 'links', 'title', 'is', 'prio', 'status', 'other']);
});

test('saved columns keep their order, drop unknown keys, always show the title', () => {
  assert.deepEqual(shownColumns(['status', 'links', 'gone', 'id', 'status'], names), ['status', 'title', 'links', 'id']);
  assert.deepEqual(chooserColumns(['status', 'links'], names), ['status', 'title', 'links', 'id', 'is', 'prio', 'other']);
});

test('toggling and moving columns', () => {
  const cols = defaultColumns(names);
  assert.deepEqual(toggleColumn(cols, 'prio'), ['id', 'links', 'title', 'is', 'status', 'other']);
  assert.deepEqual(toggleColumn(cols, 'title'), cols); // the title stays
  assert.deepEqual(toggleColumn(cols, 'links'), ['id', 'title', 'is', 'prio', 'status', 'other']); // hidden
  assert.deepEqual(toggleColumn(toggleColumn(cols, 'links'), 'links'), ['id', 'title', 'is', 'prio', 'status', 'other', 'links']); // shown again, at the end
  assert.deepEqual(moveColumn(cols, 'status', -1), ['id', 'links', 'title', 'is', 'status', 'prio', 'other']);
  assert.deepEqual(moveColumn(cols, 'id', -1), cols);
});

test('savedColumns is undefined for the default, so the view stays dynamic', () => {
  assert.equal(savedColumns(defaultColumns(names), names), undefined);
  assert.deepEqual(savedColumns(['title', 'status'], names), ['title', 'status']);
});

test('rangeIds is the run between two rows, either way', () => {
  const order = ['a', 'b', 'c', 'd', 'e'];
  assert.deepEqual(rangeIds(order, 'b', 'd'), ['b', 'c', 'd']);
  assert.deepEqual(rangeIds(order, 'd', 'b'), ['b', 'c', 'd']);
  assert.deepEqual(rangeIds(order, 'x', 'c'), ['c']);
});

test('the forge column is shown by default, after the id', () => {
  assert.deepEqual(defaultColumns(['status']).slice(0, 3), ['id', 'links', 'title']);
});
