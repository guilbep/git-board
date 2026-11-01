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

test('wrong value types are refused', () => {
  assert.ok(!tomlToField('single = "yes"\n').ok);
  assert.ok(!tomlToField('options = [1, 2]\n').ok);
  assert.ok(!tomlToField('colors = "green"\n').ok);
});
