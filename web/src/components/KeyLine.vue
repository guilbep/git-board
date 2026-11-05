<!-- The key line at the foot of the window (docs/prd/header-help.md, 3;
     the canvas's C5): the keys for where you are, ? last. Only where a
     page says where you are (the board), never over another bottom bar,
     not on a phone. -->
<template>
  <p v-if="shown" class="gb-keyline" aria-label="Keys here">
    <span v-for="s in line" :key="s.id" class="gb-keyline-k">
      <component :is="s.id === 'help' ? 'button' : 'span'" :type="s.id === 'help' ? 'button' : undefined" class="gb-keyline-keys" @click="s.id === 'help' && (helpOpen = true)">
        <kbd v-for="k in s.keys" :key="k">{{ keyLabel(k) }}</kbd>
      </component>
      {{ s.short }}
    </span>
  </p>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watchEffect } from 'vue';
import { bottomBarShown, keyContext, keyLabel, lineFor } from '@/keys';
import { helpOpen } from '@/shortcuts';

const wide = ref(true);
const media = window.matchMedia('(min-width: 48rem)');
const onMedia = () => (wide.value = media.matches);
onMedia();
media.addEventListener('change', onMedia);
onBeforeUnmount(() => {
  media.removeEventListener('change', onMedia);
  document.body.classList.remove('gb-has-keyline');
});

const line = computed(() => lineFor(keyContext.value));
const shown = computed(() => wide.value && keyContext.value.length > 0 && !bottomBarShown.value);
// the page keeps room for it, so it covers no row
watchEffect(() => document.body.classList.toggle('gb-has-keyline', shown.value));
</script>

<style scoped>
.gb-keyline { position: fixed; left: 0; right: 0; bottom: 0; z-index: 20; display: flex; justify-content: flex-end; flex-wrap: wrap; gap: .25rem 1.125rem; margin: 0; padding: .375rem var(--gutter, 1.75rem); border-top: var(--border-w) solid var(--border-subtle); background: var(--bg-subtle); color: var(--fg-muted); font-size: .78125rem; }
.gb-keyline-k { display: inline-flex; align-items: center; gap: .25rem; }
.gb-keyline-keys { display: inline-flex; gap: .1875rem; padding: 0; border: 0; background: none; font: inherit; color: inherit; }
button.gb-keyline-keys { cursor: pointer; }
button.gb-keyline-keys:hover kbd { border-color: var(--fg); color: var(--fg); }
button.gb-keyline-keys:focus-visible { outline: var(--focus-w) solid var(--primary-hover); outline-offset: .125rem; }
kbd { min-width: .75rem; padding: 0 .3125rem; border: var(--border-w) solid var(--border-strong); border-bottom-width: .125rem; border-radius: var(--radius); background: var(--bg); color: var(--fg); font-family: var(--font-sans); font-size: .71875rem; line-height: 1.35; text-align: center; }
</style>
