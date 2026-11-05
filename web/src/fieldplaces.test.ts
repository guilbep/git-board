import { test } from 'node:test';
import assert from 'node:assert/strict';
import type { ForgeModel } from './fieldtargets.ts';
import { choose, placesBody, placesOf, placesSummary, placesToml, rename } from './fieldplaces.ts';

const gh = (fields: ForgeModel['fields'] = {}): ForgeModel => ({ name: 'github', kind: 'github', project: 'o/r', host: '', sync: true, fields });

test('choose: the default again is no override; a Project column gets a name', () => {
  assert.equal(choose(undefined, 'label', 'label', 'prio'), null);
  assert.deepEqual(choose(undefined, 'project', 'label', 'prio'), { to: 'project', field: 'Priority' });
  assert.deepEqual(choose({ to: 'project', field: 'Prio' }, 'project', 'label', 'prio'), { to: 'project', field: 'Prio' });
  // renames keep an override alive, even on the default target
  assert.deepEqual(choose({ to: 'project', values: { p0: 'Urgent' } }, 'label', 'label', 'prio'), { to: 'label', values: { p0: 'Urgent' } });
});

test('rename: empty or the same name is no rename', () => {
  const m = rename(undefined, 'label', 'p0', 'Urgent');
  assert.deepEqual(m, { to: 'label', values: { p0: 'Urgent' } });
  assert.deepEqual(rename(m, 'label', 'p0', ''), { to: 'label' });
  assert.deepEqual(rename(m, 'label', 'p0', 'p0'), { to: 'label' });
});

test('placesBody: only what changed, per forge; a removed field is the server’s', () => {
  const saved = [gh({ status: { to: 'project', field: 'Status' }, est: { to: 'project', field: 'Est. hours' } })];
  const draft = placesOf(saved);
  assert.equal(placesBody(saved, draft), undefined);
  draft.github!.prio = { to: 'project', field: 'Priority' };
  delete draft.github!.status;
  assert.deepEqual(placesBody(saved, draft), { github: { set: { prio: { to: 'project', field: 'Priority' } }, remove: ['status'] } });
  assert.deepEqual(placesBody(saved, draft, ['status']), { github: { set: { prio: { to: 'project', field: 'Priority' } }, remove: [] } });
  // placesOf copies: the draft's change leaves the saved model alone
  assert.equal(saved[0]!.fields!.prio, undefined);
});

test('placesToml: the field’s tables on each forge', () => {
  const t = placesToml('prio', { github: { prio: { to: 'project', field: 'Priority', values: { p0: 'Urgent' } } }, lab: {} });
  assert.equal(t, '[forges.github.fields]\nprio = { to = "project", field = "Priority" }\n\n[forges.github.fields.prio.values]\np0 = "Urgent"');
  assert.equal(placesToml('due', { github: {} }), '');
});

test('placesSummary: the most common place counted, the others named', () => {
  const fields = { status: { single: true, options: ['a', 'b'] }, kind: { single: true, options: ['x'] }, moscow: { single: true, options: ['m'] }, prio: { single: true, options: ['p0'] } };
  const s = placesSummary(gh({ prio: { to: 'project', field: 'Priority' } }), fields, ['status', 'kind', 'moscow', 'prio'], [], 0);
  assert.equal(s, '3 as labels · prio in the Project');
});
