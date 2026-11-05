// The command palette's own matching, besides the filter (the server's):
// an issue the query names by its number (#2528 a forge's; 2528 and
// CO2-2528 its project number once the project numbers its issues, else
// a forge's) or the start of its git-bug id, 7 characters at least, as the
// filter takes them, at once, from the issues loaded when it opened; a
// command by its label (every word, else its letters in order). Best
// first; nothing typed: as given.
export type Item = {
  key: string;
  label: string;
  id?: string | undefined;
  numbers?: string[] | undefined; // its forge numbers
  project?: string | undefined; // its project number, CO2-2528
  counted?: boolean | undefined; // the project numbers its issues
};

export function jumps<T extends Item>(query: string, items: T[]): T[] {
  const q = query.trim().toLowerCase();
  const forge = /^#(\d+)$/.exec(q)?.[1];
  const bare = /^\d+$/.test(q) ? q : undefined;
  const id = /^[0-9a-f]{7,}$/.test(q) ? q : undefined;
  const own = (i: T) => (i.counted ? i.project?.slice(i.project.lastIndexOf('-') + 1) === bare : !!i.numbers?.includes(bare!));
  return items.filter(
    (i) =>
      (forge !== undefined && !!i.numbers?.includes(forge)) ||
      (bare !== undefined && own(i)) ||
      i.project?.toLowerCase() === q ||
      (id !== undefined && !!i.id?.startsWith(id)),
  );
}

// numberName is an issue's number as a card shows it: its project number,
// else its first forge number, #36
export function numberName(i: { number?: string | undefined; forges?: { number?: string | undefined }[] | undefined }): string | undefined {
  const forge = i.forges?.find((f) => f.number)?.number;
  return i.number ?? (forge ? `#${forge}` : undefined);
}

// recentName is an issue opened lately's number: an older entry's bare
// forge number takes its #
export const recentName = (r: { number?: string | undefined }) => (r.number && /^\d+$/.test(r.number) ? `#${r.number}` : r.number);

// example is a number the palette's box offers to type: the newest
// issue's, its project number else a forge's; else a neutral one
export function example(items: { project?: string | undefined; numbers?: string[] | undefined }[]): string {
  for (const i of items) {
    if (i.project) return i.project;
    if (i.numbers?.[0]) return `#${i.numbers[0]}`;
  }
  return '#12';
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

/** The filter's answer as the palette lists it: the latest edited first,
 * open or closed; the latest opened breaks a tie (a sync stamps many at once). */
export function latest<T extends { updated_at?: string; created_at?: string }>(issues: T[]): T[] {
  return [...issues].sort((a, b) => (b.updated_at ?? '').localeCompare(a.updated_at ?? '') || (b.created_at ?? '').localeCompare(a.created_at ?? ''));
}

function inOrder(needle: string, hay: string): boolean {
  let i = 0;
  for (const c of hay) if (c === needle[i] && ++i === needle.length) return true;
  return false;
}

const escape = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
