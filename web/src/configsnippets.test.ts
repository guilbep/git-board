import { test } from 'node:test';
import assert from 'node:assert/strict';
import { addSprint, addView } from './configsnippets.ts';

test('addSprint follows the last sprint', () => {
  const text = '[sprints]\nlength_days = 14\n\n[[sprints.list]]\nslug = "sprint-6"\ntitle = "Sprint 6"\nstart = 2026-02-03\n\n[views.mine]\nfilter = "assignee:@me"\n';
  const s = addSprint(text);
  assert.match(s.text, /slug = "sprint-6"\ntitle = "Sprint 6"\nstart = 2026-02-03\n\n\[\[sprints\.list\]\]\nslug = "sprint-7"\ntitle = "Sprint 7"\nstart = 2026-02-17\n\n\[views\.mine\]/);
  assert.equal(s.text.slice(...s.select), 'sprint-7');
});

test('addSprint starts the list today when there is none', () => {
  const s = addSprint('', new Date('2026-10-15T09:00:00Z'));
  assert.equal(s.text, '[[sprints.list]]\nslug = "sprint-1"\ntitle = "Sprint 1"\nstart = 2026-10-15\n');
});

test('addView appends a table and selects its name', () => {
  const s = addView('[fields.status]\nsingle = true\n');
  assert.ok(s.text.endsWith('\n\n[views.new-view]\nlayout = "board"\nfilter = "is:open"\ngroup = "status"\n'));
  assert.equal(s.text.slice(...s.select), 'new-view');
});
