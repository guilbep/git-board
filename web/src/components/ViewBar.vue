<!-- What differs from the saved view, as GitHub Projects shows it: saved
     into .git-board.toml for everyone (PATCH /api/config, the view's
     table), as a new view, or dropped. -->
<template>
  <span v-if="viewDirty" class="viewbar" role="region" aria-label="Unsaved changes to the view">
    <span class="viewbar-what" :title="details">{{ activeView ? `${viewLabel(activeView)} modified` : 'Not a saved view' }}</span>
    <form v-if="naming" class="viewbar-name" @submit.prevent="saveAs">
      <input id="viewname" aria-label="New view name" ref="nameInput" v-model="newName" class="epfl-input" placeholder="new-view-name" pattern="[A-Za-z0-9_\-]+" title="Letters, digits, - and _" required @keydown.esc.stop="naming = false" />
      <button type="submit" class="viewbar-link" :disabled="savingView">Save</button>
      <button type="button" class="viewbar-link" @click="naming = false">Cancel</button>
    </form>
    <template v-else>
      <button v-if="activeView" type="button" class="viewbar-link" :disabled="savingView" @click="save(activeView)">Save view</button>
      <button type="button" class="viewbar-link" @click="startNaming">Save as new view…</button>
      <button type="button" class="viewbar-link" @click="emit('discard')">Discard</button>
    </template>
    <span v-if="viewError" class="viewbar-error" role="alert">Not saved: {{ viewError }}</span>
  </span>
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
// what differs, for the tooltip: the bar itself is one quiet line
const details = computed(() => {
  const d = props.draft;
  return [
    d.filter ? `filter ${d.filter}` : 'every issue',
    d.layout !== props.viewLayout && `as a ${d.layout}`,
    d.group && `grouped by ${d.group}`,
    d.sort && `sorted by ${d.sort}`,
    !sameColumns(d.columns, props.view?.columns) && (d.columns ? 'columns of its own' : 'default columns'),
    lanesChanged.value && (d.lanes ? `lanes: ${d.lanes.join(', ')}` : 'every lane'),
  ].filter(Boolean).join(' · ');
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
/* one quiet line at the end of the status row: a sort or a filter tried
   moves nothing on the page */
.viewbar { display: inline-flex; flex-wrap: wrap; align-items: center; gap: .25rem .75rem; margin-left: auto; font-size: var(--fs-sm); color: var(--fg-muted); }
.viewbar-what { cursor: help; }
.viewbar-link { padding: 0; border: 0; background: none; color: var(--fg-muted); text-decoration: underline; cursor: pointer; font: inherit; }
.viewbar-link:hover { color: var(--primary-hover); }
.viewbar-link:disabled { cursor: default; opacity: .6; }
.viewbar-name { display: inline-flex; align-items: center; gap: .5rem; }
.viewbar-name .epfl-input { width: 12rem; min-height: 1.75rem; padding: .125rem .5rem; }
.viewbar-error { color: var(--danger-fg); }
</style>
