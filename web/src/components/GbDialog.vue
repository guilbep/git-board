<!-- A dialog on the page's top layer: the browser's own <dialog>, opened
     modal, so it traps the focus, closes on Esc, and makes the page under
     it inert; a click on the backdrop closes it, and so does a new address
     unless noRouteDismiss. The <dialog> is the frame, the whole window: its
     content keeps its own box, as the dialogs and the issue panel style
     it (gitboard.css), a minimized one at most 560px wide and 48px short
     of the window's height, as before (Quasar's QDialog). -->
<template>
  <dialog
    ref="el"
    class="gb-modal"
    :class="[`gb-modal-${position ?? 'standard'}`, { 'gb-modal-full': fullHeight, 'gb-modal-max': maximized, 'gb-modal-blur': blur }]"
    @cancel="onCancel"
    @close="onClose"
    @click="onClick"
  >
    <slot v-if="shown || warmed" />
  </dialog>
</template>

<script lang="ts">
// the dialogs open, the last on top: Esc closes the top one wherever the
// focus is. The browser's own Esc closes a modal dialog only while the
// focus is in it, and a button that replaces itself (Hide tips → Show
// tips again) drops the focus to the page's body.
const opened: { close: () => void; persistent: () => boolean }[] = [];
if (typeof document !== 'undefined')
  document.addEventListener('keydown', (e) => {
    const top = opened.at(-1);
    if (e.key !== 'Escape' || e.defaultPrevented || !top) return;
    e.preventDefault(); // the browser's close request, said once
    if (!top.persistent()) top.close();
  });
</script>

<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useRoute } from 'vue-router';

const props = defineProps<{
  modelValue: boolean;
  position?: 'standard' | 'top' | 'right' | undefined;
  fullHeight?: boolean | undefined; // the window's whole height, no inset
  maximized?: boolean | undefined; // the whole window
  persistent?: boolean | undefined; // Esc and the backdrop leave it open
  blur?: boolean | undefined; // the page behind it blurred
  noFocus?: boolean | undefined; // the content focuses itself: nothing is focused for it
  noRouteDismiss?: boolean | undefined; // a new address leaves it open
  // warm: its content is made while it is closed, once the page is idle,
  // and kept: opening it is showing it (the palette, the shortcuts: a
  // person's keystroke answered in a frame, even on a slow phone)
  warm?: boolean | undefined;
}>();
const emit = defineEmits<{ 'update:modelValue': [open: boolean]; show: []; hide: [] }>();

const el = ref<HTMLDialogElement | null>(null);
const shown = ref(false);
const warmed = ref(false);
if (props.warm) {
  const idle = (fn: () => void) => (typeof requestIdleCallback === 'function' ? requestIdleCallback(fn, { timeout: 2000 }) : setTimeout(fn, 500));
  onMounted(() => idle(() => (warmed.value = true)));
}
let before: Element | null = null; // what had the focus: it gets it back

const me = { close: () => close(), persistent: () => !!props.persistent };

const focusable = 'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

async function open() {
  const d = el.value;
  if (!d || d.open) return;
  before = document.activeElement;
  d.showModal();
  opened.push(me);
  shown.value = true;
  await nextTick();
  if (!props.noFocus) {
    const first = d.querySelector<HTMLElement>('[autofocus]') ?? d.querySelector<HTMLElement>(focusable);
    first?.focus();
  }
  emit('show');
}

// close closes it, and says so at once: the browser's close event comes a
// task later, and a page opened meanwhile must not find it still open
function close() {
  const d = el.value;
  if (d?.open) d.close();
  onClose();
}

watch(() => props.modelValue, (v) => void (v ? open() : close()), { flush: 'post' });
// opened at once: the dialog is in the page from its first render
watch(el, (d) => d && props.modelValue && void open(), { flush: 'post' });

// Esc is the document's listener's (above): it lets a field that keeps
// its own Esc (a rename's) stop it, as the browser's close would not
function onCancel(e: Event) {
  e.preventDefault();
}

function onClose() {
  const i = opened.indexOf(me);
  if (i < 0) return; // told already
  opened.splice(i, 1);
  shown.value = false;
  if (before instanceof HTMLElement && document.contains(before)) before.focus();
  before = null;
  if (props.modelValue) emit('update:modelValue', false);
  emit('hide');
}

// the backdrop is the dialog itself: its content is its children
function onClick(e: MouseEvent) {
  if (e.target === el.value && !props.persistent) close();
}

const route = useRoute();
watch(() => route.fullPath, () => props.noRouteDismiss || close());

onBeforeUnmount(close);
</script>
