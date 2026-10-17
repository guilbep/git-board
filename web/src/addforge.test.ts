import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { Credential } from './api/board';
import { canGoOn, guessForge, matchingCredentials, type ForgeDraft } from './addforge.ts';

const draft = (d: Partial<ForgeDraft>): ForgeDraft => ({ target: 'github', url: '', name: '', credential: '', token: '', ...d });

test('guessForge reads the forge from the address', () => {
  assert.equal(guessForge('https://github.com/guilbep/git-board-test'), 'github');
  assert.equal(guessForge('https://gitlab.epfl.ch/enac/co2'), 'gitlab');
  assert.equal(guessForge('https://gitea.example.org/a/b'), 'gitea');
  assert.equal(guessForge('https://code.example.org/a/b'), '');
  assert.equal(guessForge('github.com/a/b'), ''); // not an address yet
});

test('matchingCredentials keeps the forge, and its instance once the address says it', () => {
  const creds: Credential[] = [
    { id: 'd7f5c11', target: 'github', host: 'github.com', login: 'guilbep' },
    { id: 'aaaaaaa', target: 'gitlab', host: 'gitlab.epfl.ch', login: 'guilbert' },
    { id: 'bbbbbbb', target: 'gitlab', host: 'gitlab.com', login: 'guilbert' },
  ];
  assert.deepEqual(matchingCredentials(creds, 'github', '').map((c) => c.id), ['d7f5c11']);
  assert.deepEqual(matchingCredentials(creds, 'gitlab', '').map((c) => c.id), ['aaaaaaa', 'bbbbbbb']);
  assert.deepEqual(matchingCredentials(creds, 'gitlab', 'https://gitlab.epfl.ch/enac/co2').map((c) => c.id), ['aaaaaaa']);
  assert.deepEqual(matchingCredentials(creds, 'gitea', 'https://gitea.example.org/a/b'), []);
});

test('canGoOn: a forge, then a project address and a fair name, then a token', () => {
  assert.equal(canGoOn(0, draft({})), true);
  assert.equal(canGoOn(0, draft({ target: 'jira' })), false);

  assert.equal(canGoOn(1, draft({})), false);
  assert.equal(canGoOn(1, draft({ url: 'https://github.com/guilbep' })), false); // no project
  assert.equal(canGoOn(1, draft({ url: 'https://github.com/guilbep/git-board-test' })), true);
  assert.equal(canGoOn(1, draft({ url: 'https://gitlab.epfl.ch/a/b/c', target: 'gitlab' })), true); // a subgroup
  assert.equal(canGoOn(1, draft({ url: 'ftp://github.com/a/b' })), false);
  assert.equal(canGoOn(1, draft({ url: 'https://github.com/a/b', name: 'gh-test' })), true);
  assert.equal(canGoOn(1, draft({ url: 'https://github.com/a/b', name: 'my forge' })), false);

  assert.equal(canGoOn(2, draft({})), false);
  assert.equal(canGoOn(2, draft({ token: '   ' })), false);
  assert.equal(canGoOn(2, draft({ token: 'x' })), true);
  assert.equal(canGoOn(2, draft({ credential: 'd7f5c11' })), true);
});
