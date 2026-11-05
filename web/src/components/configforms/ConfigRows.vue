<!-- An ordered list of rows: ↑, ↓, ✕ beside each, an add button under.
     The row's editor is the parent's slot; the list itself is held here,
     mutated immutably. -->
<template>
  <div class="cr">
    <div v-for="(row, i) in rows" :key="i" class="cr-row">
      <span class="cr-order">
        <button type="button" class="cr-btn" :disabled="i === 0" :aria-label="`Up`" @click="move(i, -1)">↑</button>
        <button type="button" class="cr-btn" :disabled="i === rows.length - 1" aria-label="Down" @click="move(i, 1)">↓</button>
      </span>
      <div class="cr-main"><slot name="row" :row="row" :index="i" :set="(v: T) => setRow(i, v)" /></div>
      <button type="button" class="cr-btn cr-del" :aria-label="`Remove`" @click="remove(i)">✕</button>
    </div>
    <div class="cr-add">
      <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" @click="$emit('add')">{{ addLabel }}</button>
      <slot name="extra" />
    </div>
  </div>
</template>

<script setup lang="ts" generic="T">
const props = defineProps<{
  rows: T[];
  addLabel: string;
}>();

const emit = defineEmits<{ update: [rows: T[]]; add: [] }>();

defineSlots<{
  row(props: { row: T; index: number; set: (v: T) => void }): unknown;
  extra?(): unknown;
}>();

function move(i: number, d: number) {
  const rows = [...props.rows];
  const a = rows[i];
  const b = rows[i + d];
  if (a === undefined || b === undefined) return;
  rows[i] = b;
  rows[i + d] = a;
  emit('update', rows);
}

// setRow replaces one row: a slot input binds through it, the slot's
// props themselves aren't assignable
function setRow(i: number, v: T) {
  const rows = [...props.rows];
  rows[i] = v;
  emit('update', rows);
}

function remove(i: number) {
  emit('update', props.rows.filter((_, j) => j !== i));
}
</script>

<style scoped>
.cr { display: grid; gap: .375rem; }
.cr-row { display: flex; align-items: center; gap: .5rem; min-width: 0; }
.cr-order { display: flex; flex-direction: column; gap: .125rem; }
.cr-btn { flex: none; width: 1.5rem; height: 1.25rem; padding: 0; border: 1px solid var(--border); background: var(--bg); color: var(--fg); font: inherit; font-size: .6875rem; line-height: 1; cursor: pointer; }
.cr-btn:hover:not(:disabled) { border-color: var(--fg); }
.cr-btn:disabled { color: var(--fg-subtle); cursor: default; }
.cr-del { width: 1.75rem; height: 1.75rem; font-size: .8125rem; }
.cr-del:hover { border-color: var(--danger); color: var(--danger-fg); }
.cr-main { flex: 1; min-width: 0; }
.cr-add { display: flex; align-items: center; gap: .5rem; margin-top: .25rem; }
</style>
