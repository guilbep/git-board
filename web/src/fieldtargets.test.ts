import assert from 'node:assert/strict';
import { test } from 'node:test';
import { defaultTarget, fieldTarget, forgeLabel, targetsFor, type ForgeModel } from './fieldtargets.ts';

const fields = {
  status: { single: true, options: ['todo', 'done'] },
  est: { single: true, options: ['1', '2'], numeric: true },
  assignee: { type: 'person' },
  reviewer: { type: 'person' },
  parent: { type: 'issue' },
  due: { type: 'date' },
  sprint: { type: 'iteration' },
  notes: { type: 'text' },
};

const gh: ForgeModel = { name: 'gh', kind: 'github', project: 'o/r', host: 'github.com', sync: true };
const gl: ForgeModel = { name: 'gl', kind: 'gitlab', project: '7', host: 'gitlab.epfl.ch', sync: true };

test('the type default: a person in the assignees, an issue in sub-issues, everything else a label', () => {
  assert.equal(defaultTarget('github', 'assignee', 'person'), 'assignees');
  assert.equal(defaultTarget('github', 'reviewer', 'person'), 'label'); // one slot per forge
  assert.equal(defaultTarget('github', 'parent', 'issue'), 'sub-issues');
  assert.equal(defaultTarget('github', 'est', 'number'), 'label');
  assert.equal(defaultTarget('gitlab', 'sprint', 'iteration'), 'milestone');
  assert.equal(defaultTarget('gitlab', 'due', 'date'), 'due-date');
  assert.equal(defaultTarget('gitlab', 'est', 'number'), 'label');
});

test('fieldTarget takes the mapping, then the old homes, then the default', () => {
  const withMapping: ForgeModel = { ...gh, fields: { status: { to: 'project', field: 'Status' } } };
  assert.equal(fieldTarget(withMapping, fields, 'status', [], 0), 'project');
  // the old project map implies the Project target
  const withProjectMap = { ...fields, size: { options: ['1', '2'], numeric: true, project: { '1': 'Small' } } };
  assert.equal(fieldTarget(gh, withProjectMap, 'size', [], 0), 'project');
  // the Project's own columns live there when a Project is configured
  assert.equal(fieldTarget(gh, fields, 'sprint', [], 1), 'project');
  assert.equal(fieldTarget(gh, fields, 'sprint', [], 0), 'label');
  // the type default otherwise
  assert.equal(fieldTarget(gl, fields, 'est', [], 0), 'label');
  assert.equal(fieldTarget(gl, fields, 'due', [], 0), 'due-date');
});

test('targetsFor offers what the forge has, the default first, the paid tier marked', () => {
  assert.deepEqual(targetsFor('github', 'select', 'status').map((t) => t.id), ['label', 'project', 'none']);
  assert.deepEqual(targetsFor('gitlab', 'select', 'status').map((t) => t.id), ['label', 'scoped-label', 'assignees', 'milestone', 'due-date', 'none']);
  assert.match(targetsFor('gitlab', 'select', 'status')[1]?.label ?? '', /paid tier/);
  assert.deepEqual(targetsFor('github', 'person', 'reviewer').map((t) => t.id), ['label', 'assignees', 'none']);
  assert.deepEqual(targetsFor('gitlab', 'issue', 'parent').map((t) => t.id), ['label', 'assignees', 'milestone', 'due-date', 'none']);
});

test('forgeLabel spells the value as the forge has it', () => {
  const plain: ForgeModel = { ...gh, fields: { moscow: { to: 'label', as: 'plain' } } };
  assert.equal(forgeLabel(plain, 'moscow', 'couldhave'), 'couldhave');
  assert.equal(forgeLabel(gh, 'status', 'ready'), 'status:ready');
});
