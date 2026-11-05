// The issue body's sections (docs/design/time-flow/protocol.md): the
// rendered text cut at its headings, the slugs anchors name, the anchor
// grammar, the version count, and a suggestion applied to the Markdown.
// Pure: built from the thread the server already sends.
import type { IssueEvent, Person } from './api/types.ts';
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

// anchorLabel is "§3 › 3", "§3"; a slug no section has keeps its name.
export function anchorLabel(a: Anchor, sections: Section[]): string {
  const s = sections.find((x) => x.slug === a.slug);
  return (s ? `§${s.n}` : `§ ${a.slug}`) + (a.item ? ` › ${a.item}` : '');
}

// isTick: the two texts differ only by "- [ ]" ↔ "- [x]" flips.
const unticked = (s: string) => s.replace(/^(\s*[-*+]\s+)\[[ xX]\]/gm, '$1[ ]');
export const isTick = (before: string, after: string): boolean => before !== after && unticked(before) === unticked(after);

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
