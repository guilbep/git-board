<!-- One tip at a time (docs/prd/header-help.md, 4; the canvas's D1, D2,
     D3): a shortcut that works where you are and that you don't use yet,
     every 12 s. Hover or focus pauses it; a click opens the list; × hides
     that tip for good. With reduced motion: one tip, no change, no fade.
     A note, never a live region: a screen reader isn't told each change. -->
<template>
  <p v-if="tip" class="gb-tip" role="note" :class="{ 'is-fading': fading }" @mouseenter="pause" @mouseleave="resume" @focusin="pause" @focusout="resume">
    <button type="button" class="gb-tip-go" :title="`Every shortcut: ${tip.group}`" @click="helpOpen = true">
      <span class="gb-tip-word">Tip</span>
      <span class="gb-tip-keys"><kbd v-for="k in tip.keys" :key="k">{{ keyLabel(k) }}</kbd></span>
      <span class="gb-tip-text">{{ tip.tip }}</span>
    </button>
    <span v-if="pool.length > 1" class="gb-tip-dots" aria-hidden="true"><span v-for="s in pool" :key="s.id" :class="{ on: s.id === tip.id }" /></span>
    <button type="button" class="gb-tip-x" :aria-label="`Hide this tip (${tip.what})`" title="Hide this tip" @click="dismissTip(tip.id)"><GbIcon name="close" small /></button>
  </p>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue';
import { inContext, keyContext, keyLabel, usesChanged } from '@/keys';
import { EVERY_MS, dismissTip, pickTip, retired, tipsChanged, tipsOff } from '@/tips';
import { helpOpen } from '@/shortcuts';
import GbIcon from '@/components/GbIcon.vue';

const still = window.matchMedia('(prefers-reduced-motion: reduce)').matches;

// the tips that fit here, minus the retired; recomputed on a use or a dismissal
const pool = computed(() => {
  void usesChanged.value;
  void tipsChanged.value;
  if (tipsOff.value) return [];
  const gone = retired();
  return inContext(keyContext.value).filter((s) => s.tip && !gone.has(s.id));
});
const current = ref<string | undefined>();
const tip = computed(() => pool.value.find((s) => s.id === current.value) ?? null);

function next() {
  current.value = pickTip(pool.value, new Set(), current.value)?.id;
}
// a new place, or the current tip gone: the next one at once
watch(pool, (p) => {
  if (!p.some((s) => s.id === current.value)) next();
}, { immediate: true });

const fading = ref(false);
let timer: ReturnType<typeof setInterval> | undefined;
function resume() {
  if (still || timer) return;
  timer = setInterval(() => {
    fading.value = true;
    setTimeout(() => {
      next();
      fading.value = false;
    }, 200);
  }, EVERY_MS);
}
function pause() {
  clearInterval(timer);
  timer = undefined;
}
resume();
onBeforeUnmount(pause);
</script>

<style scoped>
.gb-tip { display: flex; align-items: center; gap: .5rem; min-width: 0; margin: 0; color: var(--fg-muted); font-size: .8125rem; transition: opacity .2s; }
.gb-tip.is-fading { opacity: 0; }
.gb-tip-go { display: inline-flex; align-items: center; gap: .5rem; min-width: 0; padding: 0; border: 0; background: none; color: inherit; font: inherit; cursor: pointer; text-align: left; }
.gb-tip-go:hover .gb-tip-text { color: var(--fg); text-decoration: underline; }
.gb-tip-go:focus-visible, .gb-tip-x:focus-visible { outline: var(--focus-w) solid var(--primary-hover); outline-offset: .125rem; }
.gb-tip-word { font-weight: var(--w-bold); }
.gb-tip-keys { display: inline-flex; gap: .1875rem; flex: none; }
.gb-tip-text { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
kbd { min-width: .75rem; padding: 0 .3125rem; border: var(--border-w) solid var(--border-strong); border-bottom-width: .125rem; border-radius: var(--radius); background: var(--bg); color: var(--fg); font-family: var(--font-sans); font-size: .75rem; line-height: 1.35; text-align: center; }
.gb-tip-dots { display: inline-flex; gap: .1875rem; flex: none; }
.gb-tip-dots > span { width: .3125rem; height: .3125rem; border-radius: 50%; background: var(--border-strong); }
.gb-tip-dots > span.on { background: var(--fg); }
.gb-tip-x { display: inline-flex; flex: none; padding: .125rem; border: 0; background: none; color: var(--fg-muted); cursor: pointer; }
.gb-tip-x:hover { color: var(--fg); }
@media (prefers-reduced-motion: reduce) { .gb-tip { transition: none; } }
</style>
