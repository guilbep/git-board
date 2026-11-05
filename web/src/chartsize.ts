// Charts are drawn at their real width, one SVG unit a pixel, so their
// text keeps its size on any screen; and with round ticks.
import { onBeforeUnmount, onMounted, ref, type Ref } from 'vue';

export function useWidth(el: Ref<HTMLElement | null>, fallback: number): Ref<number> {
  const width = ref(fallback);
  let ro: ResizeObserver | undefined;
  onMounted(() => {
    if (!el.value) return;
    width.value = el.value.clientWidth || fallback;
    ro = new ResizeObserver(([e]) => (width.value = Math.max(240, Math.round(e!.contentRect.width))));
    ro.observe(el.value);
  });
  onBeforeUnmount(() => ro?.disconnect());
  return width;
}

// niceTicks: 0 up to past max, in steps of 1, 2 or 5 times a power of ten,
// about count of them.
export function niceTicks(max: number, count = 4): number[] {
  const raw = Math.max(1, max) / count;
  const p = 10 ** Math.floor(Math.log10(raw));
  const step = Math.max(1, [1, 2, 5, 10].map((m) => m * p).find((m) => m >= raw)!); // counts: whole steps
  const top = Math.ceil(Math.max(1, max) / step) * step;
  const res: number[] = [];
  for (let v = 0; v <= top + step / 2; v += step) res.push(Math.round(v * 1e6) / 1e6);
  return res;
}
