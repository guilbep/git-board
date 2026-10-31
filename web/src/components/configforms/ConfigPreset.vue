<!-- A preset menu: Add opens the list, each preset with its one-line why;
     picking one adds and selects it, it never overwrites silently. -->
<template>
  <span class="cpm">
    <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :aria-expanded="open" @click="open = !open">{{ label }}</button>
    <div v-if="open" class="cpm-menu" role="menu">
      <button v-for="(p, i) in presets" :key="p.label" type="button" role="menuitem" class="cpm-item" @click="pick(i)">
        <b>{{ p.label }}</b>
        <span class="cpm-why">{{ p.why }}</span>
      </button>
      <slot />
    </div>
  </span>
</template>

<script setup lang="ts">
import { ref } from 'vue';

defineProps<{
  label: string; // "Add a field", "Add a view"
  presets: { label: string; why: string }[];
}>();

const emit = defineEmits<{ pick: [index: number] }>();

const open = ref(false);

function pick(i: number) {
  open.value = false;
  emit('pick', i);
}
</script>

<style scoped>
.cpm { position: relative; display: inline-block; }
.cpm-menu { position: absolute; top: 100%; left: 0; z-index: 20; min-width: 22rem; max-height: 24rem; overflow: auto; margin-top: .25rem; padding: .25rem; border: 1px solid var(--border-strong); background: var(--bg); display: grid; }
.cpm-item { display: grid; gap: .125rem; padding: .5rem .625rem; border: 0; background: none; color: var(--fg); font: inherit; text-align: left; cursor: pointer; }
.cpm-item:hover { background: var(--bg-subtle); }
.cpm-item b { font-size: .9375rem; }
.cpm-why { font-size: .8125rem; color: var(--fg-muted); }
</style>
