<!-- A control and its popover (docs/design/issue-head/README.md,
     Components): the button names what it opens; the panel floats under
     it, bordered, the one place with a shadow. Esc closes it and gives the
     focus back to the button, before the drawer's own Esc; a click outside
     closes it; nothing traps the focus. It keeps inside the drawer: it
     opens on the side that fits, else shifts to fit. -->
<template>
  <span ref="root" class="gb-pop" :class="{ 'is-open': open }">
    <button
      ref="btn"
      type="button"
      :class="buttonClass ?? 'gb-icon-btn'"
      :aria-label="label"
      :title="tip ?? label"
      :aria-expanded="open"
      aria-haspopup="dialog"
      :disabled="disabled"
      @click="toggle"
      @keydown.esc="onEsc"
    >
      <slot name="button" />
    </button>
    <div v-if="open" ref="panel" class="gb-pop-panel" :class="`is-${side}`" :style="offset" tabindex="-1" role="dialog" :aria-label="title ?? label" @keydown.esc="onEsc">
      <p v-if="title" class="gb-pop-title">{{ title }}</p>
      <slot :close="close" />
    </div>
  </span>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref } from 'vue';

const props = defineProps<{
  label: string; // the button's name, and its tooltip unless tip
  tip?: string | undefined;
  title?: string | undefined; // the panel's heading
  align?: 'left' | 'right' | undefined; // which edge of the button the panel keeps to
  buttonClass?: string | undefined;
  disabled?: boolean | undefined;
  focus?: string | undefined; // what takes the focus on opening, a selector; else the panel itself: no input jumps in
}>();
const emit = defineEmits<{ open: []; close: [] }>();

const open = ref(false);
const root = ref<HTMLElement | null>(null);
const btn = ref<HTMLButtonElement | null>(null);
const panel = ref<HTMLElement | null>(null);

// the side it opens on, and a shift when neither side fits the drawer
const side = ref<'left' | 'right'>('left');
const shift = ref(0);
const offset = computed(() => (shift.value ? { [side.value]: `${shift.value}px` } : undefined));
function fit() {
  const el = panel.value;
  const anchor = root.value;
  if (!el || !anchor) return;
  const box = (anchor.closest('.gb-panel') ?? document.body).getBoundingClientRect();
  const a = anchor.getBoundingClientRect();
  const w = el.offsetWidth;
  const m = 8; // clear of the drawer's edges, in CSS pixels
  const fitsLeft = a.left + w <= box.right - m; // its left edge at the button's
  const fitsRight = a.right - w >= box.left + m; // its right edge at the button's
  const want = props.align ?? 'left';
  side.value = want === 'left' ? (fitsLeft || !fitsRight ? 'left' : 'right') : fitsRight || !fitsLeft ? 'right' : 'left';
  shift.value = 0;
  if (!fitsLeft && !fitsRight) shift.value = side.value === 'left' ? Math.round(box.right - m - (a.left + w)) : Math.round(a.right - w - (box.left + m));
}

// what it holds can grow once open (a branch made, an editor opened in
// place): it fits again then
let watcher: ResizeObserver | undefined;

function outside(e: PointerEvent) {
  if (root.value && e.target instanceof Node && !root.value.contains(e.target)) close(false);
}
async function show() {
  if (open.value) return;
  side.value = props.align ?? 'left';
  shift.value = 0;
  open.value = true;
  emit('open');
  document.addEventListener('pointerdown', outside, true);
  await nextTick();
  fit();
  if (panel.value && typeof ResizeObserver !== 'undefined') {
    watcher = new ResizeObserver(() => fit());
    watcher.observe(panel.value);
  }
  // the focus goes into the panel, never into an input by itself (a
  // browser's autofill would jump in); Tab goes on from there
  const el = (props.focus && panel.value?.querySelector<HTMLElement>(props.focus)) || panel.value;
  el?.focus({ preventScroll: true });
}
function close(refocus = true) {
  if (!open.value) return;
  open.value = false;
  emit('close');
  watcher?.disconnect();
  watcher = undefined;
  document.removeEventListener('pointerdown', outside, true);
  if (refocus) btn.value?.focus({ preventScroll: true });
}
const toggle = () => (open.value ? close() : void show());
function onEsc(e: KeyboardEvent) {
  if (!open.value) return;
  e.stopPropagation();
  e.preventDefault();
  close();
}
onBeforeUnmount(() => {
  watcher?.disconnect();
  document.removeEventListener('pointerdown', outside, true);
});
defineExpose({ show, close });
</script>
