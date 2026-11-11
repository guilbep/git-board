// v-near="fn": fn runs when the element comes within a screen of its
// scrolling box (or the viewport), and again after each update while it
// stays there: the "more" at the end of a paged list, asked before the
// reader gets there, until the list fills past it or ends.
import type { Directive } from 'vue';

type State = { io: IntersectionObserver; fn: () => void; near: boolean };
const states = new WeakMap<Element, State>();

// the element's scrolling ancestor, if any: the screen ahead is its. One
// that scrolls up and down: a table's box that scrolls sideways only
// (overflow-x: auto makes overflow-y auto too) grows with its rows, and
// taken for the scroller it held every row near, so that a phone's
// table loaded every page of the list at once.
export function scrollerOf(el: Element): Element | null {
  for (let p = el.parentElement; p; p = p.parentElement) if (/(auto|scroll)/.test(getComputedStyle(p).overflowY) && p.scrollHeight > p.clientHeight) return p;
  return null;
}

export const vNear: Directive<Element, () => void> = {
  mounted(el, binding) {
    const st: State = {
      fn: binding.value,
      near: false,
      io: new IntersectionObserver(
        (es) => {
          st.near = es.some((e) => e.isIntersecting);
          if (st.near) st.fn();
        },
        { root: scrollerOf(el), rootMargin: '100% 0px' },
      ),
    };
    st.io.observe(el);
    states.set(el, st);
  },
  updated(el, binding) {
    const st = states.get(el);
    if (!st) return;
    st.fn = binding.value; // the latest list's
    if (st.near) queueMicrotask(() => st.near && st.fn());
  },
  unmounted(el) {
    states.get(el)?.io.disconnect();
    states.delete(el);
  },
};
