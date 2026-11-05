// Since your last visit, per view, kept in this browser: each visit
// remembers the one before it, so the issues changed since then get a dot.
// A reload within GAP is the same visit (the dots stay); after a break,
// what you saw then becomes the comparison point.
// ponytail: localStorage is per origin, so two repositories served on the
// same address share it; key by repository when the hosted service lands.
export const GAP = 30 * 60 * 1000;

export type Visit = { prev: number | null; current: number };

export function visit(rec: Visit | null, now: number): Visit {
  if (!rec) return { prev: null, current: now };
  if (now - rec.current > GAP) return { prev: rec.current, current: now };
  return { prev: rec.prev, current: now };
}

export function changedSince(issues: { id: string; updated_at?: string }[], since: number | null): Set<string> {
  const res = new Set<string>();
  if (since === null) return res;
  for (const i of issues) if (i.updated_at && Date.parse(i.updated_at) > since) res.add(i.id);
  return res;
}

const keyOf = (view: string) => `gb-visit:${view || '*'}`;

export function loadVisit(view: string): Visit | null {
  try {
    return JSON.parse(localStorage.getItem(keyOf(view)) ?? 'null') as Visit | null;
  } catch {
    return null;
  }
}

export function saveVisit(view: string, rec: Visit) {
  try {
    localStorage.setItem(keyOf(view), JSON.stringify(rec));
  } catch {
    // private window: the dots last until reload
  }
}
