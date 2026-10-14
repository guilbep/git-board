<!-- layout = "board": one column per group, cards in the server's sort
     order. Columns scroll sideways inside the board, never the page. -->
<template>
  <div class="board">
    <section
      v-for="bucket in buckets"
      :key="bucket.group"
      class="board-column"
      :aria-label="`${groupLabel(bucket.group)}, ${bucket.issues.length} issues`"
    >
      <h2 class="column-title">
        {{ groupLabel(bucket.group) }}
        <span class="column-count">{{ bucket.issues.length }}</span>
      </h2>
      <ul class="cards">
        <li v-for="issue in bucket.issues" :key="issue.id">
          <button type="button" class="issue-card" @click="emit('open', issue)">
            <span class="card-id">{{ issue.id.slice(0, 8) }}</span>
            <span class="card-title">{{ issue.title }}</span>
            <span v-if="meta(issue).length" class="card-meta">
              <span v-for="m in meta(issue)" :key="m.name">
                <span class="meta-name">{{ displayName(m.name) }}</span> {{ m.value }}
              </span>
            </span>
            <span v-if="otherLabels(issue.labels, fields).length" class="card-other">
              {{ otherLabels(issue.labels, fields).join(', ') }}
            </span>
          </button>
        </li>
      </ul>
    </section>
  </div>
</template>

<script setup lang="ts">
import type { Bucket, Issue } from '@/api/board';
import {
  displayName, fieldNames, fieldValues, formatValue, otherLabels, type Fields,
} from '@/fields';

const props = defineProps<{ buckets: Bucket[]; fields: Fields; group: string }>();
const emit = defineEmits<{ open: [issue: Issue] }>();

function groupLabel(value: string): string {
  if (!props.group) return 'All issues';
  if (value === '(none)') return `No ${props.group}`;
  return formatValue(props.group, value, props.fields);
}

// Every configured field but the one the board is grouped by: the column
// already says it.
function meta(issue: Issue) {
  return fieldNames(props.fields)
    .filter((name) => name !== props.group)
    .map((name) => ({
      name,
      value: fieldValues(issue.labels, name).map((v) => formatValue(name, v, props.fields)).join(', '),
    }))
    .filter((m) => m.value);
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
.issue-card:focus-visible { outline: var(--focus-w) solid var(--primary-hover); outline-offset: 0.125rem; }

.card-id { font-family: var(--font-mono); font-size: var(--fs-xs); color: var(--fg-muted); }
.card-title { font-size: 0.9375rem; font-weight: var(--w-bold); line-height: var(--lh-normal); overflow-wrap: anywhere; }
.card-meta { display: flex; flex-wrap: wrap; gap: var(--space-1) var(--space-3); font-size: var(--fs-sm); }
.meta-name { color: var(--fg-muted); }
.card-other { font-size: var(--fs-xs); color: var(--fg-muted); }
</style>
