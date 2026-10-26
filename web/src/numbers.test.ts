import { test } from 'node:test';
import assert from 'node:assert/strict';
import { numbered, numberExample, numbersPreview, numbersSummary, prefixProblem, startProblem, suggestPrefix, suggestStart } from './numbers.ts';

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

test('start: one above every number known, GitHub included when it answered', () => {
  assert.equal(suggestStart(2518), 2519);
  assert.equal(suggestStart(2518, 3051), 3052);
  assert.equal(suggestStart(0), 1);
  assert.equal(startProblem(3052, 3051), '');
  assert.match(startProblem(3051, 3051), /above 3051/);
  assert.notEqual(startProblem(0, 0), '');
  assert.notEqual(startProblem(1.5, 0), '');
  assert.notEqual(startProblem(NaN, 0), '');
});

test('the preview: what the issues on origin become, GitHub numbers below start kept', () => {
  assert.equal(numbersPreview('GB', 5, [0, 0, 0, 0], 0), 'Your 4 issues on origin will become GB-5 … GB-8.');
  assert.equal(numbersPreview('GB', 5, [0], 0), 'Your 1 issue on origin will become GB-5.');
  assert.equal(numbersPreview('CO2', 3052, [0, 2518, 3060], 0), 'Your 2 issues on origin will become CO2-3052 … CO2-3053; #2518 stays CO2-2518.');
  assert.equal(
    numbersPreview('CO2', 3052, [2518, 12, 0], 2),
    'Your 1 issue on origin will become CO2-3052; #2518 stays CO2-2518, as 1 more keeps its GitHub number. 2 issues only here get their number when pushed.',
  );
  assert.equal(numbersPreview('CO2', 3052, [], 1), 'No issue on origin yet. 1 issue only here gets its number when pushed.');
  assert.equal(numbersPreview('CO2', 3052, [5], 0), 'No issue on origin needs a number; #5 stays CO2-5.');
});

test('what a numbering did, and the Overview line', () => {
  assert.equal(numbered('GB', 9, 4), '4 issues numbered: GB-5 … GB-8.');
  assert.equal(numbered('GB', 9, 1), '1 issue numbered: GB-8.');
  assert.equal(numbered('GB', 9, 0), 'No issue needed a number.');
  assert.equal(numbersSummary({ on: true, prefix: 'GB', next: 9 }), 'GB · next GB-9.');
  assert.equal(numbersSummary({ on: false }), 'Off: issues go by their forge numbers.');
  // the example a page gives: the board's next number, else a neutral one
  assert.equal(numberExample({ on: true, prefix: 'GB', next: 39 }), 'GB-39');
  assert.equal(numberExample({ on: false }), 'ABC-123');
  assert.equal(numberExample(null), 'ABC-123');
});
