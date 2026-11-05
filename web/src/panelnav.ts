// j/k in the board's panel: the issues before and after the open one. The
// open issue gone from the list (closed, or moved out of the view's filter,
// from the panel itself), its neighbours are those around where it was:
// next is the issue that took its place, prev the one before.

export interface Neighbours {
  at: number; // where the open issue is, -1 when gone
  prev?: string | undefined;
  next?: string | undefined;
}

// last: where the open issue was when last in the list, -1 for never
export function neighbours(ids: string[], open: string, last: number): Neighbours {
  const at = ids.indexOf(open);
  if (at >= 0) return { at, prev: ids[at - 1], next: ids[at + 1] };
  if (last < 0) return { at: -1 };
  const from = Math.min(last, ids.length);
  return { at: -1, prev: ids[from - 1], next: ids[from] };
}
