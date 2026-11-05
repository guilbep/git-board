import { test } from 'node:test';
import assert from 'node:assert/strict';
import { prefixProblem, suggestPrefix } from './numbers.ts';

test('a prefix from the repository name: its initials, a first word with a digit kept, one word its first three letters', () => {
  assert.equal(suggestPrefix('git-board'), 'GB');
  assert.equal(suggestPrefix('co2-calculator'), 'CO2');
  assert.equal(suggestPrefix('gitboard'), 'GIT');
  assert.equal(suggestPrefix('my_big.web-app'), 'MBWA');
  assert.equal(suggestPrefix('a-b-c-d-e-f-g'), 'ABCDE');
  assert.equal(suggestPrefix('2048-game'), 'GAM'); // a number first: from the first letter
  assert.equal(suggestPrefix('42'), '');
  assert.equal(suggestPrefix(''), '');
  for (const name of ['git-board', 'co2-calculator', 'gitboard', '2048-game', 'k8s-tools']) assert.equal(prefixProblem(suggestPrefix(name)), '', name);
});

test('a prefix is checked as the server does: a letter, then letters or digits', () => {
  assert.equal(prefixProblem('CO2'), '');
  assert.equal(prefixProblem('gb'), '');
  assert.notEqual(prefixProblem(''), '');
  assert.notEqual(prefixProblem('CO-2'), '');
  assert.notEqual(prefixProblem('2CO'), '');
  assert.notEqual(prefixProblem('C O'), '');
});
