<template>
  <AppHeader :views="viewNames" :active="activeView" />

  <div class="page">
    <FilterSidebar
      v-model:query="filterText"
      :facets="facetList"
      :sel="sel"
      :fields="fields"
      :modified="modified"
      :query-changed="queryChanged"
      @toggle="toggle"
      @reset="resetAll"
      @apply="load"
    />

    <main class="main">
      <div v-if="configError" class="epfl-alert epfl-alert-danger notice" role="alert">
        <b>Could not load the board configuration.</b> {{ configError }}
        <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" @click="loadConfig">Try again</button>
      </div>
      <div v-if="unknownView" class="epfl-alert epfl-alert-warning notice" role="alert">
        There is no saved view named “{{ unknownView }}”. Showing {{ activeView || 'all open issues' }} instead.
      </div>

      <div class="page-head">
        <div class="epfl-eyebrow">{{ activeView ? 'Saved view' : 'Issues' }}</div>
        <h1 class="h3 page-title">{{ activeView || 'All open issues' }}</h1>
        <p class="page-meta">
          {{ layout === 'board' ? 'Board' : 'Table' }}
          <template v-if="groupField"> · grouped by {{ groupField }}</template>
          <template v-if="loading && buckets"> · updating…</template>
        </p>
      </div>

      <div class="toolbar">
        <div class="toolbar-field toolbar-search">
          <label class="epfl-label" for="search">Search <span class="search-hint">titles, descriptions, comments</span></label>
          <input
            id="search"
            class="epfl-input"
            type="search"
            placeholder="Text, assignee:guilbep, label:bug, no:assignee…"
            autocomplete="off"
            :value="q"
            @input="setQuery({ q: ($event.target as HTMLInputElement).value || undefined })"
          />
        </div>
        <div class="toolbar-field">
          <label class="epfl-label" for="group">Group by</label>
          <select id="group" v-model="groupField" class="epfl-select" @change="load">
            <option value="">No grouping</option>
            <option v-for="name in groupChoices" :key="name" :value="name">{{ displayName(name) }}</option>
          </select>
        </div>
        <!-- A table sorts from its column headers; a board has none. -->
        <div v-if="layout === 'board'" class="toolbar-field">
          <label class="epfl-label" for="sort">Sort cards by</label>
          <select id="sort" class="epfl-select" :value="sortValue" @change="onSortSelect">
            <option value="">Default order</option>
            <option v-for="o in sortChoices" :key="o.value" :value="o.value">{{ o.label }}</option>
          </select>
        </div>
        <div class="toolbar-new">
          <button type="button" class="epfl-btn epfl-btn-primary" @click="newOpen = true"><GbIcon name="plus" small /> New issue</button>
          <span class="gb-keys" aria-hidden="true"><kbd>c</kbd></span>
        </div>
      </div>

      <div v-if="buckets" class="status" role="status">
        <span><b>{{ count }}</b> of {{ total }} {{ total === 1 ? 'issue' : 'issues' }}</span>
        <template v-if="chips.length">
          <span class="status-sep" aria-hidden="true">|</span>
          <span v-for="c in chips" :key="c.label" class="chip">
            {{ c.label }}
            <button type="button" :aria-label="`Remove filter ${c.label}`" @click="setQuery(c.patch)">×</button>
          </span>
        </template>
      </div>

      <div :aria-busy="loading">
        <div v-if="listError" class="epfl-alert epfl-alert-danger" role="alert">
          <b>Could not load issues.</b> {{ listError }}
        </div>
        <p v-else-if="!buckets" class="state">Loading issues…</p>
        <div v-else-if="total === 0" class="state state-empty">
          <p class="state-title">No issues match this filter expression.</p>
          <p>Try a broader one, for example <code>is:open</code>.</p>
        </div>
        <div v-else-if="count === 0 && layout !== 'board'" class="state state-empty">
          <p class="state-title">No issues match these filters.</p>
          <p><button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" @click="resetAll">Reset filters</button></p>
        </div>
        <div v-else-if="layout === 'board'">
          <p v-if="moveError" class="epfl-alert epfl-alert-danger notice" role="alert"><b>Not moved.</b> {{ moveError }}</p>
          <IssueBoard
            :buckets="boardBuckets"
            :names="names"
            :fields="fields"
            :group="groupField"
            @open="openIssue"
            @move="onMove"
          />
        </div>
        <IssueTable
          v-else
          :buckets="shown.filter((b) => b.issues.length)"
          :names="names"
          :fields="fields"
          :group="groupField"
          :sort="sort"
          @open="openIssue"
          @sort="sortBy"
        />
      </div>
    </main>
  </div>

  <q-dialog
    v-model="detailOpen"
    :position="panelMode === 'side' ? 'right' : 'standard'"
    :full-height="panelMode === 'side'"
    :class="`gb-dialog-${panelMode}`"
    :backdrop-filter="panelMode === 'wide' ? 'blur(6px)' : undefined"
    no-route-dismiss
    @hide="closePanel"
  >
    <IssuePanel
      v-if="detailId"
      :id="detailId"
      :key="detailId"
      :names="names"
      :fields="fields"
      :title="detailTitle"
      :crumb="activeView || 'All open issues'"
      :position="position"
      :has-prev="index > 0"
      :has-next="index >= 0 && index < flat.length - 1"
      :find-ref="findRef"
      @close="detailOpen = false"
      @prev="step(-1)"
      @next="step(1)"
      @changed="load"
      @open="openId"
    />
  </q-dialog>

  <q-dialog v-model="newOpen">
    <NewIssueDialog :names="names" :fields="fields" :view="activeView" :filter="filterText" @close="newOpen = false" @created="onCreated" />
  </q-dialog>
</template>

<script setup lang="ts">
import { panelMode } from '@/panelsize';
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import {
  changeLabels, getFields, getIssues, getViews,
  type Bucket, type FieldConfig, type Issue, type ViewConfig,
} from '@/api/board';
import {
  facetLabel, facets, matches, namespaces, parseSelection, parseSort, sortIssues, valueLabel,
} from '@/facets';
import { loadColors } from '@/colors';
import { displayName, orderBuckets } from '@/fields';
import AppHeader from '@/components/AppHeader.vue';
import FilterSidebar from '@/components/FilterSidebar.vue';
import IssueBoard from '@/components/IssueBoard.vue';
import IssueTable from '@/components/IssueTable.vue';
import GbIcon from '@/components/GbIcon.vue';
import IssuePanel from '@/components/IssuePanel.vue';
import NewIssueDialog from '@/components/NewIssueDialog.vue';

const route = useRoute();
const router = useRouter();

const views = ref<Record<string, ViewConfig>>({});
const fields = ref<Record<string, FieldConfig>>({});
const configError = ref('');

// The view's filter expression and grouping go to the server; the facets,
// search and sort below refine its answer here, and live in the URL.
const filterText = ref('is:open');
const groupField = ref('');

const buckets = ref<Bucket[] | null>(null); // null until a load lands
const loading = ref(false);
const listError = ref('');

const viewNames = computed(() => Object.keys(views.value).sort());
const requestedView = computed(() => (typeof route.query.view === 'string' ? route.query.view : ''));
// The view in the URL, or the first one (alphabetical) when none is asked for.
const activeView = computed(() =>
  requestedView.value in views.value ? requestedView.value : (viewNames.value[0] ?? ''),
);
const unknownView = computed(() =>
  requestedView.value && viewNames.value.length && !(requestedView.value in views.value)
    ? requestedView.value
    : '',
);
const view = computed(() => views.value[activeView.value]);
const layout = computed(() => view.value?.layout ?? 'table');

type Patch = Record<string, string | string[] | undefined>;
/** Merge keys into the URL query; undefined removes one. */
const setQuery = (patch: Patch) => router.replace({ query: { ...route.query, ...patch } });

const sel = computed(() => parseSelection(route.query));
const q = computed(() => (typeof route.query.q === 'string' ? route.query.q : ''));
const sort = computed(() =>
  parseSort(typeof route.query.sort === 'string' ? route.query.sort : (view.value?.sort ?? '')),
);
const sortValue = computed(() => (typeof route.query.sort === 'string' ? route.query.sort : ''));

const queryChanged = computed(() => filterText.value !== (view.value?.filter ?? 'is:open'));
const modified = computed(
  () =>
    queryChanged.value ||
    groupField.value !== (view.value?.group ?? '') ||
    Object.keys(route.query).some((k) => k !== 'view' && k !== 'issue'),
);

const all = computed(() => (buckets.value ?? []).flatMap((b) => b.issues));
const names = computed(() => namespaces(all.value, fields.value));
const facetList = computed(() => facets(all.value, sel.value, '', fields.value));
const shown = computed(() =>
  orderBuckets(buckets.value ?? [], groupField.value, fields.value).map((b) => ({
    group: b.group,
    issues: sortIssues(
      b.issues.filter((i) => matches(i, sel.value, '')),
      sort.value,
      fields.value,
    ),
  })),
);
// A kanban shows every option of the field it's grouped by, empty or not,
// so a card can be dropped there.
const boardBuckets = computed(() => {
  const options = fields.value[groupField.value]?.options ?? [];
  const present = new Set(shown.value.map((b) => b.group));
  const empty = options.filter((o) => !present.has(o)).map((o) => ({ group: o, issues: [] as Issue[] }));
  return orderBuckets([...shown.value, ...empty], groupField.value, fields.value);
});

// A card dropped in another column takes its value: moved at once, saved
// in one label change (the old value out, the new in), the board reloaded.
const moveError = ref('');
async function onMove(issue: Issue, from: string, to: string) {
  const field = groupField.value;
  moveError.value = '';
  for (const b of buckets.value ?? []) b.issues = b.issues.filter((i) => i.id !== issue.id);
  const target = (buckets.value ?? []).find((b) => b.group === to);
  if (target) target.issues.push(issue);
  else buckets.value?.push({ group: to, issues: [issue] });
  try {
    await changeLabels(issue.id, to === '(none)' ? [] : [`${field}:${to}`], from === '(none)' ? [] : [`${field}:${from}`]);
  } catch (e: unknown) {
    moveError.value = message(e);
  }
  void load();
}

const total = computed(() => all.value.length);
const count = computed(() => shown.value.reduce((n, b) => n + b.issues.length, 0));

/** One removable chip per search and selected value. */
const chips = computed(() => {
  const out: { label: string; patch: Patch }[] = [];
  if (q.value) out.push({ label: `Search: ${q.value}`, patch: { q: undefined } });
  for (const [key, values] of Object.entries(sel.value)) {
    for (const v of values) {
      const rest = values.filter((x) => x !== v);
      out.push({
        label: `${facetLabel(key)}: ${valueLabel(key, v, fields.value)}`,
        patch: { [`f.${key}`]: rest.length ? rest : undefined },
      });
    }
  }
  return out;
});

function toggle(key: string, value: string) {
  const cur = sel.value[key] ?? [];
  const next = cur.includes(value) ? cur.filter((x) => x !== value) : [...cur, value];
  void setQuery({ [`f.${key}`]: next.length ? next : undefined });
}

// A header click sorts ascending, a second one descending.
function sortBy(key: string) {
  const s = sort.value;
  void setQuery({ sort: s?.key === key && !s.desc ? `-${key}` : key });
}

const sortChoices = computed(() =>
  [
    { key: 'title', label: 'Title' },
    { key: 'id', label: 'ID' },
    ...names.value.map((n) => ({ key: n, label: displayName(n) })),
  ].flatMap(({ key, label }) => [
    { value: key, label: `${label}, ascending` },
    { value: `-${key}`, label: `${label}, descending` },
  ]),
);

function onSortSelect(event: Event) {
  void setQuery({ sort: (event.target as HTMLSelectElement).value || undefined });
}

// The fields found on the issues, plus the view's own group key.
const groupChoices = computed(() => {
  const g = view.value?.group;
  return g && !names.value.includes(g) ? [...names.value, g] : names.value;
});

function message(e: unknown): string {
  return e instanceof Error ? e.message : String(e);
}

async function loadConfig() {
  configError.value = '';
  try {
    [views.value, fields.value] = await Promise.all([getViews(), getFields()]);
  } catch (e: unknown) {
    configError.value = message(e);
  }
}

// Back to the view's filter expression and grouping (the URL's facets stay:
// a shared link keeps its filters).
function resetView() {
  filterText.value = view.value ? view.value.filter : 'is:open';
  groupField.value = view.value?.group ?? '';
  void load();
}

function resetAll() {
  void router.replace({ query: requestedView.value ? { view: requestedView.value } : {} });
  resetView();
}

// Only the latest request may write: switching views quickly must not let a
// slow earlier response overwrite a newer one.
let latest = 0;
async function load() {
  const mine = ++latest;
  loading.value = true;
  listError.value = '';
  try {
    const [result] = await Promise.all([getIssues(filterText.value, groupField.value, q.value), loadColors()]);
    if (mine === latest) buckets.value = result;
  } catch (e: unknown) {
    if (mine === latest) {
      listError.value = message(e);
      buckets.value = null;
    }
  } finally {
    if (mine === latest) loading.value = false;
  }
}

watch(activeView, resetView);

// the search runs on the server (git-bug's full-text index): asked once
// typing pauses
let searchTimer: ReturnType<typeof setTimeout> | undefined;
watch(q, () => {
  clearTimeout(searchTimer);
  searchTimer = setTimeout(() => void load(), 300);
});

onMounted(async () => {
  await loadConfig();
  // No saved view to switch to (none configured, or the config failed):
  // the watch above never fires, so load the default filter here.
  if (!activeView.value) resetView();
});

// The issue panel: ?issue=<id> in the URL, so a link opens it; j / k move
// through the issues as the board shows them.
const detailOpen = ref(false);
const detailId = ref('');
const flat = computed(() => shown.value.flatMap((b) => b.issues));
const index = computed(() => flat.value.findIndex((i) => i.id === detailId.value));
const detailTitle = computed(() => all.value.find((i) => i.id === detailId.value)?.title ?? '');
const position = computed(() => {
  if (index.value < 0) return '';
  const b = shown.value.find((x) => x.issues.some((i) => i.id === detailId.value));
  if (groupField.value && b) {
    const at = b.issues.findIndex((i) => i.id === detailId.value);
    return `${b.group === '(none)' ? `no ${groupField.value}` : b.group}, ${at + 1} of ${b.issues.length}`;
  }
  return `${index.value + 1} of ${flat.value.length}`;
});

function openIssue(issue: Issue) {
  openId(issue.id);
}
function openId(id: string) {
  void setQuery({ issue: id });
}
function step(delta: number) {
  const next = flat.value[index.value + delta];
  if (next) openId(next.id);
}
function closePanel() {
  if (route.query.issue) void setQuery({ issue: undefined });
}
watch(
  () => route.query.issue,
  (id) => {
    if (typeof id === 'string' && id) {
      detailId.value = id;
      detailOpen.value = true;
    } else detailOpen.value = false;
  },
  { immediate: true },
);

// #n in a text: the local issue on the same forge as the open one
function findRef(n: string): string | undefined {
  const base = all.value.find((i) => i.id === detailId.value)?.forges?.[0]?.url.replace(/\d+$/, '');
  if (!base) return undefined;
  return all.value.find((i) => i.forges?.some((f) => f.url === base + n))?.id;
}

// New issue: the button, or c
const newOpen = ref(false);
function onCreated(id: string, another: boolean) {
  void load();
  if (!another) {
    newOpen.value = false;
    openId(id);
  }
}
function onKey(e: KeyboardEvent) {
  if (e.key !== 'c' || e.ctrlKey || e.metaKey || e.altKey || detailOpen.value || newOpen.value) return;
  if (e.target instanceof Element && e.target.closest('input, textarea, select, [contenteditable]')) return;
  e.preventDefault();
  newOpen.value = true;
}
onMounted(() => window.addEventListener('keydown', onKey));
onBeforeUnmount(() => window.removeEventListener('keydown', onKey));
</script>

<style scoped>
/* The masonry-walls database layout: a 17rem filter sidebar, a fluid main
   column; the sidebar goes on top on narrower screens. */
.page {
  display: grid;
  grid-template-columns: 17rem minmax(0, 1fr);
  gap: var(--space-4);
  padding: 0 var(--gutter);
}
.main { min-width: 0; padding: var(--space-4) 0 var(--space-5); }

.notice { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-2); margin-bottom: var(--space-3); }

.page-head { margin-bottom: var(--space-4); }
.page-title { margin: 0 0 var(--space-1); overflow-wrap: anywhere; }
.page-meta { margin: 0; font-size: var(--fs-sm); color: var(--fg-muted); }

.toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  gap: var(--space-3);
  margin-bottom: var(--space-3);
}
.toolbar-field { flex: 0 1 13rem; min-width: 0; }
.toolbar-search { flex: 1 1 20rem; max-width: 32rem; }
.search-hint { font-weight: var(--w-normal); color: var(--fg-muted); }
.toolbar-new { display: flex; align-items: center; gap: .5rem; margin-left: auto; align-self: flex-end; }
.toolbar-new .epfl-btn { display: inline-flex; align-items: center; gap: .375rem; }

.status {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2) 0.75rem;
  margin-bottom: var(--space-3);
  padding-bottom: var(--space-3);
  font-size: var(--fs-sm);
  border-bottom: var(--border-w) solid var(--border-subtle);
}
.status-sep { color: var(--epfl-gray-300); }
.chip {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  padding: 0.125rem 0.125rem 0.125rem var(--space-2);
  border: var(--border-w) solid var(--epfl-gray-300);
  border-radius: var(--radius);
}
.chip button {
  padding: 0 0.375rem;
  font: inherit;
  line-height: 1.2;
  color: var(--fg-muted);
  background: none;
  border: 0;
  cursor: pointer;
}
.chip button:hover { color: var(--primary-hover); }
.chip button:focus-visible { outline: var(--focus-w) solid var(--primary-hover); }

.state { margin: 0; padding: var(--space-5) var(--space-3); text-align: center; color: var(--fg-muted); }
.state-empty { border: var(--border-w) solid var(--border); }
.state-empty p { margin: 0; }
.state-title { color: var(--fg); font-weight: var(--w-bold); margin-bottom: var(--space-1); }

@media (max-width: 63.99rem) {
  .page { grid-template-columns: minmax(0, 1fr); gap: 0; }
}
@media (max-width: 47.99rem) {
  .page { padding: 0 var(--space-3); }
  .toolbar-field,
  .toolbar-search { flex-basis: 100%; max-width: none; }
}
</style>
