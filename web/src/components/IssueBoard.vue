<!-- layout = "board": one column per group, cards in the chosen sort
     order. Columns scroll sideways inside the board, never the page.
     Grouped by a field, a card dragged to another column takes that
     column's value (the browser's drag and drop; the panel's field is the
     keyboard way). -->
<template>
  <div class="board">
    <section
      v-for="bucket in buckets"
      :key="bucket.group"
      class="board-column"
      :class="[colorClass(group, bucket.group), { 'is-over': over === bucket.group && dragFrom !== bucket.group }]"
      :aria-label="`${groupLabel(bucket.group)}, ${bucket.issues.length} issues`"
      @dragover="onOver(bucket.group, $event)"
      @dragleave="onLeave(bucket.group, $event)"
      @drop="onDrop(bucket.group, $event)"
    >
      <h2 class="column-title">
        <span class="column-name"><span v-if="group && colorClass(group, bucket.group)" class="gb-dot" aria-hidden="true" />{{ groupLabel(bucket.group) }}</span>
        <span class="column-count">{{ bucket.issues.length }}</span>
      </h2>
      <ul class="cards">
        <li v-for="issue in bucket.issues" :key="issue.id">
          <button
            type="button"
            class="issue-card"
            :class="{ 'is-dragging': dragging === issue.id }"
            :draggable="!!group"
            @dragstart="onStart(issue, bucket.group, $event)"
            @dragend="onEnd"
            @click="emit('open', issue)"
          >
            <span class="card-id">{{ issue.id.slice(0, 8) }}</span>
            <span class="card-title">{{ issue.title }}</span>
            <span v-if="meta(issue).length" class="card-meta">
              <span v-for="m in meta(issue)" :key="m.name" class="meta-item">
                <span class="meta-name">{{ displayName(m.name) }}</span>
                <span v-for="v in m.values" :key="v" class="gb-val" :class="colorClass(m.name, v)">{{ formatValue(m.name, v, fields) }}</span>
              </span>
            </span>
            <span v-if="otherLabels(issue.labels, names).length" class="card-other">
              {{ otherLabels(issue.labels, names).join(', ') }}
            </span>
          </button>
        </li>
      </ul>
    </section>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import type { Bucket, Issue } from '@/api/board';
import { colorClass } from '@/colors';
import { displayName, fieldValues, formatValue, otherLabels, type Fields } from '@/fields';

const props = defineProps<{ buckets: Bucket[]; names: string[]; fields: Fields; group: string }>();
const emit = defineEmits<{ open: [issue: Issue]; move: [issue: Issue, from: string, to: string] }>();

const dragging = ref(''); // the card's issue id
const dragFrom = ref('');
const over = ref('');
let dragged: Issue | null = null;

function onStart(issue: Issue, from: string, e: DragEvent) {
  dragged = issue;
  dragging.value = issue.id;
  dragFrom.value = from;
  e.dataTransfer?.setData('text/plain', issue.id);
  if (e.dataTransfer) e.dataTransfer.effectAllowed = 'move';
}
function onOver(group: string, e: DragEvent) {
  if (!dragged) return;
  e.preventDefault(); // a drop target
  if (e.dataTransfer) e.dataTransfer.dropEffect = 'move';
  over.value = group;
}
function onLeave(group: string, e: DragEvent) {
  const to = e.relatedTarget as Node | null;
  if (over.value === group && !(e.currentTarget as HTMLElement).contains(to)) over.value = '';
}
function onDrop(group: string, e: DragEvent) {
  e.preventDefault();
  if (dragged && group !== dragFrom.value) emit('move', dragged, dragFrom.value, group);
  onEnd();
}
function onEnd() {
  dragged = null;
  dragging.value = '';
  dragFrom.value = '';
  over.value = '';
}

function groupLabel(value: string): string {
  if (!props.group) return 'All issues';
  if (value === '(none)') return `No ${props.group}`;
  return formatValue(props.group, value, props.fields);
}

// Every field but the one the board is grouped by: the column already says it.
function meta(issue: Issue) {
  return props.names
    .filter((name) => name !== props.group)
    .map((name) => ({ name, values: fieldValues(issue.labels, name) }))
    .filter((m) => m.values.length);
}
</script>

<style scoped>
.board {
  display: grid;
  grid-auto-flow: column;
  grid-auto-columns: minmax(16rem, 1fr);
  gap: var(--space-3);
  align-items: start;
  overflow-x: auto;
  padding-bottom: var(--space-3);
}

.board-column {
  background: var(--bg-subtle);
  border: var(--border-w) solid var(--border-subtle);
  padding: var(--space-3);
}

.column-title {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: var(--space-2);
  margin: 0 0 var(--space-3);
  font-size: var(--fs-sm);
  font-weight: var(--w-bold);
  line-height: var(--lh-normal);
  text-transform: uppercase;
  letter-spacing: .04em;
}
.column-count { color: var(--fg-muted); font-weight: var(--w-normal); }
.column-name { display: inline-flex; align-items: center; gap: var(--space-2); }
.board-column[class*="gb-c-"] { border-top: 3px solid var(--c-dot); }
.meta-item { display: inline-flex; align-items: center; gap: var(--space-1); }

.cards {
  list-style: none;
  margin: 0;
  padding: 0;
  display: grid;
  gap: var(--space-2);
}

.issue-card {
  display: grid;
  gap: var(--space-1);
  width: 100%;
  padding: var(--space-2) var(--space-3) var(--space-3);
  text-align: left;
  font: inherit;
  color: var(--fg);
  background: var(--bg);
  border: var(--border-w) solid var(--border);
  border-radius: var(--radius);
  cursor: pointer;
  transition: border-color var(--t-fast);
}
.issue-card:hover { border-color: var(--fg); }
.issue-card[draggable="true"] { cursor: grab; }
.issue-card.is-dragging { opacity: .4; }
.board-column.is-over { background: var(--bg); outline: 2px dashed var(--epfl-black); outline-offset: -2px; }
.cards { min-height: 3rem; }
.issue-card:focus-visible { outline: var(--focus-w) solid var(--primary-hover); outline-offset: 0.125rem; }

.card-id { font-family: var(--font-mono); font-size: var(--fs-xs); color: var(--fg-muted); }
.card-title { font-size: 0.9375rem; font-weight: var(--w-bold); line-height: var(--lh-normal); overflow-wrap: anywhere; }
.card-meta { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-1) var(--space-3); font-size: var(--fs-sm); }
.meta-name { color: var(--fg-muted); }
.card-other { font-size: var(--fs-xs); color: var(--fg-muted); }
</style>
