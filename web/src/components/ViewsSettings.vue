<!-- Settings › Views: the [views.<name>] tables, the tabs above the issues,
     in the file's order — the first is the one git-board opens on. A card
     each: layout, filter, group, sort, columns; ↑ and ↓ reorder, Remove
     deletes the tab, Add offers the presets. The filter takes the one
     syntax everywhere; its reference sits beside the form. -->
<template>
  <div class="set-panel vs">
    <h2 class="h4">Views</h2>
    <p class="vs-lead">
      A view is a saved page: a tab above the issues, in this order. Table, board or roadmap is a switch on every view —
      <code>layout</code> is only the one it opens in. On the board, change the filter and <b>Save view</b> writes here too.
    </p>

    <p v-if="modelError" class="epfl-alert epfl-alert-danger" role="alert">{{ modelError }}</p>
    <p v-else-if="!draft.value.value" class="vs-muted">Loading…</p>

    <template v-else>
      <div class="vs-tools">
        <ConfigPreset label="+ Add a view" :presets="viewPresets" @pick="addPreset" />
        <span class="gb-hint">or start from a blank one</span>
        <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" @click="addBlank">Blank view</button>
      </div>

      <div v-for="(v, i) in views" :key="v.key" class="vs-item">
        <span class="vs-order">
          <button type="button" class="vs-btn" :disabled="i === 0" aria-label="Move the tab up" @click="move(i, -1)">↑</button>
          <button type="button" class="vs-btn" :disabled="i === views.length - 1" aria-label="Move the tab down" @click="move(i, 1)">↓</button>
        </span>
        <ConfigCard
          class="vs-card"
          :title="v.name || 'a new view'"
          :removable="true"
          :confirm="`The tab ${v.name} goes from everyone's board. A link to it (#/…?view=${v.name}) shows the first view instead.`"
          @remove="remove(i)"
        >
          <div class="vs-form">
            <label class="vs-field">
              <span>Name</span>
              <input v-model="v.name" class="vs-input" :disabled="!v.isNew" spellcheck="false" aria-label="The view's name" />
            </label>
            <label class="vs-field">
              <span>Opens as</span>
              <select v-model="v.view.layout" class="vs-input vs-small" aria-label="Layout">
                <option value="table">table</option>
                <option value="board">board</option>
                <option value="roadmap">roadmap</option>
              </select>
            </label>
            <label class="vs-field vs-grow">
              <span>Filter</span>
              <input v-model="v.view.filter" class="vs-input" spellcheck="false" placeholder="is:open" aria-label="Filter" />
            </label>
            <label class="vs-field">
              <span>Group by</span>
              <input v-model="v.view.group" class="vs-input vs-small" spellcheck="false" placeholder="status" aria-label="Group" />
            </label>
            <label class="vs-field">
              <span>Sort</span>
              <input v-model="v.view.sort" class="vs-input vs-small" spellcheck="false" placeholder="prio asc" aria-label="Sort" />
            </label>
          </div>
        </ConfigCard>
      </div>

      <ConfigSaveBar
        section="views"
        :dirty="draft.dirty.value"
        :saving="draft.saving.value"
        :saved="draft.saved.value"
        :valid="valid"
        :error="draft.saveError.value"
        :conflict="draft.conflict.value"
        :state="state"
        @save="save"
        @discard="reset"
        @reload="draft.load()"
      />
    </template>

    <ConfigRefPanel :sections="['views', 'filters']" title="a view's keys, the filter's syntax" />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import type { ViewConfig } from '@/api/board';
import { takeUnique, viewPresets } from '@/configpresets';
import { SectionDraft, configModel, configModelError } from '@/configmodel';
import ConfigCard from '@/components/configforms/ConfigCard.vue';
import ConfigPreset from '@/components/configforms/ConfigPreset.vue';
import ConfigRefPanel from '@/components/configforms/ConfigRefPanel.vue';
import ConfigSaveBar from '@/components/configforms/ConfigSaveBar.vue';

// a row is the view plus its name and whether it's a new one (its name is
// still editable; once saved, the file's table is named by it)
type ViewRow = { key: number; name: string; isNew: boolean; view: ViewConfig };

const modelError = configModelError;
let key = 0;

// the draft is the rows themselves; the save builds the section's body
// from them. A name the file has that the draft lost — deleted, or
// renamed — is the remove list: the form owns the views' tables.
const draft = new SectionDraft<ViewRow[]>(
  'views',
  (m) =>
    m.view_order.flatMap((name) => {
      const v = m.views[name];
      return v ? [{ key: key++, name, isNew: false, view: { ...v } }] : [];
    }),
  (a, b) => JSON.stringify(a.map((v) => [v.name, v.view])) !== JSON.stringify(b.map((v) => [v.name, v.view])),
  (rows) => {
    const names = rows.map((v) => v.name.trim()).filter(Boolean);
    return {
      set: Object.fromEntries(rows.filter((v) => v.name.trim()).map((v) => [v.name.trim(), v.view])),
      remove: (configModel.value?.view_order ?? []).filter((n) => !names.includes(n)),
      order: names,
    };
  },
);

// the rows as the cards hold them
const views = computed({
  get: () => draft.value.value ?? [],
  set: (rows) => {
    if (draft.value.value) draft.value.value = rows;
  },
});

function move(i: number, d: number) {
  const rows = [...views.value];
  const a = rows[i];
  const b = rows[i + d];
  if (!a || !b) return;
  rows[i] = b;
  rows[i + d] = a;
  views.value = rows;
}

function remove(i: number) {
  const rows = [...views.value];
  rows.splice(i, 1);
  views.value = rows;
}

function addBlank() {
  views.value = [...views.value, { key: key++, name: '', isNew: true, view: { layout: 'table', filter: 'is:open', group: '', sort: '' } }];
}

function addPreset(i: number) {
  const p = viewPresets[i];
  if (!p) return;
  const taken = views.value.map((v) => v.name);
  views.value = [...views.value, { key: key++, name: takeUnique(p.value.name, taken), isNew: false, view: { ...p.value.view } }];
}

const valid = computed(() => views.value.every((v) => !v.name.trim() || /^[A-Za-z0-9_-]+$/.test(v.name.trim())));
const state = computed(() => `${views.value.length} view${views.value.length === 1 ? '' : 's'} · ${views.value[0]?.name || '—'} opens`);

function reset() {
  if (configModel.value) draft.reset(configModel.value);
}

async function save() {
  await draft.save();
}

onMounted(() => void draft.load());
</script>

<style scoped>
.vs { display: grid; gap: 1rem; }
h2, p { margin: 0; }
.vs-lead { max-width: 42rem; color: var(--fg-muted); }
.vs-muted { color: var(--fg-muted); }
.vs-tools { display: flex; align-items: center; gap: .75rem; flex-wrap: wrap; }
.vs-item { display: flex; align-items: flex-start; gap: .5rem; }
.vs-order { display: flex; flex-direction: column; gap: .25rem; padding-top: 1rem; }
.vs-btn { flex: none; width: 1.75rem; height: 1.5rem; padding: 0; border: 1px solid var(--border); background: var(--bg); color: var(--fg); font: inherit; font-size: .75rem; line-height: 1; cursor: pointer; }
.vs-btn:hover:not(:disabled) { border-color: var(--fg); }
.vs-btn:disabled { color: var(--fg-subtle); cursor: default; }
.vs-card { flex: 1; min-width: 0; }
.vs-form { display: flex; gap: 1rem; flex-wrap: wrap; }
.vs-field { display: grid; gap: .25rem; font-size: .8125rem; }
.vs-field > span { color: var(--fg-muted); }
.vs-grow { flex: 1; min-width: 12rem; }
.vs-input { padding: .375rem .625rem; border: 1px solid var(--border); background: var(--bg); color: var(--fg); font: inherit; }
.vs-small { max-width: 9rem; }
.vs-input:disabled { color: var(--fg-muted); background: var(--bg-subtle); }
.vs-input:focus { outline: 2px solid var(--fg); outline-offset: 2px; border-color: var(--fg); }
</style>
