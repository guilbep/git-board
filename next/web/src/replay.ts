// An issue's history replayed (the History mode of the issue panel): every
// change in time order, comment edits included, and the issue as it was
// after any of them. Pure: built from the thread the server already sends.
import type { IssueEvent, Person } from './api/board';
import { lineDiff, type DiffLine } from './linediff.ts';
import { isTick } from './sections.ts';

// the core's operation names (core/issue/ops.go)
export type Op = 'create' | 'comment' | 'edit-text' | 'change-labels' | 'set-status' | 'set-title' | 'set-meta';

export interface Step {
  kind: 'created' | 'comment' | 'edit' | 'labels' | 'status' | 'title' | 'meta';
  op: Op;
  id: string; // the operation's id, '' when the server sent none
  key: string;
  at: string;
  author: Person;
  event: IssueEvent; // the timeline item it belongs to (an edit: its comment)
  before?: string | undefined; // title: the old one; edit: the version it replaced
  after?: string | undefined; // title: the new one; created, comment, edit: the text
  added?: string[] | undefined;
  removed?: string[] | undefined;
  status?: 'open' | 'closed' | undefined;
  meta?: Record<string, string> | undefined; // meta: the keys it set
  version?: number | undefined; // the description's create and edits: the version it published, 0 for none (a tick)
  pending: string[]; // forges it hasn't reached
}

export interface CommentState {
  id: string;
  kind: 'created' | 'comment';
  author: Person;
  at: string;
  body: string;
  edits: number; // edits so far
}

export interface IssueState {
  title: string;
  status: 'open' | 'closed';
  labels: string[];
  comments: CommentState[];
}

const time = (s: Step) => Date.parse(s.at);

// steps flattens the timeline: a comment's edits become steps of their own,
// at their time. The latest edit carries edit_pending; an edit of a
// comment still waiting waits with it.
export function steps(events: IssueEvent[]): Step[] {
  const out: Step[] = [];
  for (const e of events) {
    const pending = e.pending ?? [];
    const base = { id: e.id, key: e.id, at: e.at, author: e.author, event: e, pending };
    switch (e.kind) {
      case 'created':
      case 'comment': {
        const versions = e.history?.length ? e.history : [{ id: e.id, author: e.author, at: e.at, body: e.body ?? '' }];
        const desc = e.kind === 'created';
        let v = 1;
        out.push({ ...base, kind: e.kind, op: desc ? 'create' : 'comment', after: versions[0]!.body, ...(desc ? { version: 1 } : {}) });
        versions.slice(1).forEach((x, i) => {
          const last = i === versions.length - 2;
          const before = versions[i]!.body;
          out.push({
            kind: 'edit', op: 'edit-text', id: x.id ?? '', key: `${e.id}#${i + 1}`, at: x.at, author: x.author, event: e,
            before, after: x.body, pending: pending.length ? pending : last ? (e.edit_pending ?? []) : [],
            ...(desc ? { version: x.body !== before && !isTick(before, x.body) ? ++v : 0 } : {}),
          });
        });
        // after the edits: an Apply's edit and its set-meta share a second
        (e.meta_history ?? []).forEach((m, j) =>
          out.push({ kind: 'meta', op: 'set-meta', id: m.id, key: `${e.id}@${j}`, at: m.at, author: m.author, event: e, meta: m.meta, pending: [] }));
        break;
      }
      case 'labels':
        out.push({ ...base, kind: 'labels', op: 'change-labels', added: e.added ?? [], removed: e.removed ?? [] });
        break;
      case 'status':
        out.push({ ...base, kind: 'status', op: 'set-status', status: e.status });
        break;
      case 'title':
        out.push({ ...base, kind: 'title', op: 'set-title', before: e.was, after: e.title });
        break;
    }
  }
  // stable: same time keeps the timeline's order
  // ponytail: the server sends times to the second, the log's order is
  // lost between texts; a meta on a later comment and an edit of an
  // earlier text in the same second keep the events' order
  return out.map((s, i) => [s, i] as const).sort(([a, i], [b, j]) => time(a) - time(b) || i - j).map(([s]) => s);
}

// asOf is the events as they were once steps[0..i] happened: the ones
// created by then, each text's history and body cut to its edits by then,
// its meta the keys no set-meta touched plus the sets by then. M2's and
// M3's rules (versions, bodyThreads, stateOf) read them unchanged.
// ponytail: a key the comment wrote and a set-meta changed later reads
// unset before that set; the server's first meta if that bites
export function asOf(all: Step[], i: number): IssueEvent[] {
  const done = new Set(all.slice(0, i + 1).map((s) => s.key));
  const events = [...new Set(all.map((s) => s.event))];
  return events.filter((e) => done.has(e.id)).map((e) => {
    if (e.kind !== 'created' && e.kind !== 'comment') return e;
    const h = e.history ?? [];
    const n = 1 + h.slice(1).filter((_, j) => done.has(`${e.id}#${j + 1}`)).length;
    const mh = e.meta_history ?? [];
    const touched = new Set(mh.flatMap((m) => Object.keys(m.meta)));
    const meta = Object.fromEntries(Object.entries(e.meta ?? {}).filter(([k]) => !touched.has(k)));
    mh.forEach((m, j) => done.has(`${e.id}@${j}`) && Object.assign(meta, m.meta));
    const cut = h.length > n ? h.slice(0, n) : h;
    const out: IssueEvent = { ...e, meta, history: cut, edited: cut.length > 1, meta_history: mh.filter((_, j) => done.has(`${e.id}@${j}`)) };
    if (cut !== h) Object.assign(out, { body: cut.at(-1)!.body, html: cut.at(-1)!.html ?? '' });
    return out;
  });
}

// metaWhat words a set-meta: a thread resolved, reopened or applied,
// else the keys it set
export function metaWhat(meta: Record<string, string> = {}): string {
  const r = meta['thread:resolved'];
  if (r === '') return 'reopened a thread';
  if (r) return meta['thread:applied'] ? 'applied a suggested edit' : 'resolved a thread';
  return `set ${Object.keys(meta).join(', ')}`;
}

// stateAfter is the issue once steps[0..i] happened. Its first title is
// the one the first rename replaced, else today's.
export function stateAfter(all: Step[], i: number, title: string): IssueState {
  const firstRename = all.find((s) => s.kind === 'title');
  const st: IssueState = { title: firstRename?.before ?? title, status: 'open', labels: [], comments: [] };
  const labels = new Set<string>();
  for (const s of all.slice(0, i + 1)) {
    switch (s.kind) {
      case 'created':
      case 'comment':
        st.comments.push({ id: s.event.id, kind: s.kind, author: s.author, at: s.at, body: s.after ?? '', edits: 0 });
        break;
      case 'edit': {
        const c = st.comments.find((x) => x.id === s.event.id);
        if (c) {
          c.body = s.after ?? '';
          c.edits++;
        }
        break;
      }
      case 'labels':
        s.removed?.forEach((l) => labels.delete(l));
        s.added?.forEach((l) => labels.add(l));
        break;
      case 'status':
        st.status = s.status ?? st.status;
        break;
      case 'title':
        st.title = s.after ?? st.title;
        break;
    }
  }
  st.labels = [...labels].sort();
  return st;
}

export type WordOp = { op: ' ' | '-' | '+'; text: string };

// wordDiff marks the words in and out between two texts (a word with the
// spaces after it), consecutive ones merged.
export function wordDiff(a: string, b: string): WordOp[] {
  const words = (s: string) => s.match(/\s+|\S+\s*/g) ?? [];
  const lines = lineDiff(words(a).join('\n'), words(b).join('\n'));
  const out: WordOp[] = [];
  for (const l of lines) {
    const last = out.at(-1);
    if (last && last.op === l.op) last.text += l.text;
    else out.push({ op: l.op, text: l.text });
  }
  return out;
}

// textDiff is a line diff whose changed lines carry their word diff: a
// line out then a line in, side by side, show the words that changed.
export type TextLine = DiffLine & { words?: WordOp[] };
export function textDiff(a: string, b: string): TextLine[] {
  const lines: TextLine[] = lineDiff(a, b);
  for (let i = 0; i + 1 < lines.length; i++) {
    const out = lines[i]!;
    const inn = lines[i + 1]!;
    if (out.op === '-' && inn.op === '+') {
      const w = wordDiff(out.text, inn.text);
      out.words = w.filter((t) => t.op !== '+');
      inn.words = w.filter((t) => t.op !== '-');
      i++;
    }
  }
  return lines;
}
