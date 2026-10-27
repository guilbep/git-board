// The issue body's sections (docs/design/time-flow/protocol.md): the
// rendered text cut at its headings, the slugs anchors name, the anchor
// grammar, the version count, and a suggestion applied to the Markdown.
// Pure: built from the thread the server already sends.
import type { IssueEvent } from './api/board.ts';

export interface Section { n: number; slug: string; heading: string; html: string }

const ENTITIES: Record<string, string> = { amp: '&', lt: '<', gt: '>', quot: '"', apos: "'", nbsp: ' ' };
const decode = (s: string) => s.replace(/&(#x[0-9a-f]+|#\d+|\w+);/gi, (m, e: string) =>
  e[0] !== '#' ? (ENTITIES[e.toLowerCase()] ?? m) : String.fromCodePoint(e[1] === 'x' || e[1] === 'X' ? parseInt(e.slice(2), 16) : Number(e.slice(1))));
const textOf = (html: string) => decode(html.replace(/<[^>]*>/g, '')).trim();

// slugs: lower case, runs of other characters as one "-", an index on a
// repeat ("expected", "expected-2"). `taken` holds slugs already used.
export function slugs(headings: string[], taken: string[] = []): string[] {
  const seen = new Map<string, number>(taken.map((t) => [t, 1]));
  return headings.map((h) => {
    const base = h.toLowerCase().replace(/[^\p{L}\p{N}]+/gu, '-').replace(/^-|-$/g, '') || 'section';
    const n = (seen.get(base) ?? 0) + 1;
    seen.set(base, n);
    return n === 1 ? base : `${base}-${n}`;
  });
}

// sectionsOf cuts the rendered body at its h1–h6 outside any blockquote:
// the prose before the first heading is §1 "intro"; no heading, one
// section. The only place a slug is computed.
export function sectionsOf(html: string): Section[] {
  const cuts: { at: number; heading: string }[] = [];
  let depth = 0;
  for (const m of html.matchAll(/<(\/?)(blockquote|h[1-6])\b[^>]*>/gi)) {
    const tag = m[2]!.toLowerCase();
    if (tag === 'blockquote') depth += m[1] ? -1 : 1;
    else if (!m[1] && depth === 0) {
      const close = html.indexOf(`</${tag}>`, m.index);
      cuts.push({ at: m.index, heading: textOf(html.slice(m.index + m[0].length, close < 0 ? undefined : close)) });
    }
  }
  const intro = html.slice(0, cuts[0]?.at ?? html.length);
  const parts = [
    ...(intro.trim() || !cuts.length ? [{ heading: '', html: intro }] : []),
    ...cuts.map((c, i) => ({ heading: c.heading, html: html.slice(c.at, cuts[i + 1]?.at ?? html.length) })),
  ];
  const hasIntro = parts[0]!.heading === '';
  const named = slugs(parts.filter((p) => p.heading !== '').map((p) => p.heading), hasIntro ? ['intro'] : []);
  return parts.map((p, i) => ({ n: i + 1, slug: hasIntro && i === 0 ? 'intro' : named[i - (hasIntro ? 1 : 0)]!, ...p }));
}

export interface Anchor { text: 'body'; v: number; slug: string; item?: number }

// anchorOf reads `body@v<n>/<section>[#<item>]`; plan anchors are later.
export function anchorOf(s: string | undefined): Anchor | undefined {
  const m = /^body@v([1-9]\d*)\/([^/#\s]+)(?:#([1-9]\d*))?$/.exec(s ?? '');
  if (!m) return undefined;
  return { text: 'body', v: Number(m[1]), slug: m[2]!, ...(m[3] ? { item: Number(m[3]) } : {}) };
}

export const anchorString = (a: Anchor): string => `${a.text}@v${a.v}/${a.slug}${a.item ? `#${a.item}` : ''}`;

// anchorLabel is "§3 › 3", "§3"; a slug no section has keeps its name.
export function anchorLabel(a: Anchor, sections: Section[]): string {
  const s = sections.find((x) => x.slug === a.slug);
  return (s ? `§${s.n}` : `§ ${a.slug}`) + (a.item ? ` › ${a.item}` : '');
}

// isTick: the two texts differ only by "- [ ]" ↔ "- [x]" flips.
const unticked = (s: string) => s.replace(/^(\s*[-*+]\s+)\[[ xX]\]/gm, '$1[ ]');
export const isTick = (before: string, after: string): boolean => before !== after && unticked(before) === unticked(after);

// version is the body's: 1 + each edit that changes text and is no tick.
export function version(created: IssueEvent): number {
  const h = created.history ?? [];
  return 1 + h.slice(1).filter((v, i) => v.body !== h[i]!.body && !isTick(h[i]!.body, v.body)).length;
}

// The Markdown source's spans of `sections`: heading lines paired with
// the rendered headings by order. start is the heading's first line (0
// for intro), body the first line after it, end exclusive.
export interface MdSection { slug: string; start: number; body: number; end: number }

const ITEM = /^(\s*(?:[-*+]|\d+[.)])\s+(?:\[[ xX]\]\s+)?)(.*)$/;
const FENCE = /^ {0,3}(```|~~~)/;
const QUOTE = /^ {0,3}>/;

function headingLines(lines: string[]): { start: number; body: number }[] {
  const out: { start: number; body: number }[] = [];
  let fenced = false;
  lines.forEach((l, i) => {
    if (FENCE.test(l)) fenced = !fenced;
    if (fenced || FENCE.test(l) || QUOTE.test(l)) return;
    if (/^ {0,3}#{1,6}(\s|$)/.test(l)) out.push({ start: i, body: i + 1 });
    else if (i > 0 && /^ {0,3}(=+|-+)\s*$/.test(l)) {
      const p = lines[i - 1]!;
      // ponytail: a one-line setext title; a multi-line one starts a line late
      if (p.trim() && !ITEM.test(p) && !QUOTE.test(p) && !/^ {0,3}#/.test(p) && !out.some((o) => o.start === i - 1)) out.push({ start: i - 1, body: i + 1 });
    }
  });
  return out;
}

export function mdSections(md: string, sections: Section[]): MdSection[] | undefined {
  const lines = md.split('\n');
  const heads = headingLines(lines);
  const named = sections.filter((s) => s.heading !== '');
  if (heads.length !== named.length) return undefined;
  const out: MdSection[] = named.map((s, i) => ({ slug: s.slug, ...heads[i]!, end: heads[i + 1]?.start ?? lines.length }));
  if (sections[0]?.heading === '') out.unshift({ slug: 'intro', start: 0, body: 0, end: heads[0]?.start ?? lines.length });
  return out;
}

// the anchored item's line index, or the section's span
function locate(md: string, sections: Section[], a: Anchor) {
  const lines = md.split('\n');
  const s = mdSections(md, sections)?.find((x) => x.slug === a.slug);
  if (!s) return undefined;
  if (!a.item) return { lines, s, line: -1 };
  let fenced = false;
  let n = 0;
  for (let i = s.body; i < s.end; i++) {
    if (FENCE.test(lines[i]!)) fenced = !fenced;
    // ponytail: a nested item counts; an item picker would count per list
    else if (!fenced && ITEM.test(lines[i]!) && ++n === a.item) return { lines, s, line: i };
  }
  return undefined;
}

// lineAt is the anchored text: the item's (marker stripped), or, with no
// item, the section's text after its heading: what Apply would replace.
export function lineAt(md: string, sections: Section[], a: Anchor): string | undefined {
  const at = locate(md, sections, a);
  if (!at) return undefined;
  if (at.line >= 0) return ITEM.exec(at.lines[at.line]!)![2];
  return at.lines.slice(at.s.body, at.s.end).join('\n').trim();
}

// suggestionOf is the ```suggestion block's text.
export function suggestionOf(body: string): string | undefined {
  const m = /^```suggestion[ \t]*\n([\s\S]*?)\n?^```[ \t]*$/m.exec(body);
  return m ? m[1] : undefined;
}

// applySuggestion replaces the anchored item's text (its marker kept) or
// the section's text after its heading; undefined when the anchor names
// nothing or the two texts' headings do not pair.
export function applySuggestion(md: string, sections: Section[], a: Anchor, replacement: string): string | undefined {
  const at = locate(md, sections, a);
  if (!at) return undefined;
  const { lines, s, line } = at;
  if (line >= 0) return [...lines.slice(0, line), ITEM.exec(lines[line]!)![1] + replacement, ...lines.slice(line + 1)].join('\n');
  const text = [...(s.body > 0 ? [''] : []), ...replacement.split('\n'), ...(s.end < lines.length ? [''] : [])];
  return [...lines.slice(0, s.body), ...text, ...lines.slice(s.end)].join('\n');
}
