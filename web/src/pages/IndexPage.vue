<template>
  <q-page class="q-pa-md">
    <div class="row q-col-gutter-md items-end q-mb-md">
      <div class="col-auto" style="min-width: 220px">
        <q-select
          v-model="selectedView"
          :options="viewNames"
          label="View"
          clearable
          dense
          outlined
          @update:model-value="onViewChange"
        />
      </div>
      <div class="col">
        <q-input
          v-model="filterText"
          label="Filter"
          hint="e.g. is:open status:ready"
          dense
          outlined
          @keyup.enter="load"
        />
      </div>
      <div class="col-auto">
        <q-btn color="primary" label="Apply" no-caps @click="load" />
      </div>
    </div>

    <q-banner v-if="error" class="bg-negative text-white q-mb-md">{{ error }}</q-banner>

    <q-inner-loading :showing="loading" />

    <div v-if="!loading && buckets.length === 0" class="text-grey text-center q-pa-xl">
      No issues match this filter.
    </div>

    <div v-for="bucket in buckets" :key="bucket.group">
      <div v-if="bucket.group" class="text-subtitle1 text-weight-bold q-mt-md">
        {{ bucket.group }}
      </div>
      <q-table
        :rows="bucket.issues"
        :columns="columns"
        row-key="id"
        flat
        bordered
        dense
        @row-click="(_evt, row) => openIssue(row as Issue)"
      />
    </div>

    <q-dialog v-model="dialogOpen">
      <q-card v-if="activeIssue" style="min-width: 420px">
        <q-card-section>
          <div class="text-h6">{{ activeIssue.title }}</div>
          <div class="text-caption text-grey">{{ activeIssue.id }}</div>
        </q-card-section>

        <q-card-section class="q-gutter-md">
          <q-select
            v-for="name in singleFieldNames"
            :key="name"
            :model-value="fieldValue(activeIssue, name)"
            :options="fields[name]?.options ?? []"
            :label="name"
            dense
            outlined
            @update:model-value="(v) => setSingleField(name, v as string)"
          />
          <q-select
            v-for="name in multiFieldNames"
            :key="name"
            :model-value="fieldValues(activeIssue, name)"
            :label="name"
            multiple
            use-input
            use-chips
            new-value-mode="add-unique"
            dense
            outlined
            @update:model-value="(v) => setMultiField(name, v as string[])"
          />
        </q-card-section>

        <q-card-actions align="right">
          <q-btn flat label="Close" color="primary" v-close-popup />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { useQuasar } from 'quasar';
import {
  getViews, getFields, getIssues, patchIssue,
  type ViewConfig, type FieldConfig, type Bucket, type Issue,
} from '@/api/board';

const $q = useQuasar();

const views = ref<Record<string, ViewConfig>>({});
const fields = ref<Record<string, FieldConfig>>({});
const buckets = ref<Bucket[]>([]);
const loading = ref(false);
const error = ref('');

const selectedView = ref<string | null>(null);
const filterText = ref('is:open');
const sortSpec = ref('');
const groupField = ref('');

const viewNames = computed(() => Object.keys(views.value));
const singleFieldNames = computed(() =>
  Object.keys(fields.value).filter((n) => fields.value[n]?.single),
);
const multiFieldNames = computed(() =>
  Object.keys(fields.value).filter((n) => !fields.value[n]?.single),
);

const columns = [
  { name: 'title', label: 'Title', field: 'title', align: 'left' as const },
  { name: 'status', label: 'Status', field: 'status', align: 'left' as const },
  {
    name: 'labels',
    label: 'Labels',
    field: (row: Issue) => row.labels.join(', '),
    align: 'left' as const,
  },
];

function fieldValue(issue: Issue, name: string): string {
  const prefix = `${name}:`;
  return issue.labels.find((l) => l.startsWith(prefix))?.slice(prefix.length) ?? '';
}

function fieldValues(issue: Issue, name: string): string[] {
  const prefix = `${name}:`;
  return issue.labels.filter((l) => l.startsWith(prefix)).map((l) => l.slice(prefix.length));
}

function onViewChange(name: string | null) {
  const view = name ? views.value[name] : undefined;
  filterText.value = view?.filter ?? 'is:open';
  sortSpec.value = view?.sort ?? '';
  groupField.value = view?.group ?? '';
  void load();
}

async function load() {
  loading.value = true;
  error.value = '';
  try {
    buckets.value = await getIssues(filterText.value, sortSpec.value, groupField.value);
  } catch (e: unknown) {
    error.value = e instanceof Error ? e.message : String(e);
  } finally {
    loading.value = false;
  }
}

const dialogOpen = ref(false);
const activeIssue = ref<Issue | null>(null);

function openIssue(issue: Issue) {
  activeIssue.value = issue;
  dialogOpen.value = true;
}

// Re-fetches and re-buckets after any field change — a patched group field
// (e.g. status on the current-sprint view) must move the row to a new
// bucket, not just update it in place.
async function refreshAfterPatch(updated: Issue) {
  activeIssue.value = updated;
  await load();
}

async function setSingleField(name: string, value: string) {
  if (!activeIssue.value || !value) return;
  try {
    const updated = await patchIssue(activeIssue.value.id, { [name]: value });
    await refreshAfterPatch(updated);
  } catch (e: unknown) {
    $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
  }
}

async function setMultiField(name: string, newValues: string[]) {
  if (!activeIssue.value) return;
  const before = fieldValues(activeIssue.value, name);
  const added = newValues.filter((v) => !before.includes(v));
  const removed = before.filter((v) => !newValues.includes(v));
  try {
    let updated = activeIssue.value;
    for (const v of added) updated = await patchIssue(updated.id, { [name]: v });
    for (const v of removed) updated = await patchIssue(updated.id, { [name]: `-${v}` });
    await refreshAfterPatch(updated);
  } catch (e: unknown) {
    $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
  }
}

onMounted(async () => {
  try {
    [views.value, fields.value] = await Promise.all([getViews(), getFields()]);
  } catch (e: unknown) {
    error.value = e instanceof Error ? e.message : String(e);
  }
  await load();
});
</script>
