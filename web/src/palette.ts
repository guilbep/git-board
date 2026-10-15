// The command palette's matching: an issue by its forge number (#2528 or
// 2528), its git-bug id prefix, or its title (every word, else its letters
// in order); an action by its label. Best first; nothing typed: as given.
export type Item = { key: string; label: string; id?: string | undefined; number?: string | undefined };

export function rank<T extends Item>(query: string, items: T[]): T[] {
  const q = query.trim().toLowerCase();
  if (!q) return items;
  const num = q.replace(/^#/, '');
  const words = q.split(/\s+/);
  const scored: { item: T; score: number }[] = [];
  for (const item of items) {
    let score = 0;
    if (item.number && /^\d+$/.test(num)) {
      if (item.number === num) score = 100;
      else if (item.number.startsWith(num)) score = 60;
    }
    if (item.id && /^[0-9a-f]{4,}$/.test(q) && item.id.startsWith(q)) score = Math.max(score, 90);
    const label = item.label.toLowerCase();
    if (words.every((w) => label.includes(w))) {
      const first = words[0]!;
      score = Math.max(score, 50 + (label.startsWith(first) ? 10 : 0) + (new RegExp(`(^|[^\\p{L}])${escape(first)}`, 'u').test(label) ? 5 : 0));
    } else if (inOrder(q.replace(/\s+/g, ''), label)) {
      score = Math.max(score, 20);
    }
    if (score) scored.push({ item, score });
  }
  return scored.sort((a, b) => b.score - a.score).map((s) => s.item);
}

function inOrder(needle: string, hay: string): boolean {
  let i = 0;
  for (const c of hay) if (c === needle[i] && ++i === needle.length) return true;
  return false;
}

const escape = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
