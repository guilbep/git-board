<!-- layout = "table": one table per group. Its columns are the view's
     (ColumnChooser, tablecols.ts): by default the CLI's (cmd/git-board
     render.go) id, title, state, one per field, then the labels without a
     namespace; Links (the forge issues) on demand. A field's cell is edited
     in place (FieldCell); rows are picked with their box (Shift: a range)
     for the bulk bar. The title opens the issue. -->
<template>
  <div class="tables">
    <div class="table-tools">
      <ColumnChooser :columns="columns" :names="names" @update:columns="(c) => emit('update:columns', c)" />
    </div>

    <!-- what to do with the picked rows: one change per issue, in turn -->
    <div v-if="picked.length" class="bulk" role="region" aria-label="Edit the selected issues">
      <b class="bulk-count">{{ picked.length }} selected</b>
      <select
        v-for="f in bulkFields"
        :key="f"
        class="epfl-select bulk-control"
        :aria-label="`Set ${displayName(f)} on the selected issues`"
        :disabled="bulkBusy"
        @change="bulkSet(f, $event)"
      >
        <option value="" selected disabled>Set {{ displayName(f) }}…</option>
        <option value="(none)">Not set</option>
        <option v-for="o in fields[f]?.options ?? []" :key="o" :value="o">{{ formatValue(f, o, fields) }}</option>
      </select>
      <form v-for="f in personFields" :key="f" class="bulk-form" @submit.prevent="bulkAssign(f)">
        <input
          v-model="assign[f]"
          class="epfl-input bulk-control"
          autocomplete="off" data-1p-ignore data-lpignore="true" data-form-type="other"
          :aria-label="`Add to ${displayName(f)} on the selected issues`"
          :placeholder="`${displayName(f)}: add, or @me`"
          :list="fields[f]?.people?.length ? `bulk-people-${f}` : undefined"
          :disabled="bulkBusy"
        />
        <datalist v-if="fields[f]?.people?.length" :id="`bulk-people-${f}`"><option v-for="p in fields[f]!.people" :key="p" :value="p" /></datalist>
      </form>
      <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="bulkBusy" @click="bulkClose">Close</button>
      <button type="button" class="bulk-clear" :disabled="bulkBusy" @click="selected = new Set()">Clear</button>
      <span v-if="bulkStatus" class="bulk-status" role="status">{{ bulkStatus }}</span>
      <p v-if="bulkError" class="bulk-error" role="alert">{{ bulkError }}</p>
    </div>

    <section v-for="bucket in buckets" :key="bucket.group" class="table-group">
      <h2 v-if="group" class="group-title">
        <ValueMark :field="group" :value="bucket.group" />
        {{ groupLabel(bucket.group) }}
        <span class="group-count">{{ (bucket.total ?? bucket.issues.length).toLocaleString() }}</span>
      </h2>
      <div class="table-scroll">
        <table class="issue-table" :class="{ 'lead-sticky': cols[0] === 'id' && cols[1] === 'title' }">
          <thead>
            <tr>
              <th scope="col" class="cell-pick">
                <input
                  type="checkbox"
                  :aria-label="`Select all ${bucket.issues.length} issues${group ? ` in ${groupLabel(bucket.group)}` : ''}`"
                  :checked="bucket.issues.length > 0 && bucket.issues.every((i) => selected.has(i.id))"
                  :indeterminate.prop="bucket.issues.some((i) => selected.has(i.id)) && !bucket.issues.every((i) => selected.has(i.id))"
                  @change="toggleGroup(bucket)"
                />
              </th>
              <th
                v-for="key in cols"
                :key="key"
                scope="col"
                :class="{ 'cell-id': key === 'id', 'cell-title': key === 'title' }"
                :aria-sort="sort?.key === key ? (sort.desc ? 'descending' : 'ascending') : undefined"
              >
                <button v-if="key !== 'other'" type="button" class="th-sort" @click="emit('sort', key)">
                  {{ label(key) }}<span class="th-arrow" aria-hidden="true">{{ sort?.key === key ? (sort.desc ? '↓' : '↑') : '' }}</span>
                </button>
                <template v-else>{{ label(key) }}</template>
              </th>
            </tr>
          </thead>
          <tbody :ref="(el) => bodies.set(bucket.group, el as HTMLElement | null)">
            <!-- a long list's rows off screen: their height, not them
                 (windowrows.ts) -->
            <tr v-if="win(bucket)[0] > 0" class="row-spacer" aria-hidden="true" :style="{ height: `${win(bucket)[0] * rowHeight}px` }" />
            <!-- The whole row opens the issue, but its box and its field
                 cells (edited in place); the title button is the keyboard
                 entry point (its click bubbles up to the row). -->
            <tr v-for="issue in bucket.issues.slice(...win(bucket))" :key="issue.id" :class="{ 'is-picked': selected.has(issue.id) }" @click="emit('open', issue)" @mouseenter="hover(issue.id)" @focusin="hover(issue.id)" @mouseleave="unhover">
              <td class="cell-pick" @click.stop>
                <input type="checkbox" :aria-label="`Select ${issue.shown_title}`" :checked="selected.has(issue.id)" @click="toggleRow(issue.id, $event)" />
              </td>
              <template v-for="key in cols" :key="key">
                <td v-if="key === 'id'" class="cell-id">{{ issue.id.slice(0, 8) }}</td>
                <td v-else-if="key === 'title'" class="cell-title"><span v-if="changed?.has(issue.id)" class="gb-changed-dot" title="Changed since your last visit" /><button type="button" class="title-button"><span v-if="issue.number" class="gb-num">{{ issue.number }}</span>{{ issue.shown_title }}</button></td>
                <td v-else-if="key === 'is'">{{ issue.status }}</td>
                <td v-else-if="key === 'links'" class="cell-links" @click.stop>
                  <a v-for="f in issue.forges ?? []" :key="f.url" :href="safeHref(f.url)" target="_blank" rel="noopener noreferrer" class="forge-link" :title="`Open on ${f.name}${f.number ? ` #${f.number}` : ''}`">
                    {{ f.name }} #{{ f.number || '?' }}
                  </a>
                </td>
                <td v-else-if="key === 'other'" class="cell-other">{{ otherLabels(issue.labels, names).join(', ') }}</td>
                <FieldCell v-else :issue="issue" :name="key" :fields="fields" @saved="emit('changed')" />
              </template>
            </tr>
            <tr v-if="win(bucket)[1] < bucket.issues.length" class="row-spacer" aria-hidden="true" :style="{ height: `${(bucket.issues.length - win(bucket)[1]) * rowHeight}px` }" />
            <!-- the list's next page, asked as its end comes near -->
            <tr v-if="bucket.next" v-near="() => emit('more', bucket)" class="row-more">
              <td :colspan="cols.length + 1">
                <button type="button" class="gb-linkbtn" @click="emit('more', bucket)">{{ ((bucket.total ?? 0) - bucket.issues.length).toLocaleString() }} more</button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

  </div>
</template>

<script setup lang="ts">
import { prefetchThread } from '@/issue';
import { safeHref } from '@/links';
import { computed, onUpdated, reactive, ref, watch, onBeforeUnmount } from 'vue';
import { scrollerOf, vNear } from '@/near';
import { LONG, useScroll, windowRange } from '@/windowrows';
import { patchIssue, type Issue, type IssueChange } from '@/api';
import type { Bucket } from '@/bucket';
import type { Sort } from '@/facets';
import { displayName, formatValue, otherLabels, typeOf, type Fields } from '@/fields';
import { message } from '@/issue';
import { rangeIds, shownColumns } from '@/tablecols';
import ColumnChooser from '@/components/ColumnChooser.vue';
import FieldCell from '@/components/FieldCell.vue';
import GbIcon from '@/components/GbIcon.vue';
import ValueMark from '@/components/ValueMark.vue';

const props = defineProps<{
  changed?: Set<string>; // since your last visit: a dot
  buckets: Bucket[];
  names: string[];
  fields: Fields;
  group: string;
  sort: Sort | null;
  columns?: string[] | undefined;
}>();
const emit = defineEmits<{ open: [issue: Issue]; sort: [key: string]; changed: []; more: [bucket: Bucket]; 'update:columns': [columns: string[] | undefined] }>();

// windowed rows (windowrows.ts): each group's body, where it is on screen
// as the page scrolls, and the rows' height, measured once some show. A
// group's range changes only when its rows do: a scroll within them
// renders nothing.
const bodies = new Map<string, HTMLElement | null>();
const scrollers = new WeakMap<HTMLElement, Element | null>();
const scroll = useScroll();
const rowHeight = ref(41);
const ranges = ref<Record<string, [number, number]>>({});
function rangeOf(b: Bucket): [number, number] {
  if (b.issues.length <= LONG) return [0, b.issues.length];
  const body = bodies.get(b.group);
  if (!body) return [0, LONG];
  if (!scrollers.has(body)) scrollers.set(body, scrollerOf(body));
  // the rows' top, from the top of what shows them: the table's box, cut
  // to the window
  const box = scrollers.get(body)?.getBoundingClientRect() ?? { top: 0, bottom: window.innerHeight };
  const from = Math.max(box.top, 0);
  const to = Math.min(box.bottom, window.innerHeight);
  return windowRange(b.issues.length, body.getBoundingClientRect().top - from, Math.max(to - from, 0), rowHeight.value);
}
watch(
  [scroll, () => props.buckets, rowHeight],
  () => {
    for (const b of props.buckets) {
      const r = rangeOf(b);
      const was = ranges.value[b.group];
      if (!was || was[0] !== r[0] || was[1] !== r[1]) ranges.value = { ...ranges.value, [b.group]: r };
    }
  },
  { flush: 'post', immediate: true },
);
const win = (b: Bucket): [number, number] => ranges.value[b.group] ?? [0, Math.min(b.issues.length, LONG)];
onUpdated(() => {
  const row = [...bodies.values()].find(Boolean)?.querySelector<HTMLElement>('tr:not(.row-spacer):not(.row-more)');
  if (row?.offsetHeight) rowHeight.value = row.offsetHeight;
});

const cols = computed(() => shownColumns(props.columns, props.names));
const labels: Record<string, string> = { id: 'ID', title: 'Title', is: 'State', other: 'Other labels', links: 'On the forge' };
const label = (key: string) => labels[key] ?? displayName(key);

function groupLabel(value: string): string {
  if (value === '(none)') return `No ${props.group}`;
  return formatValue(props.group, value, props.fields);
}

// rows picked with their box; Shift picks the run from the last one
const selected = ref(new Set<string>());
const order = computed(() => props.buckets.flatMap((b) => b.issues.map((i) => i.id)));
const picked = computed(() => props.buckets.flatMap((b) => b.issues).filter((i) => selected.value.has(i.id)));
let anchor = '';
function toggleRow(id: string, e: MouseEvent) {
  const on = !selected.value.has(id);
  const ids = e.shiftKey && anchor ? rangeIds(order.value, anchor, id) : [id];
  const next = new Set(selected.value);
  for (const x of ids) {
    if (on) next.add(x);
    else next.delete(x);
  }
  selected.value = next;
  anchor = id;
}
function toggleGroup(b: Bucket) {
  const all = b.issues.every((i) => selected.value.has(i.id));
  const next = new Set(selected.value);
  for (const i of b.issues) {
    if (all) next.delete(i.id);
    else next.add(i.id);
  }
  selected.value = next;
}

// the bulk bar: a select per field of options, an add box per person field
const bulkFields = computed(() => props.names.filter((n) => props.fields[n]?.single && props.fields[n]?.options?.length));
const personFields = computed(() => props.names.filter((n) => typeOf(n, props.fields) === 'person'));
const assign = reactive<Record<string, string>>({});
const bulkBusy = ref(false);
const bulkStatus = ref('');
const bulkError = ref('');

// one change per issue, in turn; the first refusal stops the run and says
// which issue it was
async function runBulk(what: string, change: (issue: Issue) => Promise<unknown>) {
  const issues = picked.value;
  bulkBusy.value = true;
  bulkError.value = '';
  let done = 0;
  for (const issue of issues) {
    bulkStatus.value = `${what}: ${done + 1} of ${issues.length}…`;
    try {
      await change(issue);
      done++;
    } catch (e) {
      bulkError.value = `Stopped at ${issue.id.slice(0, 7)} “${issue.shown_title}”: ${message(e)} (${done} of ${issues.length} done).`;
      break;
    }
  }
  bulkBusy.value = false;
  bulkStatus.value = '';
  if (done === issues.length) selected.value = new Set();
  emit('changed');
}

function bulkSet(field: string, e: Event) {
  const el = e.target as HTMLSelectElement;
  const v = el.value;
  el.value = '';
  if (v === '(none)') {
    void runBulk(`Clearing ${displayName(field)}`, (i) => {
      const old = i.labels.filter((l) => l.startsWith(`${field}:`));
      return old.length ? write(i, { labels: { remove: old } }) : Promise.resolve();
    });
  } else {
    void runBulk(`Setting ${displayName(field)}`, (i) => write(i, { set: [`${field}=${v}`] }));
  }
}

function bulkAssign(field: string) {
  const v = (assign[field] ?? '').trim();
  if (!v) return;
  assign[field] = '';
  void runBulk(`Adding to ${displayName(field)}`, (i) => write(i, { set: [`${field}=+${v}`] }));
}

function bulkClose() {
  void runBulk('Closing', (i) => (i.status === 'closed' ? Promise.resolve() : write(i, { status: 'closed' })));
}

// one issue's write, from the version the row was read at
async function write(i: Issue, change: IssueChange) {
  const t = await patchIssue(i.id, i.version, change);
  i.version = t.version;
  i.labels = t.labels;
  i.status = t.status;
}

// a row rested on 80 ms reads its issue (prefetchThread): the click opens it at once
let hovering: ReturnType<typeof setTimeout> | undefined;
function hover(id: string) {
  clearTimeout(hovering);
  hovering = setTimeout(() => void prefetchThread(id), 80);
}
const unhover = () => clearTimeout(hovering);
onBeforeUnmount(unhover);
</script>

<style scoped>
.table-tools { display: flex; justify-content: flex-end; margin-bottom: var(--space-3); }
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
.issue-table :deep(td) {
  padding: var(--space-2) var(--space-3);
  text-align: left;
  vertical-align: top;
  border-bottom: var(--border-w) solid var(--border-subtle);
}
.issue-table th {
  background: var(--bg-subtle);
  border-bottom-color: var(--border-strong);
  font-weight: var(--w-bold);
  white-space: nowrap;
}
.issue-table th:has(.th-sort) { padding: 0; }
.th-sort {
  width: 100%;
  padding: var(--space-2) var(--space-3);
  font: inherit;
  color: var(--fg);
  text-align: inherit;
  background: none;
  border: 0;
  cursor: pointer;
}
.th-sort:hover { color: var(--primary-hover); }
.th-sort:focus-visible { outline: var(--focus-w) solid var(--primary-hover); outline-offset: -0.125rem; }
.th-arrow { display: inline-block; min-width: 1em; padding-left: var(--space-1); }

.cell-pick { width: 2.5rem; min-width: 2.5rem; box-sizing: border-box; padding-right: 0 !important; cursor: default; }
.cell-pick input { width: 1rem; height: 1rem; margin: .125rem 0 0; accent-color: var(--primary-hover); cursor: pointer; }

/* the box, ID and Title stay put when the table scrolls sideways, while
   ID and Title lead (not on phones: they'd be wider than the screen) */
@media (min-width: 48rem) {
  .cell-pick { position: sticky; left: 0; z-index: 1; background: var(--bg); }
  th.cell-pick { background: var(--bg-subtle); }
  .lead-sticky .cell-id,
  .lead-sticky .cell-title { position: sticky; z-index: 1; background: var(--bg); }
  .lead-sticky .cell-id { left: 2.5rem; width: 6.5rem; min-width: 6.5rem; box-sizing: border-box; }
  .lead-sticky .cell-title { left: 9rem; border-right: var(--border-w) solid var(--border-subtle); }
  .lead-sticky th.cell-id,
  .lead-sticky th.cell-title { background: var(--bg-subtle); }
}
.issue-table tbody tr:hover :deep(td) { background: var(--bg-subtle); }
.issue-table tbody tr.is-picked :deep(td) { background: color-mix(in srgb, var(--info) 8%, var(--bg)); }
.issue-table tbody tr:last-child :deep(td) { border-bottom: 0; }
.issue-table tbody tr { cursor: pointer; transition: background var(--t-fast); }

.cell-id { font-family: var(--font-mono); color: var(--fg-muted); white-space: nowrap; }
.cell-title { min-width: 18rem; }
.cell-other { color: var(--fg-muted); }
.cell-links { white-space: nowrap; cursor: default; }
.forge-link { display: flex; align-items: center; gap: .25rem; color: var(--fg-muted); font-family: var(--font-mono); font-size: var(--fs-sm); text-decoration: none; }
.forge-link:hover { text-decoration: underline; }
.forge-link + .forge-link { margin-top: .25rem; }

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

/* the bulk bar, while rows are picked: on top, stuck under the page's
   toolbar, above the tables' sticky column titles (z-index 10) */
.bulk {
  position: sticky;
  top: calc(var(--header-h, 0px) + var(--gb-tools-h, 0px));
  z-index: 15;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2) var(--space-3);
  margin-bottom: var(--space-3);
  padding: var(--space-2) var(--space-3);
  background: var(--bg);
  border: var(--border-w) solid var(--fg);
  border-left: 4px solid var(--primary-hover);
  box-shadow: var(--shadow);
}
.bulk-count { margin-right: var(--space-2); }
.bulk-control { width: auto; min-width: 10rem; min-height: 2rem; padding-top: .125rem; padding-bottom: .125rem; font-size: var(--fs-sm); }
.bulk-form { display: contents; }
.bulk-clear { padding: 0; border: 0; background: none; color: var(--fg); font: inherit; font-size: var(--fs-sm); text-decoration: underline; cursor: pointer; }
.bulk-status { font-size: var(--fs-sm); color: var(--fg-muted); }
.bulk-error { flex-basis: 100%; margin: 0; font-size: var(--fs-sm); color: var(--danger-fg); }
.row-more td { text-align: center; }
</style>
