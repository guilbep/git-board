// The issue panel's size, kept in this browser: a side panel whose width
// is dragged from its left edge, or a wide overlay leaving 2.5rem around
// it over the blurred board. `f` or the expand button switch between them.
import { ref, watch } from 'vue';

export type PanelMode = 'side' | 'wide';

const MIN = 448; // 28rem
const GAP = 64; // 4rem of board left visible

// clampWidth keeps a dragged width between 28rem and the window less 4rem;
// a window narrower than that gets the panel whole.
export function clampWidth(px: number, viewport: number): number {
  if (viewport - GAP < MIN) return viewport;
  return Math.round(Math.min(Math.max(px, MIN), viewport - GAP));
}

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
    // private window: the size lasts until reload
  }
}

export const panelMode = ref<PanelMode>(read('gb-panel-mode') === 'wide' ? 'wide' : 'side');
export const panelWidth = ref<number>(Number(read('gb-panel-width')) || 0); // 0: the default
watch(panelMode, (m) => write('gb-panel-mode', m));
watch(panelWidth, (w) => write('gb-panel-width', String(w)));

export function togglePanelMode() {
  panelMode.value = panelMode.value === 'side' ? 'wide' : 'side';
}
