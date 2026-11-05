// A long list's rows, windowed: past LONG rows loaded, a table renders
// those on screen and a screen's worth around them, spacers holding the
// height of the rest (the board page's 20,000 issues load a page at a
// time, and a reader who scrolls far would otherwise grow the DOM a page
// at a time too). Rows are about one height: the average measured.
import { onBeforeUnmount, onMounted, ref } from 'vue';

export const LONG = 300;

// windowRange is the rows to render: all of a short list; else those
// within a screen above and below the viewport. top: the list's top,
// relative to the viewport; pure, node runs windowrows.test.ts on it.
export function windowRange(count: number, top: number, viewport: number, rowHeight: number): [number, number] {
  if (count <= LONG) return [0, count];
  const first = Math.floor(-top / rowHeight);
  const shown = Math.ceil(viewport / rowHeight);
  const start = Math.max(0, Math.min(count, first - shown));
  const end = Math.max(start, Math.min(count, first + 2 * shown));
  return [start, end];
}

// useScroll is a tick that moves with any scroll (the window's, a table's
// own box's: a capturing listener hears both) and the window's size, once
// a frame at most: what windowRange reads the positions on
export function useScroll() {
  const tick = ref(0);
  let frame = 0;
  const on = () => {
    if (frame) return;
    frame = requestAnimationFrame(() => {
      frame = 0;
      tick.value++;
    });
  };
  onMounted(() => {
    document.addEventListener('scroll', on, { capture: true, passive: true });
    window.addEventListener('resize', on, { passive: true });
  });
  onBeforeUnmount(() => {
    document.removeEventListener('scroll', on, { capture: true });
    window.removeEventListener('resize', on);
    cancelAnimationFrame(frame);
  });
  return tick;
}
