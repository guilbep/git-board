// The issue's flow (docs/prd/time-flow.md): its whole life as one column,
// cut into a segment per step, and the readings a person picks of it.
// Pure: built from the thread the server already sends.
import type { CycleStages, IssueEvent } from './api/board.ts';
import { STEPS } from './cycle.ts';
import { threadsOf } from './review.ts';

export interface Segment { n: number; entries: IssueEvent[] }

const time = (e: IssueEvent) => Date.parse(e.at);

// segments cuts the flow at the operations that opened each step (call
// 3): a status move, the risk set at ready, the close. A comment goes to
// the step it carries (cycle:step), a line comment without one to Review,
// anything else to the step running when it was written; after the last
// move, the issue's own step. Every step from 1 to the highest reached
// shows, empty ones included, minus the skipped ones. No cycle (step 0):
// one segment, n 0. ponytail: a step opened by evidence alone (a PR, a
// test run) shows only through the comments that carry it; the server
// replaying internal/cycle over the operations if that misleads.
export function segments(events: IssueEvent[], stages: CycleStages | undefined, step: number, skipped: number[] = []): Segment[] {
  const sorted = events.filter((e) => e.kind !== 'created').map((e, i) => [e, i] as const)
    .sort(([a, i], [b, j]) => time(a) - time(b) || i - j).map(([e]) => e);
  if (!stages || step === 0) return [{ n: 0, entries: sorted }];
  const up = (n: number) => {
    while (n < 7 && skipped.includes(n)) n++;
    return n;
  };
  const of = (s: string) => {
    if (s === stages.ready) return 2;
    if (stages.in_progress.includes(s)) return 4;
    if (stages.in_review.includes(s)) return 6;
    return stages.validation.includes(s) || s === stages.closing ? 7 : 1;
  };
  const labels = new Set<string>();
  const moves = sorted.map((e) => {
    e.removed?.forEach((l) => labels.delete(l));
    e.added?.forEach((l) => labels.add(l));
    const risk = [...labels].some((l) => l.startsWith('risk:'));
    const status = e.added?.find((l) => l.startsWith('status:'));
    if (status) return of(status.slice(7)) === 2 && risk ? 3 : of(status.slice(7));
    if (e.added?.some((l) => l.startsWith('risk:')) && labels.has(`status:${stages.ready}`)) return 3;
    return e.kind === 'status' && e.status === 'closed' ? 7 : undefined;
  });
  const last = moves.findLastIndex((m) => m !== undefined);
  const byStep = new Map<number, IssueEvent[]>();
  let running = 1;
  sorted.forEach((e, i) => {
    let n = running;
    const carried = e.meta?.['cycle:step'];
    if (moves[i] !== undefined) n = running = up(moves[i]);
    else if (carried) running = Math.max(running, (n = up(Number(carried))));
    else if (e.review) n = up(6);
    if (i === last) running = up(step);
    byStep.set(n, [...(byStep.get(n) ?? []), e]);
  });
  const top = Math.max(step, ...byStep.keys());
  const out: Segment[] = [];
  for (let n = 1; n <= top; n++) if (!skipped.includes(n)) out.push({ n, entries: byStep.get(n) ?? [] });
  return out;
}

// A person's reading of the issue page (call 9), kept in the browser.
export interface Reading {
  tab: 'conversation' | 'history';
  show: 'step' | 'all' | 'open';
  mine: boolean;
  agent: boolean;
  milestones: boolean;
  newest: boolean;
  threads: boolean;
  description: boolean;
}

// A first visit: All, Threads, Oldest first, the description folded.
export const READING: Reading = {
  tab: 'conversation', show: 'all', mine: false, agent: false, milestones: false, newest: false, threads: true, description: false,
};

// pick applies the filters: one of This step / All / Open threads; Mine,
// Agent and Milestones narrow it to their union. Segments stay, emptied,
// so the rail still has its anchors; Newest first reverses them and their
// entries.
export function pick(segs: Segment[], events: IssueEvent[], r: Reading, meId: string, step: number): Segment[] {
  const open = new Set(threadsOf(events).filter((t) => !t.resolved).flatMap((t) => [t.root, ...t.replies].map((e) => e.id)));
  const narrow = r.mine || r.agent || r.milestones;
  const keep = (e: IssueEvent) =>
    (r.show !== 'open' || open.has(e.id)) &&
    (!narrow || (r.mine && e.author.id === meId) || (r.agent && !!e.meta?.['ai:model']) || (r.milestones && e.kind !== 'comment'));
  const out = segs.filter((s) => r.show !== 'step' || s.n === step).map((s) => ({ n: s.n, entries: s.entries.filter(keep) }));
  return r.newest ? out.reverse().map((s) => ({ n: s.n, entries: s.entries.reverse() })) : out;
}

export interface Item { event: IssueEvent; replies: IssueEvent[]; resolved?: boolean }

// fold is the Threads grouping: a line comment's replies fold under it
// when it is among the entries; a reply without it stands alone.
export function fold(entries: IssueEvent[]): Item[] {
  const roots = new Map<string, Item>();
  const items: Item[] = [];
  for (const e of entries) {
    const root = e.review?.reply_to ? roots.get(e.review.reply_to) : undefined;
    if (root) {
      root.replies.push(e);
      continue;
    }
    const item: Item = { event: e, replies: [] };
    if (e.review?.path && !e.review.reply_to) roots.set(e.id, Object.assign(item, { resolved: !!e.review.resolved }));
    items.push(item);
  }
  return items;
}

const days = new Intl.DateTimeFormat(undefined, { day: 'numeric', month: 'short' });

// divider heads a segment: "01 Intent · Oct 21 – 22 · 12 entries",
// "07 Verify · Oct 25 · current".
export function divider(s: Segment, current: boolean): string {
  const head = `${String(s.n).padStart(2, '0')} ${STEPS[s.n]}`;
  const t = s.entries.map(time);
  const span = t.length ? ` · ${days.formatRange(Math.min(...t), Math.max(...t))}` : '';
  const n = s.entries.length;
  const tail = current ? ' · current' : n ? ` · ${n} ${n === 1 ? 'entry' : 'entries'}` : '';
  return head + span + tail;
}

// firstNew is the entry the "new since your last visit" line goes above:
// the first one written after `since` by someone else.
export function firstNew(segs: Segment[], since: number | null, meId: string): string | undefined {
  if (since === null) return undefined;
  return segs.flatMap((s) => s.entries).find((e) => e.author.id !== meId && time(e) > since)?.id;
}
