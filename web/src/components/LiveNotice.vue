<!-- Live updates for what every page shares: who you write as and the
     features on; and a small notice when something changed elsewhere,
     the page having reloaded it. -->
<template>
  <transition name="gb-live">
    <p v-if="shown" class="gb-live" role="status"><GbIcon name="sync" small /> Updated just now</p>
  </transition>
  <p v-if="fileError" class="gb-live gb-live-error" role="alert" data-test="config-error">
    .git-board.toml was edited and does not open, so the board keeps the last good file: {{ fileError }}
  </p>
</template>

<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue';
import { lastLive, onLive } from '@/live';
import { loadMe } from '@/me';
import GbIcon from '@/components/GbIcon.vue';

// who writes, or the experimental features (the file's), changed
// a file edited by hand that does not open: said until one opens
const fileError = ref('');
const off = onLive((e) => {
  if (e.kind === 'me' || e.kind === 'config') void loadMe();
  if (e.kind === 'config') fileError.value = '';
  if (e.kind === 'config-error') fileError.value = e.error ?? '';
});
onBeforeUnmount(off);

const shown = ref(false);
let timer: ReturnType<typeof setTimeout> | undefined;
watch(lastLive, () => {
  shown.value = true;
  clearTimeout(timer);
  timer = setTimeout(() => (shown.value = false), 3000);
});
</script>

<style scoped>
.gb-live { position: fixed; left: 1rem; bottom: 1rem; z-index: 3000; display: inline-flex; align-items: center; gap: .375rem; margin: 0; padding: .375rem .75rem; border: 1px solid var(--border-strong); background: var(--bg); color: var(--fg-muted); font-size: .8125rem; box-shadow: var(--shadow); }
.gb-live-error { bottom: 3.5rem; max-width: min(40rem, calc(100vw - 2rem)); color: var(--danger-fg); background: var(--danger-bg); border-color: var(--danger-border); }
.gb-live-enter-active, .gb-live-leave-active { transition: opacity .2s; }
.gb-live-enter-from, .gb-live-leave-to { opacity: 0; }
</style>
