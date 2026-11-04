// Run with: node --test web/src/links.test.ts
import assert from 'node:assert/strict';
import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { test } from 'node:test';
import { forgeBase, safeHref } from './links.ts';

test('an https or http URL is kept', () => {
  assert.equal(safeHref('https://github.com/o/r/issues/12'), 'https://github.com/o/r/issues/12');
  assert.equal(safeHref('http://gitlab.example.org/g/p/-/issues/3'), 'http://gitlab.example.org/g/p/-/issues/3');
});

test('a javascript: URL is dropped, whatever its case or leading space', () => {
  assert.equal(safeHref("javascript:fetch('/api/config')//12"), undefined);
  assert.equal(safeHref('JavaScript:alert(1)'), undefined);
  assert.equal(safeHref(' \tjavascript:alert(1)'), undefined);
  assert.equal(safeHref('java\nscript:alert(1)'), undefined);
});

test('data:, vbscript: and file: URLs are dropped', () => {
  assert.equal(safeHref('data:text/html,<script>alert(1)</script>'), undefined);
  assert.equal(safeHref('vbscript:msgbox(1)'), undefined);
  assert.equal(safeHref('file:///etc/passwd'), undefined);
});

test('a relative URL, an empty one or none is dropped', () => {
  assert.equal(safeHref('/api/config'), undefined);
  assert.equal(safeHref('issues/12'), undefined);
  assert.equal(safeHref('//evil.example/x'), undefined);
  assert.equal(safeHref(''), undefined);
  assert.equal(safeHref(null), undefined);
  assert.equal(safeHref(undefined), undefined);
});

test('#123 and @name go to the forge issue’s repository and origin', () => {
  assert.deepEqual(forgeBase('https://github.com/o/r/issues/12'), {
    refs: 'https://github.com/o/r/issues/',
    people: 'https://github.com/',
  });
});

test('no base from a javascript: forge URL, so #123 stays text', () => {
  assert.equal(forgeBase("javascript:fetch('/api/x')//12"), null);
  assert.equal(forgeBase('data:text/html,12'), null);
  assert.equal(forgeBase('not a url 12'), null);
  assert.equal(forgeBase(undefined), null);
});

test('no #123 base from a URL whose path does not end with the number, @name still', () => {
  assert.deepEqual(forgeBase('https://github.com/o/r/issues/12?x=1'), { refs: null, people: 'https://github.com/' });
  assert.deepEqual(forgeBase('https://github.com/o/r/issues/12#c3'), { refs: null, people: 'https://github.com/' });
  assert.deepEqual(forgeBase('https://github.com/o/r'), { refs: null, people: 'https://github.com/' });
});

// every link to a URL from forge data (a pull request's, an issue's
// forges[].url, a conflict's) goes through safeHref: a template binding an
// href to some .url as it is would let a javascript: URL through (#89)
test('the pages bind no forge .url to an href without safeHref', () => {
  const bare: string[] = [];
  const walk = (dir: string) => {
    for (const e of readdirSync(dir, { withFileTypes: true })) {
      const p = join(dir, e.name);
      if (e.isDirectory()) walk(p);
      else if (p.endsWith('.vue'))
        for (const [, expr] of readFileSync(p, 'utf8').matchAll(/:href="([^"]*)"/g))
          if (/\.url\b/.test(expr!) && !expr!.startsWith('safeHref(')) bare.push(`${p}: ${expr}`);
    }
  };
  walk(join(import.meta.dirname, 'components'));
  walk(join(import.meta.dirname, 'pages'));
  assert.deepEqual(bare, []);
});
