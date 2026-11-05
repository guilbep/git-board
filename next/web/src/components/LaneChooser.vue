<!-- A board's lanes: which values of the field it is grouped by become
     columns, in which order, and how many cards a lane holds at most. The
     choice is the view's (saved with Save view): a board per role. -->
<template>
  <details ref="box" class="lanes">
    <summary class="epfl-btn epfl-btn-secondary epfl-btn-sm">Lanes<span v-if="lanes" class="lanes-n">{{ lanes.length }}</span></summary>
    <div class="lanes-panel" role="group" aria-label="Board lanes">
      <ol class="lanes-list">
        <li v-for="v in list" :key="v" class="lanes-row">
          <label class="lanes-check">
            <input type="checkbox" :checked="shown.includes(v)" @change="emit('update:lanes', toggleLane(lanes, v, values))" />
            <ValueMark :field="field" :value="v" />{{ v === '(none)' ? `No ${field}` : v }}
          </label>
          <span v-if="shown.includes(v)" class="lanes-tools">
            <input
              class="lanes-limit"
              type="number"
              min="0"
              :value="limits?.[v] ?? ''"
              placeholder="max"
              :aria-label="`At most this many cards in ${v}`"
              @change="setLimit(v, ($event.target as HTMLInputElement).value)"
            />
            <button type="button" class="lanes-btn" :aria-label="`Move ${v} left`" :disabled="shown.indexOf(v) === 0" @click="emit('update:lanes', moveColumn(shown, v, -1))">←</button>
            <button type="button" class="lanes-btn" :aria-label="`Move ${v} right`" :disabled="shown.indexOf(v) === shown.length - 1" @click="emit('update:lanes', moveColumn(shown, v, 1))">→</button>
          </span>
        </li>
      </ol>
      <button type="button" class="lanes-reset" :disabled="!lanes" @click="emit('update:lanes', undefined)">Every {{ field }}</button>
    </div>
  </details>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { laneChoices, toggleLane } from '@/lanes';
import { moveColumn } from '@/tablecols';
import ValueMark from '@/components/ValueMark.vue';

const props = defineProps<{ lanes: string[] | undefined; values: string[]; field: string; limits: Record<string, number> | undefined }>();
const emit = defineEmits<{ 'update:lanes': [lanes: string[] | undefined]; 'update:limits': [limits: Record<string, number> | undefined] }>();

const shown = computed(() => props.lanes ?? props.values);
const list = computed(() => laneChoices(props.lanes, props.values));

function setLimit(v: string, raw: string) {
  const next = { ...(props.limits ?? {}) };
  const n = Math.floor(Number(raw));
  if (raw.trim() && n > 0) next[v] = n;
  else delete next[v];
  emit('update:limits', Object.keys(next).length ? next : undefined);
}

// a click elsewhere folds the list, as a menu does
const box = ref<HTMLDetailsElement | null>(null);
const fold = (e: MouseEvent) => {
  if (box.value?.open && !box.value.contains(e.target as Node)) box.value.open = false;
};
onMounted(() => document.addEventListener('click', fold));
onBeforeUnmount(() => document.removeEventListener('click', fold));
</script>

<style scoped>
.lanes { position: relative; }
.lanes summary { box-sizing: border-box; display: inline-flex; align-items: center; gap: .375rem; min-height: var(--control-h, 2.5rem); list-style: none; cursor: pointer; }
.lanes summary::-webkit-details-marker { display: none; }
.lanes-n { padding: 0 .375rem; border-radius: 10rem; background: var(--bg-dark); color: var(--fg-inverse); font-size: .75rem; }
.lanes-panel { position: absolute; left: 0; z-index: 30; width: 20rem; margin-top: .375rem; padding: .5rem; background: var(--bg); border: var(--border-w) solid var(--border-strong); box-shadow: var(--shadow); }
.lanes-list { margin: 0; padding: 0; list-style: none; max-height: 24rem; overflow: auto; }
.lanes-row { display: flex; align-items: center; justify-content: space-between; gap: .5rem; padding: .25rem; }
.lanes-row + .lanes-row { border-top: 1px solid var(--border-subtle); }
.lanes-check { display: inline-flex; align-items: center; gap: .5rem; min-width: 0; font-size: var(--fs-sm); cursor: pointer; }
.lanes-check input { accent-color: var(--primary-hover); }
.lanes-tools { display: inline-flex; align-items: center; gap: .125rem; flex: none; }
.lanes-limit { width: 3.5rem; height: 1.5rem; padding: 0 .25rem; border: 1px solid var(--border); background: var(--bg); color: var(--fg); font: inherit; font-size: .8125rem; }
.lanes-btn { width: 1.5rem; height: 1.5rem; padding: 0; border: 1px solid var(--border); background: none; color: var(--fg); cursor: pointer; }
.lanes-btn:disabled { opacity: .35; cursor: default; }
.lanes-reset { margin-top: .5rem; padding: 0; border: 0; background: none; color: var(--fg); font: inherit; font-size: var(--fs-sm); text-decoration: underline; cursor: pointer; }
.lanes-reset:disabled { color: var(--fg-disabled); text-decoration: none; cursor: default; }
</style>
