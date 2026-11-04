// Run with: node --test web/src/richtext.test.ts
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { refLinks, refTokens } from './richtext.ts';

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

// enhance's links, without a DOM: refLinks decides each token (#89)
const base = {
  refHref: (n: string) => `https://github.com/o/r/issues/${n}`,
  mentionHref: (login: string) => `https://github.com/${login}`,
};

test('refLinks: #n stays in the page, @name opens the forge in a new tab', () => {
  assert.deepEqual(refLinks('see #12, @ann', base), [
    { text: 'see ' },
    { text: '#12', href: 'https://github.com/o/r/issues/12', className: 'issue-ref', ref: '12' },
    { text: ', ' },
    { text: '@ann', href: 'https://github.com/ann', className: 'mention', target: '_blank', rel: 'noopener noreferrer' },
  ]);
});

test('refLinks: no ref and no mention leaves the node as it is', () => {
  assert.equal(refLinks('a plain line, a@b.c', base), null);
});

test('refLinks: an href that is not http(s), or none, keeps the token as text', () => {
  const evil = { refHref: () => 'javascript:alert(1)', mentionHref: () => 'data:text/html,x' };
  assert.deepEqual(refLinks('#3 @bob', evil), [{ text: '#3' }, { text: ' ' }, { text: '@bob' }]);
  assert.deepEqual(refLinks('#3', { refHref: () => undefined, mentionHref: () => undefined }), [{ text: '#3' }]);
});
