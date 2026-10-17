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
      :facets="facetList"
      :sel="sel"
      :fields="fields"
      :modified="modified"
      @set="setFacet"
      @reset="resetAll"
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
          <template v-if="sprintView"> · <button type="button" class="page-report" @click="reportOpen = true">Sprint report</button></template>
        </p>
        <SprintReportDialog v-if="sprintView" v-model="reportOpen" />
      </div>

      <!-- toolbar, unsaved-view bar and count stick under the header
           (wide screens); their height feeds --gb-tools-h, which the
           table's column titles and the board's height use -->
      <div ref="tools" class="tools">
      <div class="toolbar">
        <div class="toolbar-field toolbar-search">
          <label class="epfl-label" for="filter">Filter <span class="search-hint">· words in titles, descriptions, comments; #number; field:value</span></label>
          <input
            id="filter"
            class="epfl-input"
            type="search"
            spellcheck="false"
            placeholder="Words, #2528, status:ready, assignee:@me, is:closed…"
            autocomplete="off"
            list="filter-suggest"
            :value="filter"
            @input="setFilter(($event.target as HTMLInputElement).value)"
          />
          <datalist id="filter-suggest"><option v-for="s in suggestions" :key="s" :value="s" /></datalist>
        </div>
        <div class="toolbar-field">
          <label class="epfl-label" for="group">Group by</label>
          <select id="group" v-model="groupField" class="epfl-select" @change="regroup">
            <option value="">No grouping</option>
            <option v-for="name in groupChoices" :key="name" :value="name">{{ displayName(name) }}</option>
          </select>
        </div>
        <div v-if="layout === 'board' && groupField" class="toolbar-field">
          <span class="epfl-label">Lanes <span class="search-hint">· which columns this board shows</span></span>
          <LaneChooser v-model:lanes="lanes" v-model:limits="limits" :values="laneValues" :field="groupField" />
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
          <code v-if="draft.filter" class="viewbar-filter">{{ draft.filter }}</code>
          <span v-else class="viewbar-meta">every issue</span>
          <span v-if="draft.group" class="viewbar-meta">grouped by {{ draft.group }}</span>
          <span v-if="draft.sort" class="viewbar-meta">sorted by {{ draft.sort }}</span>
          <span v-if="!sameColumns(draft.columns, view?.columns)" class="viewbar-meta">{{ draft.columns ? 'columns of its own' : 'default columns' }}</span>
          <span v-if="lanesChanged" class="viewbar-meta">{{ draft.lanes ? `lanes: ${draft.lanes.join(', ')}` : 'every lane' }}{{ draft.limits ? ' · limits' : '' }}</span>
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
        <p v-if="viewError" class="viewbar-note viewbar-error" role="alert">Not saved: {{ viewError }}</p>
      </div>

      <div v-if="buckets" class="status" role="status">
        <span><b>{{ count }}</b>{{ count === total ? '' : ` of ${total}` }} {{ total === 1 ? 'issue' : 'issues' }}</span>
        <template v-if="changed.size || onlyChanged">
          <span class="status-sep" aria-hidden="true">|</span>
          <button type="button" class="changed-toggle" :aria-pressed="onlyChanged" @click="onlyChanged = !onlyChanged">
            <span class="gb-changed-dot" aria-hidden="true" />Changed since your last visit ({{ changed.size }})
          </button>
          <button v-if="changed.size" type="button" class="changed-seen" @click="markAllSeen">Mark all seen</button>
        </template>
      </div>
      </div>

      <div :aria-busy="loading">
        <div v-if="listError" class="epfl-alert epfl-alert-danger" role="alert">
          <b>Could not load issues.</b> {{ listError }}
        </div>
        <p v-else-if="!buckets" class="state">Loading issues…</p>
        <div v-else-if="total === 0" class="state state-empty">
          <p class="state-title">No issues match this filter.</p>
          <p v-if="modified"><button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" @click="resetAll">Back to {{ activeView || 'the open issues' }}</button></p>
          <p v-else>Try a broader one, for example <code>is:open</code>.</p>
        </div>
        <div v-else-if="layout === 'board'">
          <p v-if="moveError" class="epfl-alert epfl-alert-danger notice" role="alert"><b>Not moved.</b> {{ moveError }}</p>
          <p v-if="laneView.hidden || (showAllLanes && lanes)" class="lanes-note">
            <template v-if="showAllLanes">Every {{ groupField }} shown, beyond this board's lanes.
              <button type="button" class="lanes-note-btn" @click="showAllLanes = false">Back to its lanes</button></template>
            <template v-else>{{ laneView.hidden }} issue{{ laneView.hidden === 1 ? '' : 's' }} in other {{ groupField }} values {{ laneView.hidden === 1 ? 'is' : 'are' }} not on this board.
              <button type="button" class="lanes-note-btn" @click="showAllLanes = true">Show every lane</button></template>
          </p>
          <IssueBoard
            :buckets="laneView.shown"
            :limits="limits"
            :changed="changed"
            :names="names"
            :fields="fields"
            :group="groupField"
            @open="openIssue"
            @move="onMove"
            @move-many="onMoveMany"
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
          :changed="changed"
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
    <NewIssueDialog :names="names" :fields="fields" :view="activeView" :prefill="prefill" @close="newOpen = false" @created="onCreated" />
  </q-dialog>
</template>

<script setup lang="ts">
import SprintReportDialog from '@/components/SprintReportDialog.vue';
import { debounced, onLive } from '@/live';
import { panelMode } from '@/panelsize';
import { filtersHidden, newIssueRequested, recordRecent } from '@/commands';
import { changedSince, loadVisit, saveVisit, visit } from '@/lastvisit';
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import {
  changeLabels, filterValues, getFacets, getFields, getIssues, getViews, saveView,
  type Bucket, type FieldConfig, type Issue, type ViewConfig,
} from '@/api/board';
import {
  NONE, PLAIN, STATE, namespaces, parseSelection, parseSort, sortIssues, type Facet,
} from '@/facets';
import { loadColors } from '@/colors';
import { composeFilter, suggest, withFacet } from '@/viewfilter';
import { displayName, orderBuckets } from '@/fields';
import AppHeader from '@/components/AppHeader.vue';
import FilterSidebar from '@/components/FilterSidebar.vue';
import IssueBoard from '@/components/IssueBoard.vue';
import LaneChooser from '@/components/LaneChooser.vue';
import { boardLanes } from '@/lanes';
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

// The filter (the Filter box, ?filter= in the URL once it differs from the
// view's) and the grouping go to the server, which matches and counts the
// facets; the sort below orders its answer here, and lives in the URL.
const groupField = ref('');
const columns = ref<string[] | undefined>(); // the table's, the view's until changed (tablecols.ts)
const lanes = ref<string[] | undefined>(); // the board's, the view's until changed (lanes.ts)
const limits = ref<Record<string, number> | undefined>();
const showAllLanes = ref(false);

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

const viewFilter = computed(() => view.value?.filter ?? 'is:open');
const filter = computed(() => (typeof route.query.filter === 'string' ? route.query.filter : viewFilter.value));
function setFilter(f: string) {
  return setQuery({ filter: f === viewFilter.value ? undefined : f });
}
const sort = computed(() =>
  parseSort(typeof route.query.sort === 'string' ? route.query.sort : (view.value?.sort ?? '')),
);
const sortValue = computed(() => (typeof route.query.sort === 'string' ? route.query.sort : ''));

// status: → status:ready, …
const values = ref<Record<string, string[]>>({});
void filterValues().then((v) => (values.value = v));
const suggestions = computed(() => suggest(filter.value, values.value));

// the facets, the server's for the filter: the ticks are the values it
// lets through, a tick writes a new filter
const facetList = ref<Facet[]>([]);
const sel = computed(() => Object.fromEntries(facetList.value.map((f) => [f.key, f.selected])));
function setFacet(key: string, values: string[]) {
  const f = facetList.value.find((x) => x.key === key);
  if (f) void setFilter(withFacet(f, values)).then(load);
}

// the filters panel hidden to a rail (commands.ts: the palette toggles it
// too): its badge counts the facets the filter narrows
const activeFilters = computed(() => facetList.value.filter((f) => f.selected.length).length);
const modified = computed(
  () =>
    filter.value !== viewFilter.value ||
    groupField.value !== (view.value?.group ?? '') ||
    Object.keys(route.query).some((k) => k !== 'view' && k !== 'issue'),
);

const all = computed(() => (buckets.value ?? []).flatMap((b) => b.issues));

// Since your last visit (lastvisit.ts): each view remembers the visit
// before this one; the issues changed since get a dot, and can be shown
// alone.
const visitPrev = ref<number | null>(null);
const onlyChanged = ref(false);
const changed = computed(() => changedSince(all.value, visitPrev.value));
watch(
  activeView,
  (v) => {
    const rec = visit(loadVisit(v), Date.now());
    saveVisit(v, rec);
    visitPrev.value = rec.prev;
    onlyChanged.value = false;
  },
  { immediate: true },
);
function markAllSeen() {
  const now = Date.now();
  saveVisit(activeView.value, { prev: now, current: now });
  visitPrev.value = now;
  onlyChanged.value = false;
}

// the sticky tools' height, for what sticks under them
const tools = ref<HTMLElement | null>(null);
let toolsObserver: ResizeObserver | undefined;
onMounted(() => {
  toolsObserver = new ResizeObserver(([e]) => document.documentElement.style.setProperty('--gb-tools-h', `${Math.ceil(e!.borderBoxSize[0]?.blockSize ?? e!.contentRect.height)}px`));
  if (tools.value) toolsObserver.observe(tools.value);
});
onBeforeUnmount(() => {
  toolsObserver?.disconnect();
  document.documentElement.style.removeProperty('--gb-tools-h');
});
const names = computed(() => namespaces(all.value, fields.value));
const shown = computed(() =>
  orderBuckets(buckets.value ?? [], groupField.value, fields.value).map((b) => ({
    group: b.group,
    issues: sortIssues(
      b.issues.filter((i) => !onlyChanged.value || changed.value.has(i.id)),
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

// lanes are values of the field grouped by: another field, every lane
function regroup() {
  lanes.value = undefined;
  limits.value = undefined;
  showAllLanes.value = false;
  void load();
}

// the board's lanes (a board per role), the other values' issues counted
const laneView = computed(() => (showAllLanes.value ? { shown: boardBuckets.value, hidden: 0 } : boardLanes(boardBuckets.value, lanes.value)));
const laneValues = computed(() => {
  const vs = boardBuckets.value.map((b) => b.group);
  return vs.includes('(none)') ? vs : [...vs, '(none)'];
});

// Picked cards dropped in another lane, or a lane's "Move all to…": one
// label change each, in turn; the first refusal stops and says which.
async function onMoveMany(moves: { issue: Issue; from: string }[], to: string) {
  const field = groupField.value;
  moveError.value = '';
  for (const b of buckets.value ?? []) b.issues = b.issues.filter((i) => !moves.some((m) => m.issue.id === i.id));
  const target = (buckets.value ?? []).find((b) => b.group === to);
  if (target) target.issues.push(...moves.map((m) => m.issue));
  else buckets.value?.push({ group: to, issues: moves.map((m) => m.issue) });
  for (const [n, { issue, from }] of moves.entries()) {
    try {
      await changeLabels(issue.id, to === '(none)' ? [] : [`${field}:${to}`], from === '(none)' ? [] : [`${field}:${from}`]);
    } catch (e: unknown) {
      // after the alert's "Not moved."
      moveError.value = `“${issue.shown_title}”: ${message(e)}. ${n} of ${moves.length} moved before it, the rest not tried.`;
      break;
    }
  }
  void load();
}

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

// a sprint's board: its filter picks today's sprint
const sprintView = computed(() => facetList.value.some((f) => f.selected.length === 1 && f.selected[0] === fields.value[f.key]?.current));

// New issue starts with the fields the filter picks one value of
const prefill = computed(() =>
  Object.fromEntries(
    facetList.value
      .filter((f) => f.key !== STATE && f.key !== PLAIN && f.selected.length === 1 && f.selected[0] !== NONE)
      .map((f) => [f.key, f.selected[0]!]),
  ),
);

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
const draft = computed<ViewConfig>(() => ({
  layout: layout.value,
  filter: filter.value,
  group: groupField.value,
  sort: sortValue.value || (view.value?.sort ?? ''),
  columns: columns.value,
  lanes: lanes.value,
  limits: limits.value,
}));
const sameColumns = (a?: string[], b?: string[]) => (a ?? []).join(',') === (b ?? []).join(',');
const sameLimits = (a?: Record<string, number>, b?: Record<string, number>) =>
  JSON.stringify(Object.entries(a ?? {}).sort()) === JSON.stringify(Object.entries(b ?? {}).sort());
const lanesChanged = computed(() => !sameColumns(lanes.value, view.value?.lanes) || !sameLimits(limits.value, view.value?.limits));
const viewDirty = computed(() => {
  const v = view.value;
  const d = draft.value;
  if (!v) return d.filter !== 'is:open' || d.group !== '' || d.sort !== '' || !!d.columns || !!d.lanes || !!d.limits;
  return d.filter !== v.filter || d.group !== (v.group ?? '') || d.sort !== (v.sort ?? '') || !sameColumns(d.columns, v.columns) || lanesChanged.value;
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
    // the saved view, without the URL's filter and sort it now holds
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

// Back to the view's grouping, columns and lanes (the URL's filter stays:
// a shared link keeps it).
function resetView() {
  groupField.value = view.value?.group ?? '';
  columns.value = view.value?.columns;
  lanes.value = view.value?.lanes;
  limits.value = view.value?.limits;
  showAllLanes.value = false;
  void load();
}

async function resetAll() {
  await router.replace({ query: requestedView.value ? { view: requestedView.value } : {} });
  resetView();
}

// Only the latest request may write: switching views quickly must not let a
// slow earlier response overwrite a newer one.
let latest = 0;
let loaded = ''; // the filter last asked for
async function load() {
  const mine = ++latest;
  loading.value = true;
  listError.value = '';
  loaded = filter.value;
  try {
    const [result, fs] = await Promise.all([getIssues(filter.value, groupField.value), getFacets(filter.value), loadColors()]);
    if (mine === latest) {
      buckets.value = result;
      facetList.value = fs;
    }
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

// the filter runs on the server (git-bug's full-text index for words):
// asked once typing pauses, unless a tick or the view asked already
let filterTimer: ReturnType<typeof setTimeout> | undefined;
watch(filter, () => {
  clearTimeout(filterTimer);
  filterTimer = setTimeout(() => filter.value !== loaded && void load(), 300);
});

// An older link (?q=words&f.status=ready): its filter, in the box.
function upgradeLink() {
  const ticks = parseSelection(route.query);
  const words = typeof route.query.q === 'string' ? route.query.q : '';
  if (!words && !Object.keys(ticks).length) return;
  const rest = Object.fromEntries(Object.entries(route.query).filter(([k]) => k !== 'q' && !k.startsWith('f.')));
  return router.replace({ query: { ...rest, filter: composeFilter(filter.value, ticks, words).filter } });
}

onMounted(async () => {
  await loadConfig();
  await upgradeLink();
  watch(() => route.query, upgradeLink);
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
      const i = all.value.find((x) => x.id === id);
      if (i) recordRecent({ id, title: i.shown_title, number: i.forges?.[0]?.number });
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

// New issue: the button, c, or the palette
const newOpen = ref(false);
watch(
  newIssueRequested,
  (v) => {
    if (!v) return;
    newIssueRequested.value = false;
    newOpen.value = true;
  },
  { immediate: true },
);
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

// live updates (live.ts): another page or a sync changed the issues or the
// configuration; reload once a burst is over (the board keeps its scroll)
const reportOpen = ref(false);
const reloadLive = debounced(() => void load());
const reloadConfig = debounced(async () => {
  await loadConfig();
  void load();
});
const offLive = onLive((e) => (e.kind === 'config' ? reloadConfig() : e.kind !== 'settings' && reloadLive()));
onBeforeUnmount(offLive);
</script>

<style scoped>
/* the tools stick under the header on wider screens */
@media (min-width: 48rem) {
  .tools { position: sticky; top: var(--header-h); z-index: 20; margin: 0 calc(-1 * var(--space-2)); padding: var(--space-3) var(--space-2) 0; background: var(--bg); box-shadow: 0 1px 0 var(--border-subtle); }
}
@media (min-width: 64rem) {
  .page > .filters { position: sticky; top: var(--header-h); align-self: start; max-height: calc(100vh - var(--header-h)); overflow-y: auto; }
}
.changed-toggle { display: inline-flex; align-items: center; gap: .375rem; padding: .125rem .5rem; border: 1px solid var(--border); background: var(--bg); color: var(--fg); font: inherit; font-size: var(--fs-sm); cursor: pointer; }
.changed-toggle[aria-pressed="true"] { border-color: var(--fg); background: var(--fg); color: var(--bg); }
.changed-seen { padding: 0; border: 0; background: none; color: var(--fg-muted); font: inherit; font-size: var(--fs-sm); text-decoration: underline; cursor: pointer; }
.viewbar { display: flex; flex-wrap: wrap; align-items: center; gap: .5rem .75rem; margin: 0 0 1rem; padding: .625rem .875rem; border: 1px solid var(--warning-border); border-left: 4px solid var(--warning); background: var(--warning-bg); }
.viewbar-what { display: flex; flex-wrap: wrap; align-items: baseline; gap: .25rem .625rem; flex: 1 1 22rem; min-width: 0; }
.viewbar-filter { padding: .05em .4em; background: var(--bg); border: 1px solid var(--border-subtle); font-size: .8125rem; overflow-wrap: anywhere; }
.viewbar-meta { font-size: .875rem; color: var(--fg-muted); }
.viewbar-name { display: flex; align-items: center; gap: .5rem; }
.viewbar-name .epfl-input { width: 14rem; min-height: 2rem; padding: .25rem .5rem; }
.viewbar-discard { padding: 0; border: 0; background: none; color: var(--fg); text-decoration: underline; cursor: pointer; font: inherit; }
.lanes-note { margin: 0 0 var(--space-3); color: var(--fg-muted); font-size: var(--fs-sm); }
.lanes-note-btn { padding: 0; border: 0; background: none; color: var(--fg); font: inherit; text-decoration: underline; cursor: pointer; }
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
.filters-rail { position: sticky; top: var(--header-h); align-self: start; display: flex; flex-direction: column; align-items: center; gap: var(--space-2); width: 2.75rem; min-height: 12rem; margin-top: var(--space-4); padding: var(--space-3) 0; border: var(--border-w) solid var(--border); background: var(--bg-subtle); color: var(--fg); cursor: pointer; font: inherit; }
.filters-rail:hover, .filters-rail:focus-visible { border-color: var(--fg); }
.filters-rail-label { writing-mode: vertical-rl; transform: rotate(180deg); font-weight: var(--w-bold); font-size: var(--fs-sm); letter-spacing: .04em; }
.filters-rail-badge { min-width: 1.25rem; padding: 0 .25rem; background: var(--primary); color: var(--on-primary); font-size: .75rem; font-weight: var(--w-bold); text-align: center; border-radius: var(--radius); }

.notice { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-2); margin-bottom: var(--space-3); }

.page-head { margin-bottom: var(--space-4); }
.page-title { margin: 0 0 var(--space-1); overflow-wrap: anywhere; }
.page-meta { margin: 0; font-size: var(--fs-sm); color: var(--fg-muted); }
.page-report { padding: 0; border: 0; background: none; color: var(--fg); font: inherit; text-decoration: underline; cursor: pointer; }

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
