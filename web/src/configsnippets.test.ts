import { test } from 'node:test';
import assert from 'node:assert/strict';
import { docSectionAt } from './configsnippets.ts';

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
