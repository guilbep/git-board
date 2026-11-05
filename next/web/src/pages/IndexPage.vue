<template>
  <AppHeader :views="viewNames" />

  <div class="page" :class="{ 'page-filters-hidden': filtersHidden }">
    <!-- the filters folded to a rail (wide screens): its badge counts the
         filters still applied -->
    <div v-if="filtersHidden" class="filters-rail">
      <button
        type="button"
        class="filters-rail-show"
        :aria-label="activeFilters ? `Show filters, ${activeFilters} applied` : 'Show filters'"
        title="Show filters"
        @click="filtersHidden = false"
      >
        <GbIcon name="filter" small />
        <span class="filters-rail-label">Filters</span>
        <span v-if="activeFilters" class="filters-rail-badge">{{ activeFilters }}</span>
      </button>
      <!-- the panel's ?, kept when it folds -->
      <FilterHelp align="left" @try="tryWord" />
    </div>
    <FilterSidebar
      v-show="!filtersHidden"
      @mouseenter="inFilters = true"
      @mouseleave="inFilters = false"
      @focusin="inFilters = true"
      @focusout="inFilters = false"
      @hide="filtersHidden = true"
      :facets="facetList"
      :sel="sel"
      :fields="fields"
      :modified="modified"
      @set="setFacet"
      @reset="resetAll"
      @try="tryWord"
    />

    <main class="main">
      <div v-if="configError" class="epfl-alert epfl-alert-danger notice" role="alert">
        <b>Could not load the board configuration.</b> {{ configError }}
        <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" @click="loadConfig">Try again</button>
      </div>
      <div v-if="unknownView" class="epfl-alert epfl-alert-warning notice" role="alert">
        There is no saved view named “{{ unknownView }}”. Showing {{ activeView ? viewLabel(activeView) : 'all open issues' }} instead.
      </div>

      <!-- the saved views, a tab each in the file's order; the layout, a
           switch on any of them -->
      <h1 class="page-h1">Issues{{ activeView ? `: ${viewLabel(activeView)}` : '' }}</h1>
      <div class="view-head">
        <nav class="view-tabs" aria-label="Views">
          <router-link
            v-for="name in viewNames"
            :key="name"
            class="gb-tab"
            :to="{ path: '/', query: { view: name } }"
            :aria-current="name === activeView ? 'page' : undefined"
          >{{ viewLabel(name) }}</router-link>
        </nav>
        <p class="view-meta">
          <span v-if="loading && buckets">Updating…</span>
        </p>
        <div class="layout-switch" role="group" aria-label="Layout">
          <button v-for="l in layouts" :key="l" type="button" :aria-pressed="layout === l" @click="setLayout(l)">{{ displayName(l) }}</button>
        </div>
      </div>

      <!-- toolbar, unsaved-view bar and count stick under the header
           (wide screens); their height feeds --gb-tools-h, which the
           table's column titles and the board's height use -->
      <div ref="tools" class="tools">
      <div class="toolbar">
        <div class="toolbar-field toolbar-search">
          <label class="epfl-label" for="filter">Filter</label>
          <!-- a combobox (docs/prd/header-help.md, 5): examples from this
               board when empty, completions with their counts as you type,
               the filters' ? at its end -->
          <div class="fbox" @focusout="onBoxLeave">
            <input
              id="filter"
              ref="filterInput"
              class="epfl-input"
              type="search"
              role="combobox"
              aria-autocomplete="list"
              :aria-expanded="boxOpen && boxOptions.length > 0"
              aria-controls="filter-list"
              :aria-activedescendant="boxActive >= 0 ? `filter-opt-${boxActive}` : undefined"
              title="Words in titles, descriptions, comments; #number; field:value"
              spellcheck="false"
              placeholder="Words, #12, status:ready, assignee:@me, is:closed…"
              autocomplete="off"
              :value="filter"
              @focus="boxOpen = true"
              @click="boxOpen = true"
              @input="onBoxInput"
              @keydown="onBoxKey"
            />
            <FilterHelp ref="boxHelp" button-class="filters-help fbox-help" align="right" @try="tryWord" />
            <div v-if="boxOpen && boxOptions.length" class="fbox-pop" @mousedown.prevent>
              <p v-if="!completing" class="fbox-head">{{ filter.trim() ? 'Add' : 'Try' }}</p>
              <ul id="filter-list" role="listbox" aria-label="Filter suggestions">
                <li
                  v-for="(o, i) in boxOptions"
                  :id="`filter-opt-${i}`"
                  :key="o.value"
                  role="option"
                  :aria-selected="i === boxActive"
                  :class="{ on: i === boxActive }"
                  @mousemove="boxActive = i"
                  @click="takeOption(i)"
                >
                  <code>{{ o.label }}</code>
                  <span>{{ o.note }}</span>
                </li>
              </ul>
              <p class="fbox-foot">
                <span><kbd>↑</kbd><kbd>↓</kbd> choose</span>
                <span v-if="completing"><kbd>Tab</kbd> complete</span>
                <span v-if="trick" class="fbox-trick">{{ trick }}</span>
                <button type="button" class="gb-linkbtn fbox-how" @click="boxOpen = false, boxHelp?.show()">How filtering works</button>
              </p>
            </div>
          </div>
        </div>
        <div class="toolbar-field">
          <label class="epfl-label" for="group">Group by</label>
          <select id="group" v-model="groupField" class="epfl-select" @change="regroup">
            <option value="">No grouping</option>
            <option v-for="name in groupChoices" :key="name" :value="name">{{ displayName(name) }}</option>
          </select>
        </div>
        <div v-if="layout === 'board' && groupField" class="toolbar-field toolbar-lanes" title="Which columns this board shows">
          <span class="epfl-label">Lanes</span>
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
          <b>{{ activeView ? `Unsaved changes to ${viewLabel(activeView)}` : 'Not a saved view' }}</b>
          <code v-if="draft.filter" class="viewbar-filter">{{ draft.filter }}</code>
          <span v-else class="viewbar-meta">every issue</span>
          <span v-if="draft.layout !== viewLayout" class="viewbar-meta">as a {{ draft.layout }}</span>
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
          <p v-else-if="broader">Try a broader one: <button type="button" class="gb-linkbtn" @click="setFilter(broader)"><code>{{ broader }}</code></button></p>
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
            @more="lists.more"
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
          @more="lists.more"
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
      :has-prev="!!near.prev"
      :has-next="!!near.next"
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
import FilterHelp from '@/components/FilterHelp.vue';
import { keyContext, noteUse } from '@/keys';
import { debounced, onLive } from '@/live';
import { boardReload } from '@/livereload';
import { neighbours } from '@/panelnav';
import { panelMode } from '@/panelsize';
import { filtersHidden, newIssueRequested, recordRecent } from '@/commands';
import { numberName } from '@/palette';
import { changedSince, loadVisit, saveVisit, visit } from '@/lastvisit';
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { me } from '@/me';
import { useRoute, useRouter } from 'vue-router';
import { clientId, getFields, isStale, message, patchConfig, patchIssue, type FieldConfig, type Issue } from '@/api';
import type { Bucket } from '@/bucket';
import { apiSort, useBoardLists } from '@/boardlists';
import { configModel, loadConfigModel, takeFile } from '@/configmodel';
import type { ViewConfig } from '@/configparse';
import { namespaces, parseSelection, parseSort, prefillFrom } from '@/facets';
import { loadColors, values } from '@/colors';
import { completeTerms, composeFilter, suggest, withFacet } from '@/viewfilter';
import { clean } from '@/sectionpatch';
import { examples, nextTrick } from '@/filterhelp';
import { displayName, orderBuckets } from '@/fields';
import AppHeader from '@/components/AppHeader.vue';
import FilterSidebar from '@/components/FilterSidebar.vue';
import IssueBoard from '@/components/IssueBoard.vue';
import LaneChooser from '@/components/LaneChooser.vue';
import { boardLanes, noneFirst } from '@/lanes';
import IssueTable from '@/components/IssueTable.vue';
import RoadmapView from '@/components/RoadmapView.vue';
import GbIcon from '@/components/GbIcon.vue';
import IssuePanel from '@/components/IssuePanel.vue';
import NewIssueDialog from '@/components/NewIssueDialog.vue';

const route = useRoute();
const router = useRouter();

const views = computed<Record<string, ViewConfig>>(() => configModel.value?.views ?? {});
const fields = ref<Record<string, FieldConfig>>({});
const lists = useBoardLists(fields);
const configError = ref('');

// The filter (the Filter box, ?filter= in the URL once it differs from the
// view's) and the grouping go to the server, which matches and counts the
// facets; the sort below orders its answer here, and lives in the URL.
const groupField = ref('');
const columns = ref<string[] | undefined>(); // the table's, the view's until changed (tablecols.ts)
const lanes = ref<string[] | undefined>(); // the board's, the view's until changed (lanes.ts)
const limits = ref<Record<string, number> | undefined>();
const showAllLanes = ref(false);

const buckets = lists.buckets; // null until a load lands
const loading = lists.loading;
const listError = lists.error;

const viewNames = computed(() => configModel.value?.view_order ?? []); // the file's order
// a tab's words: current-sprint → Current sprint
const viewLabel = (name: string) => displayName(name.replaceAll('-', ' '));
const requestedView = computed(() => (typeof route.query.view === 'string' ? route.query.view : ''));
// The view in the URL, or the first one when none is asked for.
const activeView = computed(() =>
  requestedView.value in views.value ? requestedView.value : (viewNames.value[0] ?? ''),
);
const unknownView = computed(() =>
  requestedView.value && viewNames.value.length && !(requestedView.value in views.value)
    ? requestedView.value
    : '',
);
const view = computed(() => views.value[activeView.value]);
const layouts = ['table', 'board', 'roadmap'];
const viewLayout = computed(() => view.value?.layout || 'table');
// the view's layout, or the switch's (?layout=, so a link keeps it)
const layout = computed(() => {
  const l = route.query.layout;
  return typeof l === 'string' && layouts.includes(l) ? l : viewLayout.value;
});

type Patch = Record<string, string | string[] | undefined>;
/** Merge keys into the URL query; undefined removes one. */
const setQuery = (patch: Patch) => router.replace({ query: { ...route.query, ...patch } });

const viewFilter = computed(() => view.value?.filter ?? 'is:open');
const filter = computed(() => (typeof route.query.filter === 'string' ? route.query.filter : viewFilter.value));
// what the board shows: the filter without a term still being typed (prio:)
const applied = computed(() => completeTerms(filter.value));
// the empty state's suggestion: the filter one term shorter (never the
// filter itself)
const broader = computed(() => filter.value.trim().split(/\s+/).slice(0, -1).join(' '));
function setFilter(f: string) {
  return setQuery({ filter: f === viewFilter.value ? undefined : f });
}
const sort = computed(() =>
  parseSort(typeof route.query.sort === 'string' ? route.query.sort : (view.value?.sort ?? '')),
);
const sortValue = computed(() => (typeof route.query.sort === 'string' ? route.query.sort : ''));

// status: → status:ready, … (the values in use: colors.ts)
const suggestions = computed(() => suggest(filter.value, values.value));

// the Filter box's list: examples from this board when it's empty, else
// the completions of its last word, each with its count from the facets
const filterInput = ref<HTMLInputElement | null>(null);
const boxHelp = ref<InstanceType<typeof FilterHelp> | null>(null);
const boxOpen = ref(false);
const boxActive = ref(-1);
// completing: the last word's values; else examples from this board, the
// ones the box doesn't hold yet, added after what it holds
const completing = computed(() => suggestions.value.length > 0);
const boxOptions = computed<{ value: string; label: string; note: string }[]>(() => {
  if (!completing.value) {
    const f = filter.value.trim();
    const held = f.split(/\s+/);
    return examples(fields.value, me.value?.me?.login ?? '')
      .filter((e) => !held.includes(e.term))
      .map((e) => ({ value: f ? `${f} ${e.term}` : e.term, label: e.term, note: e.means }));
  }
  return suggestions.value.slice(0, 8).map((full) => {
    const word = full.split(/\s+/).at(-1) ?? full;
    const [key, v] = [word.slice(0, word.indexOf(':')), word.slice(word.indexOf(':') + 1).split(',').at(-1) ?? ''];
    const n = facetList.value.find((f) => f.key === key)?.options.find(([o]) => o === v)?.[1];
    return { value: full, label: word, note: n === undefined ? '' : `${n} ${n === 1 ? 'issue' : 'issues'}` };
  });
});
const trick = computed(() => (completing.value ? nextTrick(filter.value) : null));
watch(boxOptions, () => (boxActive.value = -1));
function onBoxInput(e: Event) {
  boxOpen.value = true;
  void setFilter((e.target as HTMLInputElement).value);
}
function takeOption(i: number) {
  const o = boxOptions.value[i];
  if (!o) return;
  void setFilter(o.value);
  boxActive.value = -1;
  filterInput.value?.focus();
}
// ↑↓ choose; Tab and Enter take the chosen one (Tab the first completion
// when none is chosen); Esc closes the list first; else the keys do what
// they did
function onBoxKey(e: KeyboardEvent) {
  const n = boxOptions.value.length;
  if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
    if (!n) return;
    e.preventDefault();
    boxOpen.value = true;
    boxActive.value = e.key === 'ArrowDown' ? (boxActive.value + 1) % n : (boxActive.value - 1 + n) % n;
    return;
  }
  if (!boxOpen.value || !n) return;
  if (e.key === 'Enter' && boxActive.value >= 0) {
    e.preventDefault();
    takeOption(boxActive.value);
  } else if (e.key === 'Tab' && !e.shiftKey && (boxActive.value >= 0 || completing.value)) {
    e.preventDefault();
    takeOption(Math.max(boxActive.value, 0));
  } else if (e.key === 'Escape') {
    e.preventDefault();
    e.stopPropagation();
    boxOpen.value = false;
  }
}
function onBoxLeave(e: FocusEvent) {
  const box = e.currentTarget as HTMLElement;
  if (!(e.relatedTarget instanceof Node && box.contains(e.relatedTarget))) boxOpen.value = false;
}
// Try, in "How filtering works": the word joins what the box holds
function tryWord(term: string) {
  const f = filter.value.trim();
  void setFilter(f ? `${f} ${term}` : term);
  boxOpen.value = false;
  void nextTick(() => filterInput.value?.focus());
}

// the facets, the server's for the filter: the ticks are the values it
// lets through, a tick writes a new filter
const facetList = lists.facets;
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
    applied.value !== completeTerms(viewFilter.value) ||
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
// the server sorts each list; the page only leaves out what didn't change
const shown = computed<Bucket[]>(() =>
  orderBuckets(buckets.value ?? [], groupField.value, fields.value).map((b) =>
    onlyChanged.value ? { ...b, issues: b.issues.filter((i) => changed.value.has(i.id)), total: undefined, next: undefined } : b,
  ),
);
// A kanban shows every option of the field it's grouped by, empty or not,
// so a card can be dropped there.
const boardBuckets = computed(() => {
  const options = fields.value[groupField.value]?.options ?? [];
  const present = new Set(shown.value.map((b) => b.group));
  const empty = options.filter((o) => !present.has(o)).map((o): Bucket => ({ group: o, issues: [], total: 0 }));
  return noneFirst(orderBuckets([...shown.value, ...empty], groupField.value, fields.value));
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

// A card dropped in another lane takes its value: moved at once, saved in
// one label change from the version the card was read at (the old value
// out, the new in). Picked cards, or a lane's "Move all to…": one change
// each, in turn; the first refusal stops and says which. A card someone
// changed meanwhile (412) isn't moved: the board reloads to show it.
const moveError = ref('');
const labelsFor = (from: string, to: string) => ({
  add: to === '(none)' ? [] : [`${groupField.value}:${to}`],
  remove: from === '(none)' ? [] : [`${groupField.value}:${from}`],
});
async function onMoveMany(moves: { issue: Issue; from: string }[], to: string) {
  moveError.value = '';
  for (const { issue, from } of moves) lists.moved(issue, from, to);
  for (const [n, { issue, from }] of moves.entries()) {
    try {
      await patchIssue(issue.id, issue.version, { labels: labelsFor(from, to) });
    } catch (e: unknown) {
      // after the alert's "Not moved."
      moveError.value = `“${issue.shown_title}”: ${moveWhy(e)} ${n} of ${moves.length} moved before it, the rest not tried.`;
      break;
    }
  }
  void load();
}
const moveWhy = (e: unknown) => (isStale(e) ? 'it changed meanwhile; the board shows it as it is now, move it again.' : `${message(e)}.`);
async function onMove(issue: Issue, from: string, to: string) {
  moveError.value = '';
  lists.moved(issue, from, to);
  try {
    await patchIssue(issue.id, issue.version, { labels: labelsFor(from, to) });
  } catch (e: unknown) {
    moveError.value = moveWhy(e);
  }
  void load();
}

const total = computed(() => lists.total.value);
const count = computed(() => (onlyChanged.value ? changed.value.size : total.value));


// New issue starts with the fields the filter picks one value of
const prefill = computed(() => prefillFrom(facetList.value, fields.value));

// A header click sorts ascending, a second one descending.
function sortBy(key: string) {
  const s = sort.value;
  void setQuery({ sort: s?.key === key && !s.desc ? `-${key}` : key });
}

const sortChoices = computed(() =>
  [
    { key: 'title', label: 'Title' },
    { key: 'created', label: 'Created' },
    { key: 'updated', label: 'Updated' },
    ...names.value.map((n) => ({ key: n, label: displayName(n) })),
  ].flatMap(({ key, label }) => [
    { value: key, label: `${label}, ascending` },
    { value: `-${key}`, label: `${label}, descending` },
  ]),
);

// a board by nothing is one column: the switch groups it by status, when
// there's one, and takes that back on the way out
let switchGrouped = false;
async function setLayout(l: string) {
  await setQuery({ layout: l === viewLayout.value ? undefined : l });
  if (l === 'board' && !groupField.value && groupChoices.value.includes('status')) {
    groupField.value = 'status';
    switchGrouped = true;
    regroup();
  } else if (l !== 'board' && switchGrouped && groupField.value === 'status') {
    groupField.value = view.value?.group ?? '';
    switchGrouped = false;
    regroup();
  }
}

function onSortSelect(event: Event) {
  void setQuery({ sort: (event.target as HTMLSelectElement).value || undefined });
}

// The fields found on the issues, plus the view's own group key.
const groupChoices = computed(() => {
  const g = view.value?.group;
  return g && !names.value.includes(g) ? [...names.value, g] : names.value;
});

// The view as the page shows it now, and whether that differs from the
// saved one: the bar offers to save it into .git-board.toml.
const draft = computed<ViewConfig>(() => ({
  layout: layout.value,
  filter: applied.value,
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
  if (!v) return d.layout !== 'table' || d.filter !== 'is:open' || d.group !== '' || d.sort !== '' || !!d.columns || !!d.lanes || !!d.limits;
  return d.layout !== viewLayout.value || d.filter !== v.filter || d.group !== (v.group ?? '') || d.sort !== (v.sort ?? '') || !sameColumns(d.columns, v.columns) || lanesChanged.value;
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
    await loadConfigModel();
    const d = draft.value;
    const v = { layout: d.layout, filter: d.filter, group: d.group, sort: d.sort, columns: d.columns, lanes: d.lanes, limits: d.limits };
    takeFile(await patchConfig({ [`views.${name}`]: clean(v) }, configModel.value?.hash ?? ''));
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

// The file first (the views): the list waits for nothing else. The fields
// come beside the first list, the values' colors and suggestions after it.
let fieldsRead: Promise<Record<string, FieldConfig>> | null = null;
async function loadConfig() {
  configError.value = '';
  try {
    await loadConfigModel();
    fieldsRead = getFields();
    fields.value = await fieldsRead;
  } catch (e: unknown) {
    configError.value = message(e);
  }
}

// Back to the view's grouping, columns and lanes (the URL's filter stays:
// a shared link keeps it).
function resetView() {
  switchGrouped = false;
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

let loaded = ''; // the filter last asked for
let colorsAsked = false;
async function load() {
  loaded = applied.value;
  await lists.load({ filter: applied.value, group: groupField.value, sort: apiSort(sort.value) });
  if (!colorsAsked && fieldsRead) {
    colorsAsked = true;
    void fieldsRead.then(loadColors, () => {});
  }
}
watch(sort, () => void load());

watch(activeView, resetView);

// the filter runs on the server (git-bug's full-text index for words):
// asked once typing pauses, unless a tick or the view asked already
let filterTimer: ReturnType<typeof setTimeout> | undefined;
watch(applied, () => {
  clearTimeout(filterTimer);
  filterTimer = setTimeout(() => applied.value !== loaded && void load(), 50);
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
  void loadConfig();
  await loadConfigModel(); // the views, for the first list
  await upgradeLink();
  watch(() => route.query, upgradeLink);
  // No saved view to switch to (none configured, or the config failed):
  // the watch above never fires, so load the default filter here.
  if (!activeView.value) resetView();
});

// The issue panel: ?issue=<id> in the URL, so a link opens it; j / k move
// through the issues as the board shows them.
const detailOpen = ref(false);
// where you are, for the key line and the tips (keys.ts): the board, an
// issue open over it, the pointer or the focus in the filters
const inFilters = ref(false);
watch([detailOpen, inFilters], ([open, filters]) => {
  keyContext.value = ['board', ...(open ? (['panel'] as const) : []), ...(filters ? (['filters'] as const) : [])];
}, { immediate: true });
onBeforeUnmount(() => (keyContext.value = []));
const detailId = ref('');
const flat = computed(() => shown.value.flatMap((b) => b.issues));
const index = computed(() => flat.value.findIndex((i) => i.id === detailId.value));
// where the open issue last was in the list: closed or moved out of the
// view from the panel, j/k go on from there (panelnav.ts)
const lastAt = ref(-1);
watch(detailId, () => (lastAt.value = index.value));
watch(index, (i) => i >= 0 && (lastAt.value = i));
const near = computed(() => neighbours(flat.value.map((i) => i.id), detailId.value, lastAt.value));
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
  const next = delta < 0 ? near.value.prev : near.value.next;
  if (next) openId(next);
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
      if (i) recordRecent({ id, title: i.shown_title, number: numberName(i) });
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
  noteUse('new');
  newOpen.value = true;
}
onMounted(() => window.addEventListener('keydown', onKey));
onBeforeUnmount(() => window.removeEventListener('keydown', onKey));

// live updates (live.ts): another page or a sync changed the issues or the
// configuration; reload once a burst is over (the board keeps its scroll)
const reloadLive = debounced(() => void Promise.all([load(), loadColors(fields.value)]));
const reloadConfig = debounced(async () => {
  await loadConfig();
  void load();
});
// what an event reloads (livereload.ts): this tab's own writes to the
// issues too, so a panel closed before its write answered still shows
const offLive = onLive((e) => {
  const what = boardReload(e, clientId);
  if (what === 'config') reloadConfig();
  else if (what === 'issues') reloadLive();
}, { own: true });
onBeforeUnmount(offLive);
</script>

<style scoped>
/* the tools stick under the header on wider screens */
@media (min-width: 48rem) {
  .tools { position: sticky; top: var(--header-h); z-index: 20; margin: 0 calc(-1 * var(--space-2)); padding: var(--space-2) var(--space-2) 0; background: var(--bg); box-shadow: 0 1px 0 var(--border-subtle); }
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
/* the folded filters: the rail shows them, its ? the shortcuts */
.filters-rail { position: sticky; top: var(--header-h); align-self: start; display: flex; flex-direction: column; align-items: center; gap: var(--space-2); width: 2.75rem; margin-top: var(--space-4); padding-bottom: var(--space-3); border: var(--border-w) solid var(--border); background: var(--bg-subtle); }
.filters-rail:has(.filters-rail-show:hover) { border-color: var(--fg); }
.filters-rail-show { display: flex; flex-direction: column; align-items: center; gap: var(--space-2); width: 100%; min-height: 12rem; padding: var(--space-3) 0; border: 0; background: none; color: var(--fg); cursor: pointer; font: inherit; }
.filters-rail-show:focus-visible { outline: var(--focus-w) solid var(--primary-hover); outline-offset: -0.125rem; }
.filters-rail-label { writing-mode: vertical-rl; transform: rotate(180deg); font-weight: var(--w-bold); font-size: var(--fs-sm); letter-spacing: .04em; }
.filters-rail-badge { min-width: 1.25rem; padding: 0 .25rem; background: var(--primary); color: var(--on-primary); font-size: .75rem; font-weight: var(--w-bold); text-align: center; border-radius: var(--radius); }

.notice { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-2); margin-bottom: var(--space-3); }

/* the header's Issues says it: the h1 is for screen readers */
.page-h1 { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
/* the tabs (gitboard.css's .gb-tab), scrolling sideways when they're many,
   on a line their bar covers; then what the view is doing, and the layout */
.view-head { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-2) var(--space-3); margin-bottom: var(--space-3); }
.view-tabs { display: flex; flex: 1 1 auto; min-width: 0; overflow-x: auto; box-shadow: inset 0 -1px 0 var(--border); }
.view-tabs .gb-tab { display: inline-flex; flex: none; align-items: center; white-space: nowrap; text-decoration: none; }
.view-tabs .gb-tab::after { bottom: 0; }
.view-meta { display: flex; gap: var(--space-2); margin: 0; font-size: var(--fs-sm); color: var(--fg-muted); }
.page-report { padding: 0; border: 0; background: none; color: var(--fg); font: inherit; text-decoration: underline; cursor: pointer; }
.layout-switch { display: inline-flex; border: 1px solid var(--border-strong); }
.layout-switch button { padding: .375rem .75rem; border: 0; background: var(--bg); color: var(--fg-muted); font: inherit; font-size: var(--fs-sm); cursor: pointer; }
.layout-switch button + button { border-left: 1px solid var(--border-strong); }
.layout-switch button:hover { color: var(--fg); }
.layout-switch button[aria-pressed="true"] { background: var(--fg); color: var(--bg); font-weight: var(--w-bold); }
.layout-switch button:focus-visible { outline: var(--focus-w) solid var(--primary-hover); outline-offset: 0.125rem; }

.toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  gap: var(--space-3);
  margin-bottom: var(--space-2);
}
.toolbar-field { flex: 0 1 13rem; min-width: 0; }
.toolbar-search { flex: 1 1 20rem; max-width: 32rem; }
.fbox { position: relative; }
.fbox > .epfl-input { width: 100%; padding-right: 2.5rem; }
.fbox > :deep(.fbox-help) { position: absolute; top: 50%; right: .625rem; transform: translateY(-50%); }
.fbox-pop { position: absolute; z-index: 25; top: calc(100% + .25rem); left: 0; width: max(100%, 30rem); border: var(--border-w) solid var(--border-strong); background: var(--bg); box-shadow: var(--shadow); font-size: .875rem; }
.fbox-head { margin: 0; padding: .625rem 1rem .25rem; color: var(--fg-muted); font-size: .75rem; font-weight: var(--w-bold); letter-spacing: .06em; text-transform: uppercase; }
.fbox-pop ul { margin: 0; padding: .25rem 0; list-style: none; }
.fbox-pop li { display: flex; align-items: center; gap: .75rem; padding: .4375rem 1rem; cursor: pointer; }
.fbox-pop li.on { background: var(--bg-muted); box-shadow: inset 3px 0 0 var(--primary); }
.fbox-pop li > span { margin-left: auto; color: var(--fg-muted); font-size: .8125rem; text-align: right; }
.fbox-pop code { padding: .0625rem .3125rem; border: var(--border-w) solid var(--border-subtle); background: var(--bg-subtle); font-size: .8125rem; }
.fbox-foot { display: flex; flex-wrap: wrap; align-items: center; gap: .375rem 1rem; margin: 0; padding: .5rem 1rem; border-top: var(--border-w) solid var(--border-subtle); color: var(--fg-muted); font-size: .78125rem; }
.fbox-foot > span { display: inline-flex; align-items: center; gap: .1875rem; }
.fbox-how { margin-left: auto; }
.fbox-pop kbd { min-width: .75rem; padding: 0 .3125rem; border: var(--border-w) solid var(--border-strong); border-bottom-width: .125rem; border-radius: var(--radius); background: var(--bg); color: var(--fg); font-family: var(--font-sans); font-size: .71875rem; line-height: 1.35; text-align: center; }
.toolbar-lanes { flex: 0 0 auto; }
.toolbar-new { display: flex; align-items: center; gap: .5rem; margin-left: auto; }
/* as high as the fields beside it */
.toolbar-new .epfl-btn { display: inline-flex; align-items: center; gap: .375rem; min-height: var(--control-h); padding-block: 0; }

.status {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2) 0.75rem;
  margin-bottom: var(--space-3);
  padding-bottom: var(--space-2);
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
