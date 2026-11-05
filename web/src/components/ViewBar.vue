<!-- What differs from the saved view, as GitHub Projects shows it: saved
     into .git-board.toml for everyone (PATCH /api/config, the view's
     table), as a new view, or dropped. -->
<template>
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
      <button type="button" class="viewbar-discard" @click="emit('discard')">Discard</button>
    </template>
    <p v-if="viewError" class="viewbar-note viewbar-error" role="alert">Not saved: {{ viewError }}</p>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { message, patchConfig } from '@/api';
import { configModel, loadConfigModel, takeFile } from '@/configmodel';
import type { ViewConfig } from '@/configparse';
import { viewLabel } from '@/fields';
import { clean } from '@/sectionpatch';

// draft: the view as the page shows it now; view: the saved one
const props = defineProps<{ draft: ViewConfig; view: ViewConfig | undefined; activeView: string; viewLayout: string }>();
const emit = defineEmits<{ saved: [name: string]; discard: [] }>();

const sameColumns = (a?: string[], b?: string[]) => (a ?? []).join(',') === (b ?? []).join(',');
const sameLimits = (a?: Record<string, number>, b?: Record<string, number>) =>
  JSON.stringify(Object.entries(a ?? {}).sort()) === JSON.stringify(Object.entries(b ?? {}).sort());
const lanesChanged = computed(() => !sameColumns(props.draft.lanes, props.view?.lanes) || !sameLimits(props.draft.limits, props.view?.limits));
const viewDirty = computed(() => {
  const v = props.view;
  const d = props.draft;
  if (!v) return d.layout !== 'table' || d.filter !== 'is:open' || d.group !== '' || d.sort !== '' || !!d.columns || !!d.lanes || !!d.limits;
  return d.layout !== props.viewLayout || d.filter !== v.filter || d.group !== (v.group ?? '') || d.sort !== (v.sort ?? '') || !sameColumns(d.columns, v.columns) || lanesChanged.value;
});
const naming = ref(false);
const newName = ref('');
const nameInput = ref<HTMLInputElement | null>(null);
const savingView = ref(false);
const viewError = ref('');

function startNaming() {
  newName.value = props.activeView ? `${props.activeView}-copy` : '';
  naming.value = true;
  void Promise.resolve().then(() => nameInput.value?.select());
}

async function save(name: string) {
  savingView.value = true;
  viewError.value = '';
  try {
    await loadConfigModel();
    const d = props.draft;
    const v = { layout: d.layout, filter: d.filter, group: d.group, sort: d.sort, columns: d.columns, lanes: d.lanes, limits: d.limits };
    takeFile(await patchConfig({ [`views.${name}`]: clean(v) }, configModel.value?.hash ?? ''));
    naming.value = false;
    emit('saved', name);
  } catch (e) {
    viewError.value = message(e);
  } finally {
    savingView.value = false;
  }
}

const saveAs = () => save(newName.value.trim());
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
</style>
