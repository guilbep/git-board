<!-- One element of a section — a field, a view, a sprint: its name (fixed
     once saved: the file's tables are named by it), what it holds, and
     Remove, which asks. -->
<template>
  <div class="cc">
    <div class="cc-head">
      <slot name="head"><h4 class="cc-title">{{ title }}</h4></slot>
      <span class="gb-grow" />
      <button v-if="removable" type="button" class="gb-linkbtn cc-remove" @click="asking = true">Remove</button>
    </div>
    <div v-if="asking" class="cc-confirm" role="alert">
      <p>{{ confirm }}</p>
      <div class="cc-do">
        <button type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" @click="$emit('remove'); asking = false">Remove</button>
        <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" @click="asking = false">Keep</button>
      </div>
    </div>
    <slot v-else />
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue';

defineProps<{
  title: string;
  removable?: boolean;
  confirm?: string; // what Remove says before it does
}>();

defineEmits<{ remove: [] }>();

defineSlots<{
  /** the head's left side: the title by default, a richer head per card */
  head?(): unknown;
  /** the card's body, shown while Remove isn't asking */
  default?(): unknown;
}>();

const asking = ref(false);
</script>

<style scoped>
.cc { display: grid; gap: .625rem; padding: .75rem 1rem; border: 1px solid var(--border); border-left: 3px solid var(--border-strong); background: var(--bg); }
.cc-head { display: flex; flex-wrap: wrap; align-items: baseline; gap: .375rem .75rem; min-width: 0; }
.cc-title { margin: 0; font-size: 1rem; }
.gb-grow { flex: 1; }
.cc-remove { font-size: .8125rem; }
.cc-confirm { display: grid; gap: .5rem; padding: .625rem .75rem; background: var(--warning-bg); border-left: 3px solid var(--warning); }
.cc-confirm p { margin: 0; font-size: .9375rem; }
.cc-do { display: flex; gap: .5rem; }
</style>
