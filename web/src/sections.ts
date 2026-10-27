// The issue body's sections (docs/design/time-flow/protocol.md): the
// rendered text cut at its headings, the slugs anchors name, the anchor
// grammar, the version count, and a suggestion applied to the Markdown.
// Pure: built from the thread the server already sends.
import type { IssueEvent, Person } from './api/board.ts';
import { lineDiff } from './linediff.ts';

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

// ticksOf lists the description's ticks: the edits that only flip
// checkboxes, with the criteria (1-based) they ticked and unticked
export function ticksOf(created: IssueEvent): { at: string; author: Person; ticked: number[]; unticked: number[] }[] {
  const boxes = (s: string) => [...s.matchAll(/^\s*[-*+]\s+\[([ xX])\]/gm)].map((m) => m[1] !== ' ');
  const h = created.history ?? [];
  return h.slice(1).flatMap((x, i) => {
    if (!isTick(h[i]!.body, x.body)) return [];
    const [b, a] = [boxes(h[i]!.body), boxes(x.body)];
    const flip = (on: boolean) => a.flatMap((v, k) => (v === on && b[k] !== on ? [k + 1] : []));
    return [{ at: x.at, author: x.author, ticked: flip(true), unticked: flip(false) }];
  });
}

// criteriaOf counts a text's checkbox lines, outside fences, and the ticked ones
export function criteriaOf(body: string): { n: number; ticked: number } {
  let fence = false;
  const out = { n: 0, ticked: 0 };
  for (const l of body.split('\n')) {
    if (/^\s*(```|~~~)/.test(l)) fence = !fence;
    const box = fence ? undefined : /^\s*[-*+]\s+\[([ xX])\]/.exec(l)?.[1];
    if (box) ((out.n++), box !== ' ' && out.ticked++);
  }
  return out;
}

// version is the body's: 1 + each edit that changes text and is no tick.
export function version(created: IssueEvent): number {
  const h = created.history ?? [];
  return 1 + h.slice(1).filter((v, i) => v.body !== h[i]!.body && !isTick(h[i]!.body, v.body)).length;
}

// The body's versions, v1 first: vK's text is its last entry before
// v(K+1), its ticks included, so the last is the current text; at and
// author are the version's publication.
export interface BodyVersion { n: number; id: string; author: Person; at: string; body: string; html: string; ticks: number }
export function versions(created: IssueEvent): BodyVersion[] {
  const h = created.history?.length ? created.history : [{ id: created.id, author: created.author, at: created.at, body: created.body ?? '', html: created.html }];
  const out: BodyVersion[] = [];
  h.forEach((x, i) => {
    const last = out.at(-1);
    const prev = h[i - 1]?.body;
    if (last && (x.body === prev || isTick(prev!, x.body))) {
      if (x.body !== prev) last.ticks++;
      Object.assign(last, { body: x.body, html: x.html ?? last.html });
    } else out.push({ n: out.length + 1, id: x.id ?? '', author: x.author, at: x.at, body: x.body, html: x.html ?? '', ticks: 0 });
  });
  out.at(-1)!.html = created.html ?? out.at(-1)!.html;
  return out;
}

// a section's HTML with its boxes unticked: a tick changes nothing
const plain = (h: string) => h.replace(/\s+checked(="[^"]*")?/g, '');
// the section's items, the text around them first: [rest, item 1, item 2…]
// ponytail: an item's fragment runs to the next <li>, a nested list's
// parent included; an item picker would walk the lists
function itemsOf(h: string): string[] {
  const f = h.split(/<li\b/);
  const last = f.length - 1;
  const cut = last > 0 ? f[last]!.lastIndexOf('</li>') : -1;
  if (cut >= 0) {
    f[0] += '\0' + f[last]!.slice(cut);
    f[last] = f[last]!.slice(0, cut);
  }
  return f;
}

// changeOf: `after`'s section against the version before, by slug,
// ticks not counted; the item's number when only it differs.
export function changeOf(before: Section[], after: Section): 'unchanged' | 'changed' | number {
  const b = before.find((s) => s.slug === after.slug);
  if (!b) return 'changed';
  if (plain(b.html) === plain(after.html)) return 'unchanged';
  const p = itemsOf(plain(b.html));
  const q = itemsOf(plain(after.html));
  const diff = q.flatMap((x, i) => (x !== p[i] ? [i] : []));
  return p.length === q.length && diff.length === 1 && diff[0]! > 0 ? diff[0]! : 'changed';
}

// A body thread's state against the current version (protocol.md):
// resolved, current (written on it), carried (its section unchanged
// since), outdated since the first version its section differs in (a
// gone section included).
export type ThreadState = { state: 'resolved' | 'current' | 'carried' } | { state: 'outdated'; since: number };
export function stateOf(root: IssueEvent, vs: BodyVersion[]): ThreadState {
  if (root.meta?.['thread:resolved']) return { state: 'resolved' };
  const a = anchorOf(root.meta?.['thread:anchor']);
  if (!a || a.v >= vs.length) return { state: 'current' };
  const at = (v: number) => sectionsOf(vs[v - 1]!.html).find((s) => s.slug === a.slug);
  const base = at(a.v);
  for (let k = a.v + 1; k <= vs.length; k++) {
    const s = at(k);
    if (!s || !base || plain(s.html) !== plain(base.html)) return { state: 'outdated', since: k };
  }
  return { state: 'carried' };
}

// diffMarkdown is `after` with the lines removed since `before` struck
// and the added ones marked, inside their quote, list and checkbox
// markers; ticks compared away. A heading added is the new heading,
// plain; a removed one is dropped, its struck lines joining the section
// above. ponytail: lines, not words; inside a fence or a table line
// nothing is marked (an added line shows, a removed one is dropped).
const MARKS = /^(\s*(?:>\s*)*(?:(?:[-*+]|\d+[.)])\s+)?(?:\[[ xX]\]\s+)?)(.*)$/;
const RAW = /^ {0,3}(#{1,6}(\s|$)|(=+|-+)\s*$)|^\s*\|/;
export function diffMarkdown(before: string, after: string): string {
  const a = before.split('\n');
  const b = after.split('\n');
  const out: string[] = [];
  let fenced = false;
  const mark = (line: string, tag: 'ins' | 'del') => {
    const m = MARKS.exec(line)!;
    if (fenced || RAW.test(line) || FENCE.test(line) || !m[2]!.trim()) return tag === 'ins' ? line : undefined;
    return `${m[1]}<${tag}>${m[2]}</${tag}>`;
  };
  let i = 0;
  let j = 0;
  // the diff of the unticked texts, each line emitted as the texts have it
  for (const d of lineDiff(unticked(before), unticked(after))) {
    const line = d.op === '-' ? mark(a[i++]!, 'del') : d.op === '+' ? mark(b[j++]!, 'ins') : (i++, b[j++]!);
    if (line === undefined) continue;
    out.push(line);
    if (d.op !== '-' && FENCE.test(line)) fenced = !fenced;
  }
  return out.join('\n');
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
