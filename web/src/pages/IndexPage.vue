<template>
  <AppHeader :views="viewNames" :active="activeView" />

  <div class="page" :class="{ 'page-filters-hidden': filtersHidden }">
    <!-- the filters folded to a rail (wide screens): its badge counts the
         filters still applied -->
    <button
      v-if="filtersHidden"
      type="button"
      class="filters-rail"
      :aria-label="activeFilters ? `Show filters, ${activeFilters} applied` : 'Show filters'"
      title="Show filters"
      @click="filtersHidden = false"
    >
      <GbIcon name="filter" small />
      <span class="filters-rail-label">Filters</span>
      <span v-if="activeFilters" class="filters-rail-badge">{{ activeFilters }}</span>
    </button>
    <FilterSidebar
      v-show="!filtersHidden"
      @hide="filtersHidden = true"
      v-model:query="filterText"
      :facets="facetList"
      :sel="sel"
      :fields="fields"
      :modified="modified"
      :query-changed="queryChanged"
      @set="setFacet"
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
          {{ layout === 'board' ? 'Board' : layout === 'roadmap' ? 'Roadmap' : 'Table' }}
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
          <button type="button" class="epfl-btn epfl-btn-primary" title="New issue (keyboard: c)" aria-keyshortcuts="c" @click="newOpen = true"><GbIcon name="plus" small /> New issue</button>
        </div>
      </div>

      <!-- What differs from the saved view, as GitHub Projects shows it:
           saved into .git-board.toml for everyone, or dropped. -->
      <div v-if="viewDirty" class="viewbar" role="region" aria-label="Unsaved changes to the view">
        <span class="viewbar-what">
          <b>{{ activeView ? `Unsaved changes to ${activeView}` : 'Not a saved view' }}</b>
          <code class="viewbar-filter">{{ draft.filter }}</code>
          <span v-if="draft.group" class="viewbar-meta">grouped by {{ draft.group }}</span>
          <span v-if="draft.sort" class="viewbar-meta">sorted by {{ draft.sort }}</span>
          <span v-if="!sameColumns(draft.columns, view?.columns)" class="viewbar-meta">{{ draft.columns ? 'columns of its own' : 'default columns' }}</span>
        </span>
        <form v-if="naming" class="viewbar-name" @submit.prevent="saveAs">
          <input id="viewname" aria-label="New view name" ref="nameInput" v-model="newName" class="epfl-input" placeholder="new-view-name" pattern="[A-Za-z0-9_\-]+" title="Letters, digits, - and _" required @keydown.esc.stop="naming = false" />
          <button type="submit" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="savingView">Save</button>
          <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" @click="naming = false">Cancel</button>
        </form>
        <template v-else>
          <button v-if="activeView" type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="savingView" @click="save(activeView)">Save view</button>
          <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" @click="startNaming">Save as new view…</button>
          <button type="button" class="viewbar-discard" @click="resetAll">Discard</button>
        </template>
        <p v-if="draftSkipped.length" class="viewbar-note">Not saved: {{ draftSkipped.join(', ') }} (“(none)” together with values has no single filter).</p>
        <p v-if="viewError" class="viewbar-note viewbar-error" role="alert">Not saved: {{ viewError }}</p>
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
        <RoadmapView
          v-else-if="layout === 'roadmap'"
          :buckets="shown.filter((b) => b.issues.length)"
          :group="groupField"
          :fields="fields"
          @open="openIssue"
        />
        <IssueTable
          v-else
          :buckets="shown.filter((b) => b.issues.length)"
          :names="names"
          :fields="fields"
          :group="groupField"
          :sort="sort"
          :columns="columns"
          @open="openIssue"
          @sort="sortBy"
          @changed="load"
          @update:columns="(c) => (columns = c)"
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
    no-focus
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
  changeLabels, getFields, getIssues, getViews, saveView,
  type Bucket, type FieldConfig, type Issue, type ViewConfig,
} from '@/api/board';
import {
  facetLabel, facets, matches, namespaces, parseSelection, parseSort, sortIssues, valueLabel,
} from '@/facets';
import { loadColors } from '@/colors';
import { composeFilter } from '@/viewfilter';
import { displayName, orderBuckets } from '@/fields';
import AppHeader from '@/components/AppHeader.vue';
import FilterSidebar from '@/components/FilterSidebar.vue';
import IssueBoard from '@/components/IssueBoard.vue';
import IssueTable from '@/components/IssueTable.vue';
import RoadmapView from '@/components/RoadmapView.vue';
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
const columns = ref<string[] | undefined>(); // the table's, the view's until changed (tablecols.ts)

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

// the filters panel hidden to a rail, kept in this browser
const filtersHidden = ref(readFlag('gb-filters-hidden'));
watch(filtersHidden, (v) => writeFlag('gb-filters-hidden', v));
const activeFilters = computed(() => Object.values(sel.value).flat().length + Number(queryChanged.value));
function readFlag(key: string): boolean {
  try {
    return localStorage.getItem(key) === '1';
  } catch {
    return false;
  }
}
function writeFlag(key: string, v: boolean) {
  try {
    localStorage.setItem(key, v ? '1' : '0');
  } catch {
    // private window: lasts until reload
  }
}
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

function setFacet(key: string, values: string[]) {
  void setQuery({ [`f.${key}`]: values.length ? values : undefined });
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

// The view as the page shows it now, and whether that differs from the
// saved one: the bar offers to save it into .git-board.toml.
const composed = computed(() =>
  composeFilter(filterText.value, sel.value, q.value, Object.fromEntries(facetList.value.map((f) => [f.key, f.options.map(([v]) => v)]))),
);
const draftSkipped = computed(() => composed.value.skipped);
const draft = computed<ViewConfig>(() => ({
  layout: layout.value,
  filter: composed.value.filter,
  group: groupField.value,
  sort: sortValue.value || (view.value?.sort ?? ''),
  columns: columns.value,
}));
const sameColumns = (a?: string[], b?: string[]) => (a ?? []).join(',') === (b ?? []).join(',');
const viewDirty = computed(() => {
  const v = view.value;
  const d = draft.value;
  if (draftSkipped.value.length) return true; // the page differs, even what can't be saved
  if (!v) return d.filter !== 'is:open' || d.group !== '' || d.sort !== '' || !!d.columns;
  return d.filter !== v.filter || d.group !== (v.group ?? '') || d.sort !== (v.sort ?? '') || !sameColumns(d.columns, v.columns);
});
const naming = ref(false);
const newName = ref('');
const nameInput = ref<HTMLInputElement | null>(null);
const savingView = ref(false);
const viewError = ref('');

function startNaming() {
  newName.value = activeView.value ? `${activeView.value}-copy` : '';
  naming.value = true;
  void Promise.resolve().then(() => nameInput.value?.select());
}

async function save(name: string) {
  savingView.value = true;
  viewError.value = '';
  try {
    views.value = await saveView(name, draft.value);
    naming.value = false;
    // the saved view, without the URL's search, facets and sort it now holds
    await router.replace({ query: { view: name } });
    resetView();
  } catch (e) {
    viewError.value = message(e);
  } finally {
    savingView.value = false;
  }
}

const saveAs = () => save(newName.value.trim());

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
  columns.value = view.value?.columns;
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
.viewbar { display: flex; flex-wrap: wrap; align-items: center; gap: .5rem .75rem; margin: 0 0 1rem; padding: .625rem .875rem; border: 1px solid var(--warning-border); border-left: 4px solid var(--warning); background: var(--warning-bg); }
.viewbar-what { display: flex; flex-wrap: wrap; align-items: baseline; gap: .25rem .625rem; flex: 1 1 22rem; min-width: 0; }
.viewbar-filter { padding: .05em .4em; background: var(--bg); border: 1px solid var(--border-subtle); font-size: .8125rem; overflow-wrap: anywhere; }
.viewbar-meta { font-size: .875rem; color: var(--fg-muted); }
.viewbar-name { display: flex; align-items: center; gap: .5rem; }
.viewbar-name .epfl-input { width: 14rem; min-height: 2rem; padding: .25rem .5rem; }
.viewbar-discard { padding: 0; border: 0; background: none; color: var(--fg); text-decoration: underline; cursor: pointer; font: inherit; }
.viewbar-note { flex-basis: 100%; margin: 0; font-size: .8125rem; color: var(--fg-muted); }
.viewbar-error { color: var(--danger-fg); }
/* The masonry-walls database layout: a 17rem filter sidebar, a fluid main
   column; the sidebar goes on top on narrower screens. */
.page {
  display: grid;
  grid-template-columns: 17rem minmax(0, 1fr);
  gap: var(--space-4);
  padding: 0 var(--gutter);
}
.main { min-width: 0; padding: var(--space-4) 0 var(--space-5); }
.page-filters-hidden { grid-template-columns: 2.75rem minmax(0, 1fr); }
.filters-rail { position: sticky; top: 0; align-self: start; display: flex; flex-direction: column; align-items: center; gap: var(--space-2); width: 2.75rem; min-height: 12rem; margin-top: var(--space-4); padding: var(--space-3) 0; border: var(--border-w) solid var(--border); background: var(--bg-subtle); color: var(--fg); cursor: pointer; font: inherit; }
.filters-rail:hover, .filters-rail:focus-visible { border-color: var(--fg); }
.filters-rail-label { writing-mode: vertical-rl; transform: rotate(180deg); font-weight: var(--w-bold); font-size: var(--fs-sm); letter-spacing: .04em; }
.filters-rail-badge { min-width: 1.25rem; padding: 0 .25rem; background: var(--primary); color: var(--on-primary); font-size: .75rem; font-weight: var(--w-bold); text-align: center; border-radius: var(--radius); }

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
.status-sep { color: var(--fg-disabled); }
.chip {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  padding: 0.125rem 0.125rem 0.125rem var(--space-2);
  border: var(--border-w) solid var(--border);
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
  .page, .page-filters-hidden { grid-template-columns: minmax(0, 1fr); gap: 0; }
  /* narrow: the filters fold on top of the issues by themselves */
  .filters-rail { display: none; }
  .page-filters-hidden > .filters { display: block !important; }
}
@media (max-width: 47.99rem) {
  .page { padding: 0 var(--space-3); }
  .toolbar-field,
  .toolbar-search { flex-basis: 100%; max-width: none; }
}
</style>
