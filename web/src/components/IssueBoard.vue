<!-- layout = "board": one column per group, cards in the chosen sort
     order. Columns scroll sideways inside the board, never the page.
     Grouped by a field, a card dragged to another column takes that
     column's value (the browser's drag and drop; the panel's field is the
     keyboard way). -->
<template>
  <p v-if="picked.length" class="board-picked" role="status">
    <b>{{ picked.length }} card{{ picked.length === 1 ? '' : 's' }} picked</b>
    <span class="board-picked-hint">drag one of them to a lane to move them all, or</span>
    <select class="epfl-select board-picked-move" aria-label="Move the picked cards to" @change="movePicked($event)">
      <option value="" selected disabled>Move to…</option>
      <option v-for="b in buckets" :key="b.group" :value="b.group">{{ groupLabel(b.group) }}</option>
    </select>
    <button type="button" class="board-picked-clear" @click="picked = []">Clear (Esc)</button>
  </p>
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
        <span class="column-name"><ValueMark v-if="group" :field="group" :value="bucket.group" />{{ groupLabel(bucket.group) }}</span>
        <span class="column-count" :class="`is-${limitState(bucket.issues.length, limits?.[bucket.group]) || 'ok'}`" :title="limits?.[bucket.group] ? `At most ${limits[bucket.group]} cards here` : undefined">{{ bucket.issues.length }}<template v-if="limits?.[bucket.group]"> / {{ limits[bucket.group] }}</template></span>
        <select
          v-if="group && bucket.issues.length && buckets.length > 1"
          class="column-move"
          :aria-label="`Move every card of ${groupLabel(bucket.group)} to another lane`"
          title="Move all to…"
          @change="moveAll(bucket, $event)"
        >
          <option value="" selected disabled>⋯</option>
          <option v-for="b in buckets.filter((x) => x.group !== bucket.group)" :key="b.group" :value="b.group">Move all to {{ groupLabel(b.group) }}</option>
        </select>
      </h2>
      <ul class="cards">
        <li v-for="issue in bucket.issues" :key="issue.id">
          <input
            v-if="group"
            type="checkbox"
            class="card-pick"
            :class="{ 'is-on': picked.length }"
            :checked="picked.includes(issue.id)"
            :aria-label="`Pick ${issue.title}`"
            @click="onPickBox(issue, bucket, $event)"
          />
          <button
            type="button"
            class="issue-card"
            :class="{ 'is-dragging': dragging === issue.id || (dragging && picked.includes(dragging) && picked.includes(issue.id)), 'is-picked': picked.includes(issue.id) }"
            :draggable="!!group"
            @dragstart="onStart(issue, bucket.group, $event)"
            @dragend="onEnd"
            @click="onCard(issue, bucket, $event)"
          >
            <span class="card-id"><span v-if="changed?.has(issue.id)" class="gb-changed-dot" title="Changed since your last visit" />{{ issue.id.slice(0, 8) }}</span>
            <span class="card-title">{{ issue.title }}</span>
            <span v-if="meta(issue).length" class="card-meta">
              <span v-for="m in meta(issue)" :key="m.name" class="meta-item">
                <span class="meta-name">{{ displayName(m.name) }}</span>
                <span v-for="v in m.values" :key="v" class="gb-val" :class="colorClass(m.name, v)"><ValueMark chip :field="m.name" :value="v" />{{ formatValue(m.name, v, fields) }}</span>
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
import ValueMark from '@/components/ValueMark.vue';
import { onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { limitState, pickCards } from '@/lanes';
import type { Bucket, Issue } from '@/api/board';
import { colorClass } from '@/colors';
import { displayName, fieldValues, formatValue, otherLabels, type Fields } from '@/fields';

const props = defineProps<{ buckets: Bucket[]; names: string[]; fields: Fields; group: string; changed?: Set<string>; limits?: Record<string, number> | undefined }>();
const emit = defineEmits<{ open: [issue: Issue]; move: [issue: Issue, from: string, to: string]; moveMany: [moves: { issue: Issue; from: string }[], to: string] }>();

// Several cards picked (Cmd/Ctrl-click toggles one, Shift-click a range in
// its lane, the box on a card's corner too) move together: dragged, or
// sent to a lane from the bar. Esc lets go.
const picked = ref<string[]>([]);
let anchor = '';
watch(() => props.group, () => (picked.value = []));
function pick(issue: Issue, bucket: Bucket, range: boolean) {
  const order = bucket.issues.map((i) => i.id);
  picked.value = pickCards(picked.value, issue.id, order, { toggle: !range, range, anchor });
  if (!range) anchor = issue.id;
}
function onCard(issue: Issue, bucket: Bucket, e: MouseEvent) {
  if (props.group && (e.metaKey || e.ctrlKey || e.shiftKey)) pick(issue, bucket, e.shiftKey && !!anchor);
  else emit('open', issue);
}
function onPickBox(issue: Issue, bucket: Bucket, e: MouseEvent) {
  e.preventDefault(); // the box shows the picked list, set below
  pick(issue, bucket, e.shiftKey && !!anchor);
}
function pickedMoves(): { issue: Issue; from: string }[] {
  return props.buckets.flatMap((b) => b.issues.filter((i) => picked.value.includes(i.id)).map((issue) => ({ issue, from: b.group })));
}
function movePicked(e: Event) {
  const el = e.target as HTMLSelectElement;
  const to = el.value;
  el.value = '';
  const moves = pickedMoves().filter((m) => m.from !== to);
  if (moves.length) emit('moveMany', moves, to);
  picked.value = [];
}
function moveAll(bucket: Bucket, e: Event) {
  const el = e.target as HTMLSelectElement;
  const to = el.value;
  el.value = '';
  const n = bucket.issues.length;
  if (!window.confirm(`Move all ${n} card${n === 1 ? '' : 's'} of ${groupLabel(bucket.group)} to ${groupLabel(to)}?`)) return;
  emit('moveMany', bucket.issues.map((issue) => ({ issue, from: bucket.group })), to);
}
function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape' && picked.value.length && !document.querySelector('.q-dialog')) picked.value = [];
}
onMounted(() => window.addEventListener('keydown', onKey));
onBeforeUnmount(() => window.removeEventListener('keydown', onKey));

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
  if (dragged && picked.value.includes(dragged.id) && picked.value.length > 1) {
    const moves = pickedMoves().filter((m) => m.from !== group);
    if (moves.length) emit('moveMany', moves, group);
    picked.value = [];
  } else if (dragged && group !== dragFrom.value) emit('move', dragged, dragFrom.value, group);
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

@media (min-width: 48rem) {
  /* each column scrolls on its own under its title, the board as tall as
     the window under the sticky tools */
  .board-column { display: flex; flex-direction: column; max-height: calc(100vh - var(--header-h) - var(--gb-tools-h, 0px) - var(--space-4)); }
  .board-column .cards { min-height: 2.5rem; overflow-y: auto; }
}
.board-column {
  background: var(--bg-subtle);
  border: var(--border-w) solid var(--border-subtle);
  padding: var(--space-3);
}

.column-title {
  display: flex;
  align-items: baseline;
  justify-content: flex-start;
  gap: var(--space-2);
  margin: 0 0 var(--space-3);
  font-size: var(--fs-sm);
  font-weight: var(--w-bold);
  line-height: var(--lh-normal);
  text-transform: uppercase;
  letter-spacing: .04em;
}
.column-count { margin-left: auto; color: var(--fg-muted); font-weight: var(--w-normal); font-variant-numeric: tabular-nums; }
.column-count.is-at { padding: 0 .375rem; color: var(--warning-fg); background: var(--warning-bg); font-weight: var(--w-bold); }
.column-count.is-over { padding: 0 .375rem; color: var(--danger-fg); background: var(--danger-bg); font-weight: var(--w-bold); }
.column-move { width: 1.75rem; height: 1.5rem; padding: 0; border: 1px solid transparent; background: none; color: var(--fg-muted); font: inherit; font-weight: var(--w-bold); text-align: center; cursor: pointer; appearance: none; -webkit-appearance: none; }
.column-move:hover, .column-move:focus-visible { border-color: var(--border-strong); color: var(--fg); }
.cards li { position: relative; }
.card-pick { position: absolute; top: .5rem; right: .5rem; z-index: 1; width: 1rem; height: 1rem; margin: 0; opacity: 0; accent-color: var(--primary-hover); cursor: pointer; }
.cards li:hover .card-pick, .card-pick:focus-visible, .card-pick.is-on, .card-pick:checked { opacity: 1; }
.issue-card.is-picked { outline: 2px solid var(--primary-hover); outline-offset: -2px; }
.board-picked { display: flex; flex-wrap: wrap; align-items: center; gap: .5rem .75rem; margin: 0 0 var(--space-3); padding: .5rem .75rem; border: 1px solid var(--border-strong); border-left: 4px solid var(--primary-hover); background: var(--bg); }
.board-picked-hint { color: var(--fg-muted); font-size: var(--fs-sm); }
.board-picked-move { width: auto; min-width: 10rem; }
.board-picked-clear { padding: 0; border: 0; background: none; color: var(--fg); font: inherit; text-decoration: underline; cursor: pointer; }
.column-name { display: inline-flex; align-items: center; gap: var(--space-2); }
.board-column[class*="gb-c-"] { border-top: 3px solid var(--c-dot); }
/* values wrap inside the card: three assignees must not widen it */
.meta-item { display: inline-flex; flex-wrap: wrap; align-items: center; gap: var(--space-1); min-width: 0; }

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
.board-column.is-over { background: var(--bg); outline: 2px dashed var(--fg); outline-offset: -2px; }
.cards { min-height: 3rem; }
.issue-card:focus-visible { outline: var(--focus-w) solid var(--primary-hover); outline-offset: 0.125rem; }

.card-id { font-family: var(--font-mono); font-size: var(--fs-xs); color: var(--fg-muted); }
.card-title { font-size: 0.9375rem; font-weight: var(--w-bold); line-height: var(--lh-normal); overflow-wrap: anywhere; }
.card-meta { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-1) var(--space-3); min-width: 0; font-size: var(--fs-sm); }
.meta-name { color: var(--fg-muted); }
.card-other { font-size: var(--fs-xs); color: var(--fg-muted); }
</style>
