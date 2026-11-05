import { test } from 'node:test';
import assert from 'node:assert/strict';
import { tomlTokens } from './toml.ts';

const show = (line: string) => tomlTokens(line).map((t) => (t.cls ? `${t.cls}:${t.text}` : t.text)).join('|');

test('tomlTokens colors what .git-board.toml holds', () => {
  assert.equal(show('[fields.status]'), 'table:[fields.status]');
  assert.equal(show('[[sprints.list]]  # one sprint'), 'table:[[sprints.list]]|  |comment:# one sprint');
  assert.equal(show('options = ["backlog", "in review"]'), 'key:options| |punct:=| |punct:[|string:"backlog"|punct:,| |string:"in review"|punct:]');
  assert.equal(show('start = 2026-10-05'), 'key:start| |punct:=| |value:2026-10-05');
  assert.equal(show('single = true'), 'key:single| |punct:=| |value:true');
  assert.equal(show('project = { comite = "Comité" }'), 'key:project| |punct:=| |punct:{| |key:comite |punct:=| |string:"Comité"| |punct:}');
  assert.equal(show('"in review" = "purple"'), 'key:"in review"| |punct:=| |string:"purple"');
  assert.equal(show('# a note'), 'comment:# a note');
  assert.equal(show(''), '');
});
