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
          <label class="epfl-label" for="search">Search</label>
          <input
            id="search"
            class="epfl-input"
            type="search"
            placeholder="Title or ID"
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
        <IssueBoard
          v-else-if="layout === 'board'"
          :buckets="shown"
          :names="names"
          :fields="fields"
          :group="groupField"
          @open="openIssue"
        />
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

  <q-dialog v-model="detailOpen" position="right" full-height aria-labelledby="issue-detail-title">
    <IssueDetail
      v-if="detailIssue"
      :key="detailIssue.id"
      :issue="detailIssue"
      :names="names"
      :fields="fields"
      @updated="onUpdated"
      @close="detailOpen = false"
    />
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import {
  getFields, getIssues, getViews,
  type Bucket, type FieldConfig, type Issue, type ViewConfig,
} from '@/api/board';
import {
  facetLabel, facets, matches, namespaces, parseSelection, parseSort, sortIssues, valueLabel,
} from '@/facets';
import { displayName, orderBuckets } from '@/fields';
import AppHeader from '@/components/AppHeader.vue';
import FilterSidebar from '@/components/FilterSidebar.vue';
import IssueBoard from '@/components/IssueBoard.vue';
import IssueTable from '@/components/IssueTable.vue';
import IssueDetail from '@/components/IssueDetail.vue';

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
    Object.keys(route.query).some((k) => k !== 'view'),
);

const all = computed(() => (buckets.value ?? []).flatMap((b) => b.issues));
const names = computed(() => namespaces(all.value, fields.value));
const facetList = computed(() => facets(all.value, sel.value, q.value, fields.value));
const shown = computed(() =>
  orderBuckets(buckets.value ?? [], groupField.value, fields.value).map((b) => ({
    group: b.group,
    issues: sortIssues(
      b.issues.filter((i) => matches(i, sel.value, q.value)),
      sort.value,
      fields.value,
    ),
  })),
);
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
    const result = await getIssues(filterText.value, groupField.value);
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

onMounted(async () => {
  await loadConfig();
  // No saved view to switch to (none configured, or the config failed):
  // the watch above never fires, so load the default filter here.
  if (!activeView.value) resetView();
});

const detailOpen = ref(false);
const detailIssue = ref<Issue | null>(null);

function openIssue(issue: Issue) {
  detailIssue.value = issue;
  detailOpen.value = true;
}

// A changed group field (say status, on a board) moves the issue to another
// bucket, so re-fetch rather than patch the list in place.
function onUpdated(issue: Issue) {
  detailIssue.value = issue;
  void load();
}
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
.toolbar-search { flex: 1 1 16rem; max-width: 24rem; }

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
