import { test } from 'node:test';
import assert from 'node:assert/strict';
import { issueRef, projectMark, runWords } from './projectview.ts';
import type { ProjectRun } from './api/project.ts';

test('a conflict names its issue as resolve takes it', () => {
  assert.equal(issueRef({ url: 'https://github.com/EPFL-ENAC/co2-calculator/issues/2605', number: 2605 }), 'EPFL-ENAC/co2-calculator#2605');
  assert.equal(issueRef({ url: '', number: 7 }), '7');
});

test('a run in one line', () => {
  const r = { counts: { matched: 3, label_changes: 1, project_writes: 2, conflicts: 1, warnings: 0, unmatched: 0 }, skipped: 2, truncated: false, items: 3 } as unknown as ProjectRun;
  assert.equal(runWords(r), '3 issues matched, 1 with label changes, 2 Project writes, 2 not written (no write = true), 1 conflict');
});

test('a field in the Project, its column found or not', () => {
  const places = [
    { field: 'status', forge: 'gh', kind: 'github', to: 'project', default: 'label', column: 'Status', on_project: true },
    { field: 'risk', forge: 'gh', kind: 'github', to: 'project', default: 'label', column: 'Risk', on_project: false },
    { field: 'prio', forge: 'gh', kind: 'github', to: 'label', default: 'label' },
  ];
  assert.deepEqual(projectMark(places, 'gh', 'status'), { column: 'Status', missing: false });
  assert.deepEqual(projectMark(places, 'gh', 'risk'), { column: 'Risk', missing: true });
  assert.equal(projectMark(places, 'gh', 'prio'), null);
  // a forge found in the remotes, which the server's places don't name
  assert.deepEqual(projectMark(places, 'github', 'risk'), { column: 'Risk', missing: true });
});
