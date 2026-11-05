import { test } from 'node:test';
import assert from 'node:assert/strict';
import { nextStep } from './nextstep.ts';

const co2 = ['backlog', 'comite', 'ready', 'in-progress', 'in-review', 'ready-for-validation', 'qa', 'tests-metier', 'validated'];
const plain = ['backlog', 'ready', 'in-progress', 'in-review', 'ready-for-validation', 'validated'];

test('no status moves to the first option', () => {
  assert.deepEqual(nextStep(co2, '', false), { icon: 'forward', label: 'Move to backlog', target: 'backlog', closes: false, reopens: false });
});

test('a status in the middle moves to the next one, by name', () => {
  assert.equal(nextStep(plain, 'in-review', false)?.label, 'Move to ready-for-validation');
  assert.equal(nextStep(co2, 'qa', false)?.target, 'tests-metier');
});

test('the step onto the last option closes the issue there', () => {
  assert.deepEqual(nextStep(co2, 'tests-metier', false), { icon: 'check', label: 'Close as validated', target: 'validated', closes: true, reopens: false });
});

test('an open issue at the last option only closes', () => {
  assert.deepEqual(nextStep(plain, 'validated', false), { icon: 'check', label: 'Close the issue', target: '', closes: true, reopens: false });
});

test('a closed issue reopens, whatever its status', () => {
  assert.equal(nextStep(plain, 'validated', true)?.label, 'Reopen');
  assert.equal(nextStep([], '', true)?.reopens, true);
});

test('no workflow, no arrow', () => {
  assert.equal(nextStep([], '', false), null);
});

test('a value the workflow does not list starts it again', () => {
  assert.equal(nextStep(plain, 'triage', false)?.target, 'backlog');
});
