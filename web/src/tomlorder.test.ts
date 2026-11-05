// Run with: node --test src/tomlorder.test.ts
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { reorderTables } from './tomlorder.ts';

const file = `# the board

[fields.status]
single = true

# priorities
[fields.prio]
options = ["p0"]

[fields.prio.colors]
p0 = "red"

[views.x]
filter = ""
`;

test('a parent’s tables in a new order, their sub-tables and comments with them', () => {
  assert.equal(reorderTables(file, 'fields', ['prio', 'status']), `# the board

# priorities
[fields.prio]
options = ["p0"]

[fields.prio.colors]
p0 = "red"

[fields.status]
single = true

[views.x]
filter = ""
`);
});

test('the same order changes nothing', () => {
  assert.equal(reorderTables(file, 'fields', ['status', 'prio']), file);
});
