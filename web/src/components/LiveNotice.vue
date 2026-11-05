<!-- Live updates for what every page shares: who you write as and the
     features on; and a small notice when something changed elsewhere,
     the page having reloaded it. -->
<template>
  <transition name="gb-live">
    <p v-if="shown" class="gb-live" role="status"><GbIcon name="sync" small /> Updated just now</p>
  </transition>
</template>

<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue';
import { lastLive, onLive } from '@/live';
import { loadMe } from '@/me';
import GbIcon from '@/components/GbIcon.vue';

// who writes, or the experimental features (the file's), changed
const off = onLive((e) => {
  if (e.kind === 'me' || e.kind === 'config') void loadMe();
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
.gb-live-enter-active, .gb-live-leave-active { transition: opacity .2s; }
.gb-live-enter-from, .gb-live-leave-to { opacity: 0; }
</style>
