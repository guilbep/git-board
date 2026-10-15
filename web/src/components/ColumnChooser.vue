<!-- The table's columns: shown or hidden, and their order. The choice is
     the view's (saved with Save view); the default columns stay dynamic,
     a new field getting its own. -->
<template>
  <details ref="box" class="cols">
    <summary class="epfl-btn epfl-btn-secondary epfl-btn-sm"><GbIcon name="title" small /> Columns</summary>
    <div class="cols-panel" role="group" aria-label="Table columns">
      <ol class="cols-list">
        <li v-for="(key, i) in list" :key="key" class="cols-row">
          <label class="cols-check">
            <input type="checkbox" :checked="shown.includes(key)" :disabled="key === 'title'" @change="emit('update:columns', savedColumns(toggleColumn(shown, key), names))" />
            {{ label(key) }}
          </label>
          <span v-if="shown.includes(key)" class="cols-move">
            <button type="button" class="cols-btn" :aria-label="`Move ${label(key)} left`" :disabled="i === 0" @click="move(key, -1)">↑</button>
            <button type="button" class="cols-btn" :aria-label="`Move ${label(key)} right`" :disabled="i === shown.length - 1" @click="move(key, 1)">↓</button>
          </span>
        </li>
      </ol>
      <button type="button" class="cols-reset" :disabled="!columns" @click="emit('update:columns', undefined)">Default columns</button>
    </div>
  </details>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { displayName } from '@/fields';
import { chooserColumns, moveColumn, savedColumns, shownColumns, toggleColumn } from '@/tablecols';
import GbIcon from '@/components/GbIcon.vue';

const props = defineProps<{ columns: string[] | undefined; names: string[] }>();
const emit = defineEmits<{ 'update:columns': [columns: string[] | undefined] }>();

const shown = computed(() => shownColumns(props.columns, props.names));
const list = computed(() => chooserColumns(props.columns, props.names));
const labels: Record<string, string> = { id: 'ID', title: 'Title', is: 'State', other: 'Other labels', links: 'Forge' };
const label = (key: string) => labels[key] ?? displayName(key);
const move = (key: string, by: -1 | 1) => emit('update:columns', savedColumns(moveColumn(shown.value, key, by), props.names));

// a click elsewhere folds the list, as a menu does
const box = ref<HTMLDetailsElement | null>(null);
const fold = (e: MouseEvent) => {
  if (box.value?.open && !box.value.contains(e.target as Node)) box.value.open = false;
};
onMounted(() => document.addEventListener('click', fold));
onBeforeUnmount(() => document.removeEventListener('click', fold));
</script>

<style scoped>
.cols { position: relative; }
.cols summary { display: inline-flex; align-items: center; gap: .375rem; list-style: none; cursor: pointer; }
.cols summary::-webkit-details-marker { display: none; }
.cols-panel { position: absolute; right: 0; z-index: 15; /* above the sticky column titles */ width: 16rem; margin-top: .375rem; padding: .5rem; background: var(--bg); border: var(--border-w) solid var(--border-strong); box-shadow: var(--shadow); }
.cols-list { margin: 0; padding: 0; list-style: none; max-height: 22rem; overflow: auto; }
.cols-row { display: flex; align-items: center; justify-content: space-between; gap: .5rem; padding: .25rem .25rem; }
.cols-row + .cols-row { border-top: 1px solid var(--border-subtle); }
.cols-check { display: inline-flex; align-items: center; gap: .5rem; font-size: var(--fs-sm); cursor: pointer; }
.cols-check input { accent-color: var(--primary-hover); }
.cols-move { display: inline-flex; gap: .125rem; }
.cols-btn { width: 1.5rem; height: 1.5rem; padding: 0; border: 1px solid var(--border); background: none; color: var(--fg); cursor: pointer; }
.cols-btn:disabled { opacity: .35; cursor: default; }
.cols-reset { margin-top: .5rem; padding: 0; border: 0; background: none; color: var(--fg); font: inherit; font-size: var(--fs-sm); text-decoration: underline; cursor: pointer; }
.cols-reset:disabled { color: var(--fg-disabled); text-decoration: none; cursor: default; }
</style>
