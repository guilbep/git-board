import { test } from 'node:test';
import assert from 'node:assert/strict';
import { addSprint, addView, docSectionAt } from './configsnippets.ts';

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

test('docSectionAt follows the table under the cursor', () => {
  const text = 'list = []\n[fields.due]\ntype = "date"\n[fields.status]\noptions = ["a"]\n[fields.status.colors]\na = "red"\n[views.mine]\nlayout = "board"\nfilter = "is:open"\n[[sprints.list]]\nslug = "s1"\n[github-project]\n[forges.gh]\nkind = "github"\n[uploads]\n[repos]\n[branches]\nbase = "dev"\n[roles]\npm = "claire"\n';
  const at = (needle: string) => docSectionAt(text, text.indexOf(needle) + 1);
  assert.equal(docSectionAt(text, 0), 'overview');
  assert.equal(at('options'), 'fields');
  assert.equal(at('type = "date"'), 'types');
  assert.equal(at('a = "red"'), 'colors');
  assert.equal(at('layout'), 'views');
  assert.equal(at('filter'), 'filters');
  assert.equal(at('slug'), 'sprints');
  assert.equal(at('[github-project]'), 'github');
  assert.equal(at('kind = "github"'), 'forges');
  assert.equal(at('[uploads]'), 'attachments');
  assert.equal(at('[repos]'), 'repos');
  assert.equal(at('base'), 'branches');
  assert.equal(at('pm = '), 'roles');
  assert.equal(docSectionAt('[labels]\nlist = ["needs-design"]\n[labels.colors]\nneeds-design = "red"\n', 12), 'labels');
  assert.equal(docSectionAt('[labels]\nlist = ["needs-design"]\n[labels.colors]\nneeds-design = "red"\n', 50), 'labels');
});
