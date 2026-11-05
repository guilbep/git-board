// Run with: node --test src/idlabel.test.ts
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { forgeNumbers, idLabel } from './idlabel.ts';

const forges = [
  { name: 'GitHub', url: 'https://github.com/o/r/issues/2518', number: '2518' },
  { name: 'gitea.example.com', url: 'https://gitea.example.com/o/r/issues/1' },
  { name: 'GitLab', url: 'https://gitlab.example.com/g/r/-/issues/77', number: '77' },
];

test('forgeNumbers: the issue’s numbers on the forges', () => {
  assert.deepEqual(forgeNumbers({ forges }), ['2518', '77']);
  assert.deepEqual(forgeNumbers({}), []);
});

test('a card’s name: the project number, else the forge’s, then the id', () => {
  assert.equal(idLabel({ id: 'abcdef123', number: 'CO2-3075', forges }), 'CO2-3075 · abcdef1');
  assert.equal(idLabel({ id: 'abcdef123', forges }, 8), '#2518 · abcdef12');
  assert.equal(idLabel({ id: 'abcdef123' }), 'abcdef1');
});
