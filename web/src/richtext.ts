// Issue text arrives as clean HTML from the server (internal/markdown).
// This adds what needs the page: #123 and @name as links, and the image
// proxy's address in dev. DOM calls only, never HTML strings: the server
// stays the one place text becomes HTML.

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
    const tokens = refTokens(node.nodeValue ?? '');
    if (tokens.length === 1 && !tokens[0]!.ref && !tokens[0]!.mention) continue;
    const frag = document.createDocumentFragment();
    for (const t of tokens) {
      const href = t.ref ? opts.refHref(t.ref) : t.mention ? opts.mentionHref(t.mention) : undefined;
      if (!href) {
        frag.append(t.text);
        continue;
      }
      const a = document.createElement('a');
      a.href = href;
      a.textContent = t.text;
      if (t.ref) {
        a.className = 'issue-ref';
        a.dataset.ref = t.ref;
      } else {
        a.className = 'mention';
        a.target = '_blank';
        a.rel = 'noopener noreferrer';
      }
      frag.append(a);
    }
    node.replaceWith(frag);
  }
}
