// Issue text arrives as clean HTML from the server (internal/markdown).
// This adds what needs the page: #123 and @name as links, and the image
// proxy's address in dev. DOM calls only, never HTML strings: the server
// stays the one place text becomes HTML.

import { safeHref } from './links.ts';

export type RefToken = { text: string; ref?: string; mention?: string };

// #123 not glued to a word; @name as git-bug's sync recognises it
// (bridge/core/render.go mentionRe): not in an email or a path, not ending
// with punctuation.
const refRe = /(^|[^\w&/#])#(\d+)\b|(^|[^\w.\-/@`])@([A-Za-z0-9_](?:[A-Za-z0-9_.-]*[A-Za-z0-9_])?)/g;

export function refTokens(text: string): RefToken[] {
  const out: RefToken[] = [];
  let last = 0;
  for (const m of text.matchAll(refRe)) {
    const lead = m[1] ?? m[3] ?? '';
    const start = m.index + lead.length;
    if (start > last) out.push({ text: text.slice(last, start) });
    if (m[2] !== undefined) out.push({ ref: m[2], text: `#${m[2]}` });
    else out.push({ mention: m[4] ?? "", text: `@${m[4] ?? ""}` });
    last = m.index + m[0].length;
  }
  if (last < text.length || out.length === 0) out.push({ text: text.slice(last) });
  return out;
}

export interface EnhanceOptions {
  refHref: (n: string) => string | undefined; // where #n goes, undefined: stays text
  mentionHref: (login: string) => string | undefined;
  mediaURL: (path: string) => string; // /api/media?… → the API's origin
}

// RefLink is a token as enhance writes it: text, or a link with its
// attributes. A #ref stays in the page (the board's own handler opens it),
// a mention opens the forge in a new tab.
export type RefLink =
  | { text: string; href?: undefined }
  | { text: string; href: string; className: 'issue-ref'; ref: string }
  | { text: string; href: string; className: 'mention'; target: '_blank'; rel: 'noopener noreferrer' };

// refLinks is enhance's pure part: a text's tokens, each a link only with
// an http(s) href, whatever the caller built it from (links.ts). null when
// the text has no ref and no mention: the node stays as it is. Tested in
// node without a DOM (richtext.test.ts), so no DOM library is needed
// (ADR 0017).
export function refLinks(text: string, opts: Pick<EnhanceOptions, 'refHref' | 'mentionHref'>): RefLink[] | null {
  const tokens = refTokens(text);
  if (tokens.length === 1 && !tokens[0]!.ref && !tokens[0]!.mention) return null;
  return tokens.map((t): RefLink => {
    const href = safeHref(t.ref ? opts.refHref(t.ref) : t.mention ? opts.mentionHref(t.mention) : undefined);
    if (!href) return { text: t.text };
    if (t.ref) return { text: t.text, href, className: 'issue-ref', ref: t.ref };
    return { text: t.text, href, className: 'mention', target: '_blank', rel: 'noopener noreferrer' };
  });
}

const skipped = new Set(['A', 'CODE', 'PRE']);

// enhance links the refs and mentions in el's text, and fixes image sources.
export function enhance(el: HTMLElement, opts: EnhanceOptions): void {
  for (const img of el.querySelectorAll('img')) {
    const src = img.getAttribute('src') ?? '';
    if (src.startsWith('/api/media')) img.src = opts.mediaURL(src);
    img.loading = 'lazy';
  }
  const walker = document.createTreeWalker(el, NodeFilter.SHOW_TEXT, {
    acceptNode: (n) => {
      for (let p = n.parentElement; p && p !== el; p = p.parentElement) {
        if (skipped.has(p.tagName)) return NodeFilter.FILTER_REJECT;
      }
      return /[#@]/.test(n.nodeValue ?? '') ? NodeFilter.FILTER_ACCEPT : NodeFilter.FILTER_REJECT;
    },
  });
  const nodes: Text[] = [];
  while (walker.nextNode()) nodes.push(walker.currentNode as Text);
  for (const node of nodes) {
    const links = refLinks(node.nodeValue ?? '', opts);
    if (!links) continue;
    const frag = document.createDocumentFragment();
    for (const l of links) {
      if (!l.href) {
        frag.append(l.text);
        continue;
      }
      const a = document.createElement('a');
      a.href = l.href;
      a.textContent = l.text;
      a.className = l.className;
      if (l.className === 'issue-ref') a.dataset.ref = l.ref;
      else {
        a.target = l.target;
        a.rel = l.rel;
      }
      frag.append(a);
    }
    node.replaceWith(frag);
  }
}
