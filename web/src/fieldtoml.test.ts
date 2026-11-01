import assert from 'node:assert/strict';
import { test } from 'node:test';
import { fieldToToml, tomlToField } from './fieldtoml.ts';

test('a field round-trips through its TOML', () => {
  const f = {
    single: true,
    numeric: false,
    type: 'select',
    options: ['todo', 'doing', 'done'],
    colors: { todo: 'grey', done: 'green' },
  };
  const r = tomlToField(fieldToToml('status', f));
  assert.ok(r.ok);
  if (r.ok) {
    assert.equal(r.name, 'status');
    assert.deepEqual(r.field, { single: true, numeric: false, type: 'select', options: ['todo', 'doing', 'done'], colors: { todo: 'grey', done: 'green' } });
  }
});

test('a bare table (no header) names nothing', () => {
  const r = tomlToField('single = false\noptions = ["a", "b"]\n');
  assert.ok(r.ok);
  if (r.ok) assert.equal(r.name, undefined);
});

test('a quoted table header renames the field', () => {
  const r = tomlToField('[fields."my field"]\nsingle = true\n');
  assert.ok(r.ok);
  if (r.ok) assert.equal(r.name, 'my field');
});

test('empty keys and unnamed fields serialize to something parseable', () => {
  const r = tomlToField(fieldToToml('', { single: true }));
  assert.ok(r.ok);
  if (r.ok) assert.equal(r.name, 'new-field');
});

test('an unparsable snippet reports the error', () => {
  const r = tomlToField('single = [1,\n');
  assert.deepEqual(r.ok, false);
});

test('unknown keys are refused, not dropped', () => {
  const r = tomlToField('[fields.status]\ncolour = "green"\n');
  assert.ok(!r.ok);
  if (!r.ok) assert.match(r.error, /colour/);
});

test('several field tables are refused', () => {
  const r = tomlToField('[fields.a]\n[fields.b]\n');
  assert.ok(!r.ok);
});

test('a list of tables is refused, not read as a rename to 0', () => {
  const r = tomlToField('[[fields.status]]\nsingle = true\n');
  assert.ok(!r.ok);
  if (!r.ok) assert.match(r.error, /list of tables/);
});

test('content outside the field table is refused, not ignored', () => {
  for (const text of ['foo = 1\n[fields.status]\nsingle = true\n', '[labels]\nx = "y"\n[fields.status]\n']) {
    const r = tomlToField(text);
    assert.ok(!r.ok);
    if (!r.ok) assert.match(r.error, /outside it/);
  }
});

test('wrong value types are refused', () => {
  assert.ok(!tomlToField('single = "yes"\n').ok);
  assert.ok(!tomlToField('options = [1, 2]\n').ok);
  assert.ok(!tomlToField('colors = "green"\n').ok);
});

test('aliases round-trip, and an unknown key is still refused', () => {
  const text = fieldToToml('moscow', { single: true, options: ['couldhave', 'wonthave'], aliases: { could: 'couldhave' } });
  const r = tomlToField(text);
  assert.ok(r.ok);
  assert.deepEqual(r.ok && r.field.aliases, { could: 'couldhave' });
  const bad = tomlToField('[fields.x]\nalso = ["label"]\n');
  assert.ok(!bad.ok);
  assert.match(bad.ok ? '' : bad.error, /aliases, project/);
});
