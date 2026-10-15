// Run with: node --test web/src/richtext.test.ts
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { refTokens } from './richtext.ts';

test('#refs and @mentions become tokens, the rest stays text', () => {
  assert.deepEqual(refTokens('see #12 and @alice-b, not a#3 or me@example.com'), [
    { text: 'see ' },
    { ref: '12', text: '#12' },
    { text: ' and ' },
    { mention: 'alice-b', text: '@alice-b' },
    { text: ', not a#3 or me@example.com' },
  ]);
});

test('text without refs is one token', () => {
  assert.deepEqual(refTokens('plain text'), [{ text: 'plain text' }]);
});

test('a mention does not end with punctuation', () => {
  assert.deepEqual(refTokens('thanks @bob.'), [{ text: 'thanks ' }, { mention: 'bob', text: '@bob' }, { text: '.' }]);
});
