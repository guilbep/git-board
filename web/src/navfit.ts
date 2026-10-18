// fit: which view links fit in room, the rest under "N more". The active
// view always shows: it takes the last shown slots when it would hide,
// alone when nothing else fits beside it. Widths and room in px; more is
// the "N more" menu's width, taken from the room once anything hides.
export function fit(widths: number[], room: number, active: number, more: number): { shown: number[]; hidden: number[] } {
  const all = widths.map((_, i) => i);
  if (widths.reduce((a, w) => a + w, 0) <= room) return { shown: all, hidden: [] };
  const budget = room - more;
  const shown: number[] = [];
  let used = 0;
  for (const i of all) {
    if (used + widths[i]! > budget) break;
    shown.push(i);
    used += widths[i]!;
  }
  if (active >= 0 && !shown.includes(active)) {
    while (shown.length && used + widths[active]! > budget) used -= widths[shown.pop()!]!;
    shown.push(active); // after every shown one: it was hidden for coming later
  }
  return { shown, hidden: all.filter((i) => !shown.includes(i)) };
}
