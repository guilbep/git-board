import { test } from 'node:test';
import assert from 'node:assert/strict';
import { datastoreSummary, kindOf, urlProblem, valueOf } from './datastore.ts';

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

test('the menu says when a change waits for a restart', () => {
  assert.equal(datastoreSummary({ datastore: 'git', restart: false }), 'git');
  assert.equal(datastoreSummary({ datastore: 'git', restart: true }), 'git; a change waits for a restart');
});
