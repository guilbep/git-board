// What the command palette (CommandPalette.vue) shares with the pages: its
// own open state, a request for the board's New issue dialog, the filters
// panel's fold (kept in this browser), and the issues opened lately.
import { ref, watch } from 'vue';

export const paletteOpen = ref(false);
export const newIssueRequested = ref(false);

function read(key: string): string | null {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}
function write(key: string, value: string) {
  try {
    localStorage.setItem(key, value);
  } catch {
    // private window: lasts until reload
  }
}

export const filtersHidden = ref(read('gb-filters-hidden') === '1');
watch(filtersHidden, (v) => write('gb-filters-hidden', v ? '1' : '0'));

export type Recent = { id: string; title: string; number?: string | undefined };

export function recentIssues(): Recent[] {
  try {
    return JSON.parse(read('gb-recent') ?? '[]') as Recent[];
  } catch {
    return [];
  }
}

export function recordRecent(r: Recent) {
  if (!r.title) return;
  write('gb-recent', JSON.stringify([r, ...recentIssues().filter((x) => x.id !== r.id)].slice(0, 8)));
}
