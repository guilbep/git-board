<!-- layout = "table": one table per group, the CLI's columns (cmd/git-board
     render.go): id, title, state, one column per configured field, then the
     labels no field claims. -->
<template>
  <section v-for="bucket in buckets" :key="bucket.group" class="table-group">
    <h2 v-if="group" class="group-title">
      {{ groupLabel(bucket.group) }}
      <span class="group-count">{{ bucket.issues.length }}</span>
    </h2>
    <div class="table-scroll">
      <table class="issue-table">
        <thead>
          <tr>
            <th scope="col">ID</th>
            <th scope="col">Title</th>
            <th scope="col">State</th>
            <th v-for="name in names" :key="name" scope="col">{{ displayName(name) }}</th>
            <th scope="col">Other labels</th>
          </tr>
        </thead>
        <tbody>
          <!-- The whole row opens the issue; the title button is the
               keyboard entry point (its click bubbles up to the row). -->
          <tr v-for="issue in bucket.issues" :key="issue.id" @click="emit('open', issue)">
            <td class="cell-id">{{ issue.id.slice(0, 8) }}</td>
            <td class="cell-title"><button type="button" class="title-button">{{ issue.title }}</button></td>
            <td>{{ issue.status }}</td>
            <td v-for="name in names" :key="name">{{ cell(issue, name) }}</td>
            <td class="cell-other">{{ otherLabels(issue.labels, fields).join(', ') }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { Bucket, Issue } from '@/api/board';
import {
  displayName, fieldNames, fieldValues, formatValue, otherLabels, type Fields,
} from '@/fields';

const props = defineProps<{ buckets: Bucket[]; fields: Fields; group: string }>();
const emit = defineEmits<{ open: [issue: Issue] }>();

const names = computed(() => fieldNames(props.fields));

function groupLabel(value: string): string {
  if (value === '(none)') return `No ${props.group}`;
  return formatValue(props.group, value, props.fields);
}

function cell(issue: Issue, name: string): string {
  return fieldValues(issue.labels, name).map((v) => formatValue(name, v, props.fields)).join(', ');
}
</script>

<style scoped>
.table-group + .table-group { margin-top: var(--space-5); }

.group-title {
  display: flex;
  align-items: baseline;
  gap: var(--space-2);
  margin: 0 0 var(--space-3);
  font-size: var(--fs-h5);
  font-weight: var(--w-bold);
  line-height: var(--lh-normal);
}
.group-count { font-size: var(--fs-sm); font-weight: var(--w-normal); color: var(--fg-muted); }

.table-scroll { overflow-x: auto; border: var(--border-w) solid var(--border); }

.issue-table {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--fs-sm);
  line-height: var(--lh-normal);
}
.issue-table th,
.issue-table td {
  padding: var(--space-2) var(--space-3);
  text-align: left;
  vertical-align: top;
  border-bottom: var(--border-w) solid var(--border-subtle);
}
.issue-table th {
  background: var(--bg-subtle);
  border-bottom-color: var(--border);
  font-weight: var(--w-bold);
  white-space: nowrap;
}
.issue-table tbody tr:last-child td { border-bottom: 0; }
.issue-table tbody tr { cursor: pointer; transition: background var(--t-fast); }
.issue-table tbody tr:hover { background: var(--bg-subtle); }

.cell-id { font-family: var(--font-mono); color: var(--fg-muted); white-space: nowrap; }
.cell-title { min-width: 18rem; }
.cell-other { color: var(--fg-muted); }

.title-button {
  padding: 0;
  font: inherit;
  text-align: left;
  color: var(--fg);
  background: none;
  border: 0;
  cursor: pointer;
  transition: color var(--t-fast);
}
.issue-table tbody tr:hover .title-button { color: var(--primary-hover); }
.title-button:focus-visible { outline: var(--focus-w) solid var(--primary-hover); outline-offset: 0.125rem; }
</style>
