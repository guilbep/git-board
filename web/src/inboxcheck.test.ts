import { test } from 'node:test';
import assert from 'node:assert/strict';
import { FRESH, checkFailure, toCheck, toPull } from './inboxcheck.ts';
import type { Remote } from './api/board';

const remote = (name: string, checked: string | undefined, receive = 0): Remote => ({
  name, url: '', push_urls: [], tracked: true, issues: { send: 0, receive }, people: { send: 0, receive: 0 }, ...(checked ? { checked } : {}),
});

test('opening the inbox checks the remotes not checked lately; Refresh checks them all', () => {
  const now = Date.parse('2026-10-17T10:00:00Z');
  const rs = [
    remote('origin', new Date(now - FRESH + 1000).toISOString()), // a minute ago: a reload
    remote('upstream', new Date(now - FRESH - 1000).toISOString()),
    remote('backup', undefined), // never checked
  ];
  assert.deepEqual(toCheck(rs, now, false), ['upstream', 'backup']);
  assert.deepEqual(toCheck(rs, now, true), ['origin', 'upstream', 'backup']);
});

test('a remote with issues to receive offers Pull', () => {
  assert.deepEqual(toPull([remote('origin', undefined, 3), remote('upstream', undefined, 0)]), [{ name: 'origin', issues: 3 }]);
});

// Review Focus 5: a forge sync holds the remotes; the page says so.
test('a check refused during a forge sync says so', () => {
  assert.equal(checkFailure('origin', 'syncing', 'a sync with the forges is running'), 'origin: a forge sync is running: check again when it ends');
  assert.equal(checkFailure('upstream', '', 'fetch upstream: ssh: handshake failed'), "upstream: couldn't check: fetch upstream: ssh: handshake failed");
});
