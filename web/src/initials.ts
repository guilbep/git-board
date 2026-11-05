// initials for an avatar: "Pierre Guilbert" → "PG". A login in parentheses
// ("Guilbert Pierre (guilbep)", as forge imports name people) and
// punctuation don't count; a single word gives its first two letters.
export function initials(name: string): string {
  const outside = name.replace(/\([^)]*\)/g, ' ');
  const base = /\p{L}/u.test(outside) ? outside : name;
  const words = base.split(/[^\p{L}\p{N}]+/u).filter(Boolean);
  if (words.length === 0) return '?';
  if (words.length === 1) return words[0]!.slice(0, 2).toUpperCase();
  return (words[0]![0]! + words[words.length - 1]![0]!).toUpperCase();
}
