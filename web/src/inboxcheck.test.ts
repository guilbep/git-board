// Run with: node --test src/inboxcheck.test.ts
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { bellState, followLine, mergeRows } from './inboxcheck.ts';
import type { InboxRow } from './api/types.ts';

const row = (id: string, latest: string): InboxRow =>
  ({ issue: { id, title: id, shown_title: id, status: 'open', labels: [], updated_at: latest, created_at: latest, comments: 0, version: 'v' }, reasons: [], news: [], who: [], latest, first: '' });

test('a row you read stays dimmed until Refresh, and is unread again when something new happens', () => {
  const read = new Map([['x', '2026-10-17T10:00:00Z']]);
  const shown = [row('x', '2026-10-17T10:00:00Z'), row('y', '2026-10-17T09:00:00Z')];
  // x was read: the server no longer lists it, the page keeps it, dimmed
  let m = mergeRows([row('y', '2026-10-17T09:00:00Z')], shown, read);
  assert.deepEqual(m.rows.map((r) => r.issue.id), ['x', 'y']);
  assert.ok(m.read.has('x'));
  // a colleague comments on x: it comes back newer, so it's unread again, once
  m = mergeRows([row('x', '2026-10-17T11:00:00Z'), row('y', '2026-10-17T09:00:00Z')], m.rows, m.read);
  assert.deepEqual(m.rows.map((r) => r.issue.id), ['x', 'y']);
  assert.ok(!m.read.has('x'));
});

test('the side column says why you follow an issue, or that a project role brings only its new and closed', () => {
  assert.deepEqual(followLine({ following: true, reasons: ['assignee', 'commented'], set: null }), { state: 'Following', because: 'Because: assignee · commented' });
  assert.deepEqual(followLine({ following: false, reasons: ['lead-dev (project)'], set: null }), { state: 'New and closed only', because: 'As lead-dev of the project.' });
  assert.deepEqual(followLine({ following: false, reasons: [], set: null }), { state: 'Not following', because: "You're not in it." });
  assert.deepEqual(followLine({ following: true, reasons: ['followed'], set: true }), { state: 'Following', because: 'You follow it.' });
  assert.deepEqual(followLine({ following: false, reasons: ['assignee'], set: false }), { state: 'Not following', because: 'You unfollowed it.' });
});

test('the bell: its shape and tone from the follow line', () => {
  assert.deepEqual(bellState({ following: true, reasons: [], set: true }), { shape: 'bell', tone: 'on', label: 'Inbox: following. You follow it.' });
  assert.deepEqual(bellState({ following: false, reasons: [], set: false }), { shape: 'bell-off', tone: 'off', label: 'Inbox: not following. You unfollowed it.' });
  assert.equal(bellState({ following: true, reasons: ['assignee'], set: null }).label, 'Inbox: following. Because: assignee');
  assert.deepEqual(bellState({ following: false, reasons: ['pm (project)'], set: null }), { shape: 'bell', tone: 'part', label: 'Inbox: new and closed only. As pm of the project.' });
  assert.equal(bellState({ following: false, reasons: [], set: null }).shape, 'bell-off');
});
