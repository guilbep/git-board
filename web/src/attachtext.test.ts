// Run with: node --test web/src/attachtext.test.ts
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { fillPlaceholder, insertAt, placeholderFor } from './attachtext.ts';

test('the placeholder goes on a line of its own at the cursor', () => {
  assert.equal(insertAt('ab', 1, 'P'), 'a\nP\nb');
  assert.equal(insertAt('', 0, 'P'), 'P\n');
  assert.equal(insertAt('a\n', 2, 'P'), 'a\nP\n');
});

test('the uploaded file takes its placeholder’s place', () => {
  const p = placeholderFor('shot.png', 1);
  assert.equal(fillPlaceholder(`see\n${p}\nthere`, p, '![shot](https://cdn/x.png)'), 'see\n![shot](https://cdn/x.png)\nthere');
});

test('the Markdown is taken as it is, $& and all', () => {
  const p = placeholderFor('a.txt', 1);
  assert.equal(fillPlaceholder(p, p, '[a $& b](u)'), '[a $& b](u)');
});

test('its placeholder gone (the comment sent and the box cleared), the file is not dropped', () => {
  const p = placeholderFor('shot.png', 1);
  assert.equal(fillPlaceholder('', p, '![shot](u)'), '![shot](u)');
  assert.equal(fillPlaceholder('edited [Uploading shot', p, '![shot](u)'), 'edited [Uploading shot\n![shot](u)');
});

test('two uploads of one name have two placeholders', () => {
  assert.notEqual(placeholderFor('image.png', 1), placeholderFor('image.png', 2));
});
