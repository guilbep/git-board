import { test } from 'node:test';
import assert from 'node:assert/strict';
import { datastoreSummary, folderOf, keptIn, needsGit, folderProblem, kindOf, urlProblem, valueOf } from './datastore.ts';

test('kindOf reads board.datastore', () => {
  assert.equal(kindOf('git'), 'git');
  assert.equal(kindOf(''), 'git');
  assert.equal(kindOf('forge'), 'forge');
  assert.equal(kindOf('postgres://me@db/board'), 'postgres');
  assert.equal(kindOf('postgresql://me:xxxxx@db/board'), 'postgres');
});

test('valueOf is what is saved', () => {
  assert.equal(valueOf('git', 'postgres://x/y'), 'git');
  assert.equal(valueOf('forge', ''), 'forge');
  assert.equal(valueOf('postgres', ' postgres://me@db/board '), 'postgres://me@db/board');
});

test('urlProblem asks for a postgres:// URL naming its database', () => {
  assert.notEqual(urlProblem(''), '');
  assert.notEqual(urlProblem('mysql://db/x'), '');
  assert.notEqual(urlProblem('postgres://db'), '');
  assert.equal(urlProblem('postgres://me@db:5432/board?sslmode=require'), '');
});

test('a Markdown folder is markdown:<dir>, inside the checkout', () => {
  assert.equal(kindOf('markdown:issues'), 'markdown');
  assert.equal(valueOf('markdown', ' docs/issues '), 'markdown:docs/issues');
  assert.equal(folderOf('markdown:docs/issues'), 'docs/issues');
  assert.equal(folderOf('git'), '');
  assert.equal(folderProblem('issues'), '');
  assert.notEqual(folderProblem(''), '');
  assert.notEqual(folderProblem('/tmp/x'), '');
  assert.notEqual(folderProblem('../x'), '');
});

test('the menu says the datastore, and where when that says more', () => {
  assert.equal(datastoreSummary({ name: 'git', where: 'git' }), 'git');
  assert.equal(datastoreSummary({ name: 'Markdown files', where: 'issues' }), 'Markdown files, issues');
});

test('needsGit is the server’s sentence on a board kept elsewhere, nothing on git', () => {
  const pg = { name: 'PostgreSQL', replica: false };
  assert.equal(needsGit(pg, 'git board pull'), 'git board pull needs a board kept in git; this board is kept in PostgreSQL');
  assert.equal(needsGit({ name: 'git', replica: true }, 'git board pull'), '');
  assert.equal(needsGit(null, 'git board pull'), ''); // not known yet: as on git
  assert.equal(keptIn({ name: 'Markdown files', replica: false }), 'the board is kept in Markdown files');
  assert.equal(keptIn({ name: 'git', replica: true }), '');
});
