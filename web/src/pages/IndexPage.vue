<template>
  <AppHeader :views="viewNames" :active="activeView" />

  <main class="page">
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
        <template v-if="buckets"> · {{ count }} {{ count === 1 ? 'issue' : 'issues' }}</template>
        <template v-if="groupField"> · grouped by {{ groupField }}</template>
        <template v-if="loading && buckets"> · updating…</template>
      </p>
    </div>

    <form class="toolbar" role="search" aria-label="Filter issues" @submit.prevent="load">
      <div class="toolbar-field toolbar-filter">
        <label class="epfl-label" for="filter">Filter</label>
        <input
          id="filter"
          v-model="filterText"
          class="epfl-input"
          type="text"
          spellcheck="false"
          autocomplete="off"
          aria-describedby="filter-hint"
        />
        <small id="filter-hint">For example: is:open status:ready prio:p1 -title:*spec*</small>
      </div>
      <div class="toolbar-field">
        <label class="epfl-label" for="group">Group by</label>
        <select id="group" v-model="groupField" class="epfl-select" @change="load">
          <option value="">No grouping</option>
          <option v-for="name in groupChoices" :key="name" :value="name">{{ displayName(name) }}</option>
        </select>
      </div>
      <div class="toolbar-field">
        <label class="epfl-label" for="sort">Sort by</label>
        <select id="sort" v-model="sortSpec" class="epfl-select" @change="load">
          <option value="">Default order</option>
          <option v-for="s in sortChoices" :key="s" :value="s">{{ sortLabel(s) }}</option>
        </select>
      </div>
      <div class="toolbar-actions">
        <button type="submit" class="epfl-btn epfl-btn-primary">Apply</button>
        <button v-if="modified" type="button" class="epfl-btn epfl-btn-secondary" @click="resetView">
          Reset view
        </button>
      </div>
    </form>

    <div :aria-busy="loading">
      <div v-if="listError" class="epfl-alert epfl-alert-danger" role="alert">
        <b>Could not load issues.</b> {{ listError }}
      </div>
      <p v-else-if="!buckets" class="state" role="status">Loading issues…</p>
      <div v-else-if="count === 0" class="state state-empty">
        <p class="state-title">No issues match this filter.</p>
        <p>Try a broader filter, for example <code>is:open</code>.</p>
      </div>
      <IssueBoard
        v-else-if="layout === 'board'"
        :buckets="ordered"
        :fields="fields"
        :group="groupField"
        @open="openIssue"
      />
      <IssueTable v-else :buckets="ordered" :fields="fields" :group="groupField" @open="openIssue" />
    </div>
  </main>

  <q-dialog v-model="detailOpen" position="right" full-height aria-labelledby="issue-detail-title">
    <IssueDetail
      v-if="detailIssue"
      :key="detailIssue.id"
      :issue="detailIssue"
      :fields="fields"
      @updated="onUpdated"
      @close="detailOpen = false"
    />
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { useRoute } from 'vue-router';
import {
  getFields, getIssues, getViews,
  type Bucket, type FieldConfig, type Issue, type ViewConfig,
} from '@/api/board';
import { displayName, fieldNames, orderBuckets } from '@/fields';
import AppHeader from '@/components/AppHeader.vue';
import IssueBoard from '@/components/IssueBoard.vue';
import IssueTable from '@/components/IssueTable.vue';
import IssueDetail from '@/components/IssueDetail.vue';

const route = useRoute();

const views = ref<Record<string, ViewConfig>>({});
const fields = ref<Record<string, FieldConfig>>({});
const configError = ref('');

const filterText = ref('is:open');
const groupField = ref('');
const sortSpec = ref('');

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
const modified = computed(
  () =>
    !!view.value &&
    (filterText.value !== view.value.filter ||
      groupField.value !== view.value.group ||
      sortSpec.value !== view.value.sort),
);

const ordered = computed(() => orderBuckets(buckets.value ?? [], groupField.value, fields.value));
const count = computed(() => (buckets.value ?? []).reduce((n, b) => n + b.issues.length, 0));

// Configured fields, plus the view's own group/sort key when it isn't one
// (e.g. backlog groups by the unconfigured "sprint" label namespace).
const groupChoices = computed(() => withExtra(fieldNames(fields.value), view.value?.group));
const sortChoices = computed(() =>
  withExtra(
    fieldNames(fields.value).flatMap((n) => [`${n} asc`, `${n} desc`]),
    view.value?.sort,
  ),
);

function withExtra(list: string[], extra: string | undefined): string[] {
  return extra && !list.includes(extra) ? [...list, extra] : list;
}

function sortLabel(spec: string): string {
  const [name = '', dir] = spec.split(' ');
  return `${displayName(name)}, ${dir === 'desc' ? 'descending' : 'ascending'}`;
}

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

function resetView() {
  filterText.value = view.value ? view.value.filter : 'is:open';
  groupField.value = view.value?.group ?? '';
  sortSpec.value = view.value?.sort ?? '';
  void load();
}

// Only the latest request may write: switching views quickly must not let a
// slow earlier response overwrite a newer one.
let latest = 0;
async function load() {
  const mine = ++latest;
  loading.value = true;
  listError.value = '';
  try {
    const result = await getIssues(filterText.value, sortSpec.value, groupField.value);
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
.page { padding: var(--space-4) var(--gutter) var(--space-5); }

.notice { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-2); margin-bottom: var(--space-3); }

.page-head { margin-bottom: var(--space-4); }
.page-title { margin: 0 0 var(--space-1); overflow-wrap: anywhere; }
.page-meta { margin: 0; font-size: var(--fs-sm); color: var(--fg-muted); }

.toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  gap: var(--space-3);
  margin-bottom: var(--space-4);
  padding-bottom: var(--space-4);
  border-bottom: var(--border-w) solid var(--border-subtle);
}
.toolbar-field { flex: 0 1 13rem; min-width: 0; }
.toolbar-filter { flex: 1 1 22rem; }
.toolbar-filter small { display: block; margin-top: var(--space-1); }
.toolbar-filter .epfl-input { font-family: var(--font-mono); font-size: var(--fs-sm); }
.toolbar-actions { display: flex; gap: var(--space-2); margin-top: 1.4rem; }

.state { margin: 0; padding: var(--space-5) var(--space-3); text-align: center; color: var(--fg-muted); }
.state-empty { border: var(--border-w) solid var(--border); }
.state-empty p { margin: 0; }
.state-title { color: var(--fg); font-weight: var(--w-bold); margin-bottom: var(--space-1); }

@media (max-width: 47.99rem) {
  .page { padding: var(--space-3) var(--space-3) var(--space-5); }
  .toolbar-field,
  .toolbar-filter { flex-basis: 100%; }
  .toolbar-actions { margin-top: 0; }
}
</style>
