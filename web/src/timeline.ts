// An issue's history on one time axis, from its opening to now (the top of
// the History mode): the status it was in, when it was closed, and a marker
// per change. Positions are percents of the axis. Pure: built from the
// replayed steps (replay.ts).
import type { Segment as StepSegment } from './flow.ts';
import type { Step } from './replay.ts';

export interface Segment {
  value: string; // the status field's value, '' while not set
  left: number;
  width: number;
}

export interface Marker {
  lane: 'texts' | 'versions' | 'changes'; // comments and their edits; the description's versions; what was changed
  kind: Step['kind'];
  tick?: boolean; // a description edit that published no version
  left: number;
  index: number; // the step, in the order steps() gives
}

export interface Timeline {
  start: number; // ms
  end: number;
  segments: Segment[];
  closed: { left: number; width: number }[];
  markers: Marker[];
  bars: Bar[];
}

// a step, from its opening to the next one's, the last to now
export interface Bar { n: number; left: number; width: number }

export function timeline(all: Step[], now: number, field = 'status', segs: StepSegment[] = []): Timeline {
  const start = all.length ? Date.parse(all[0]!.at) : now;
  const end = Math.max(now, ...all.map((s) => Date.parse(s.at)));
  const span = end - start || 1;
  const pct = (t: number) => ((t - start) / span) * 100;

  const segments: Segment[] = [];
  const closed: Timeline['closed'] = [];
  let value = '';
  let since = start;
  let closedAt: number | null = null;
  const cut = (at: number) => {
    if (at > since || !segments.length) segments.push({ value, left: pct(since), width: pct(at) - pct(since) });
    since = at;
  };

  all.forEach((s) => {
    const at = Date.parse(s.at);
    if (s.kind === 'labels') {
      const set = (s.added ?? []).find((l) => l.startsWith(`${field}:`));
      const unset = (s.removed ?? []).some((l) => l.startsWith(`${field}:`));
      const next = set ? set.slice(field.length + 1) : unset ? '' : value;
      if (next !== value) {
        cut(at);
        value = next;
      }
    } else if (s.kind === 'status') {
      if (s.status === 'closed' && closedAt === null) closedAt = at;
      if (s.status === 'open' && closedAt !== null) {
        closed.push({ left: pct(closedAt), width: pct(at) - pct(closedAt) });
        closedAt = null;
      }
    }
  });
  cut(end);
  if (closedAt !== null) closed.push({ left: pct(closedAt), width: pct(end) - pct(closedAt) });
  // a status set at the very start leaves an empty first piece: drop it
  const band = segments.filter((g, i) => g.width > 0 || segments.length === 1 || i === segments.length - 1);

  const markers = all.map((s, index): Marker => ({
    lane: s.version !== undefined ? 'versions' : s.kind === 'comment' || s.kind === 'edit' ? 'texts' : 'changes',
    kind: s.kind,
    left: pct(Date.parse(s.at)),
    index,
    ...(s.version === 0 ? { tick: true } : {}),
  }));
  const open = segs.filter((g) => g.n > 0 && (g.n === 1 || g.at)).map((g) => ({ n: g.n, left: g.n === 1 ? 0 : pct(Date.parse(g.at!)) }));
  const bars = open.map((b, i) => ({ ...b, width: (open[i + 1]?.left ?? 100) - b.left }));
  // one instant (a new issue, nothing since): the band is all of it
  const whole = end === start || !band.length;
  return { start, end, segments: whole ? [{ value, left: 0, width: 100 }] : band, closed, markers, bars };
}
