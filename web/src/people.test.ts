import { test } from 'node:test';
import assert from 'node:assert/strict';
import { peopleLabel, peopleOf, personByLogin } from './people.ts';
import type { IssueEvent, Person } from './api/types.ts';

const a: Person = { id: 'a', name: 'Person A' };
const b: Person = { id: 'b', name: 'Person B' };
const c: Person = { id: 'c', name: 'Person C' };
const comment = (author: Person, at: string): IssueEvent => ({ kind: 'comment', id: `${author.id}-${at}`, author, at });

test('the author first, then the commenters by their latest comment, newest first', () => {
  const events = [comment(b, '2026-10-01T10:00:00Z'), comment(c, '2026-10-02T10:00:00Z'), comment(b, '2026-10-03T10:00:00Z')];
  assert.deepEqual(peopleOf(a, events, '').map((r) => [r.person.name, r.role]), [['Person A', 'opened it'], ['Person B', 'commented'], ['Person C', 'commented']]);
});

test('the author commenting stays first, once; the viewer is marked', () => {
  const rows = peopleOf(a, [comment(a, '2026-10-05T10:00:00Z'), comment(c, '2026-10-04T10:00:00Z')], 'c');
  assert.deepEqual(rows.map((r) => [r.person.id, r.you]), [['a', false], ['c', true]]);
});

test('the label', () => {
  assert.equal(peopleLabel(1), '1 in this conversation');
  assert.equal(peopleLabel(7), '7 in this conversation');
});

test('personByLogin: a person field names a person by login, case and @ aside', () => {
  const ids: Person[] = [{ id: 'x', name: 'Alice Martin', login: 'alice', avatar_url: 'https://avatars.githubusercontent.com/u/1?s=88' }, { id: 'y', name: 'No Login' }];
  assert.equal(personByLogin(ids, '@Alice')?.id, 'x');
  assert.equal(personByLogin(ids, 'alice')?.avatar_url, 'https://avatars.githubusercontent.com/u/1?s=88');
  assert.equal(personByLogin(ids, 'bob'), undefined);
  assert.equal(personByLogin(null, 'alice'), undefined);
});
