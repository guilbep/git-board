// An issue's history replayed (the History mode of the issue panel): every
// change in time order, comment edits included, and the issue as it was
// after any of them. Pure: built from the thread the server already sends.
import type { IssueEvent, Person } from './api/board';
import { lineDiff, type DiffLine } from './linediff.ts';

export interface Step {
  kind: 'created' | 'comment' | 'edit' | 'labels' | 'status' | 'title';
  key: string;
  at: string;
  author: Person;
  event: IssueEvent; // the timeline item it belongs to (an edit: its comment)
  before?: string | undefined; // title: the old one; edit: the version it replaced
  after?: string | undefined; // title: the new one; created, comment, edit: the text
  added?: string[] | undefined;
  removed?: string[] | undefined;
  status?: 'open' | 'closed' | undefined;
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
    switch (e.kind) {
      case 'created':
      case 'comment': {
        const versions = e.history?.length ? e.history : [{ author: e.author, at: e.at, body: e.body ?? '' }];
        out.push({ kind: e.kind, key: e.id, at: e.at, author: e.author, event: e, after: versions[0]!.body, pending });
        versions.slice(1).forEach((v, i) => {
          const last = i === versions.length - 2;
          out.push({
            kind: 'edit', key: `${e.id}#${i + 1}`, at: v.at, author: v.author, event: e,
            before: versions[i]!.body, after: v.body, pending: pending.length ? pending : last ? (e.edit_pending ?? []) : [],
          });
        });
        break;
      }
      case 'labels':
        out.push({ kind: 'labels', key: e.id, at: e.at, author: e.author, event: e, added: e.added ?? [], removed: e.removed ?? [], pending });
        break;
      case 'status':
        out.push({ kind: 'status', key: e.id, at: e.at, author: e.author, event: e, status: e.status, pending });
        break;
      case 'title':
        out.push({ kind: 'title', key: e.id, at: e.at, author: e.author, event: e, before: e.was, after: e.title, pending });
        break;
    }
  }
  // stable: same time keeps the timeline's order
  return out.map((s, i) => [s, i] as const).sort(([a, i], [b, j]) => time(a) - time(b) || i - j).map(([s]) => s);
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
