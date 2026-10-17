// The command palette's own matching, besides the filter (the server's):
// an issue the query names by its number on a forge (#2528 or 2528) or the
// start of its git-bug id, 7 characters at least, as the filter takes
// them, at once, from the issues loaded when it opened; a command by its
// label (every word, else its letters in order). Best first; nothing
// typed: as given.
export type Item = { key: string; label: string; id?: string | undefined; numbers?: string[] | undefined };

export function jumps<T extends Item>(query: string, items: T[]): T[] {
  const q = query.trim().toLowerCase();
  const num = /^#?(\d+)$/.exec(q)?.[1];
  const id = /^[0-9a-f]{7,}$/.test(q) ? q : undefined;
  return items.filter((i) => (num !== undefined && !!i.numbers?.includes(num)) || (id !== undefined && !!i.id?.startsWith(id)));
}

export function rank<T extends { label: string }>(query: string, items: T[]): T[] {
  const q = query.trim().toLowerCase();
  if (!q) return items;
  const words = q.split(/\s+/);
  const scored: { item: T; score: number }[] = [];
  for (const item of items) {
    const label = item.label.toLowerCase();
    let score = 0;
    if (words.every((w) => label.includes(w))) {
      const first = words[0]!;
      score = 50 + (label.startsWith(first) ? 10 : 0) + (new RegExp(`(^|[^\\p{L}])${escape(first)}`, 'u').test(label) ? 5 : 0);
    } else if (inOrder(q.replace(/\s+/g, ''), label)) {
      score = 20;
    }
    if (score) scored.push({ item, score });
  }
  return scored.sort((a, b) => b.score - a.score).map((s) => s.item);
}

/** The filter's answer as the palette lists it: open first, the latest changed first. */
export function openFirst<T extends { status: string; updated_at?: string }>(issues: T[]): T[] {
  return [...issues].sort((a, b) => Number(a.status !== 'open') - Number(b.status !== 'open') || (b.updated_at ?? '').localeCompare(a.updated_at ?? ''));
}

function inOrder(needle: string, hay: string): boolean {
  let i = 0;
  for (const c of hay) if (c === needle[i] && ++i === needle.length) return true;
  return false;
}

const escape = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
