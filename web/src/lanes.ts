// A board per role: the lanes it shows (a view's `lanes`: values of the
// group field, in order), their limits, and picking several cards to move
// at once. Pure, no "@/" imports: node runs lanes.test.ts on it.
import type { Bucket } from './api/board';
import { rangeIds } from './tablecols.ts';

// boardLanes keeps the buckets of the lanes, in their order, an empty one
// for a lane no issue is in; the other buckets' issues are counted, not
// shown. No lanes: every bucket, as given.
export function boardLanes(buckets: Bucket[], lanes: string[] | undefined): { shown: Bucket[]; hidden: number } {
  if (!lanes?.length) return { shown: buckets, hidden: 0 };
  const by = new Map(buckets.map((b) => [b.group, b]));
  const shown = lanes.map((l) => by.get(l) ?? { group: l, issues: [] });
  const hidden = buckets.filter((b) => !lanes.includes(b.group)).reduce((n, b) => n + b.issues.length, 0);
  return { shown, hidden };
}

// the lane chooser's list: the lanes, in their order, then the field's
// other values
export function laneChoices(lanes: string[] | undefined, values: string[]): string[] {
  if (!lanes) return values;
  return [...lanes, ...values.filter((v) => !lanes.includes(v))];
}

// toggleLane ticks or unticks a value. No lanes means every value: the
// first untick starts from all of them; ticking back to every value, in
// the field's order, is no lanes of its own again.
export function toggleLane(lanes: string[] | undefined, value: string, all: string[]): string[] | undefined {
  const cur = lanes ?? all;
  const next = cur.includes(value) ? cur.filter((v) => v !== value) : [...cur, value];
  const sorted = all.filter((v) => next.includes(v));
  return next.length === all.length && next.every((v, i) => v === sorted[i]) ? undefined : next;
}

// a lane's cards against its limit
export function limitState(count: number, limit: number | undefined): '' | 'at' | 'over' {
  if (!limit) return '';
  return count > limit ? 'over' : count === limit ? 'at' : '';
}

// pickCards: Cmd/Ctrl-click toggles a card, Shift-click adds the cards
// from the last one picked to this one in its lane's order.
export function pickCards(picked: string[], id: string, order: string[], how: { toggle?: boolean; range?: boolean; anchor?: string }): string[] {
  if (how.range && how.anchor) {
    const add = rangeIds(order, how.anchor, id);
    return [...picked, ...add.filter((x) => !picked.includes(x))];
  }
  return picked.includes(id) ? picked.filter((x) => x !== id) : [...picked, id];
}
