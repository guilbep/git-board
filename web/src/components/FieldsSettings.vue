<!-- Settings › Fields: the project's ontology, one card a field — its
     type (the 7, guessed until set), its options in workflow order,
     single, numeric, unit, and its colors and descriptions as rows. Add
     offers the presets; Remove takes the field's tables out. The file's
     other fields keep their lines, comments included. -->
<template>
  <div class="set-panel fs">
    <h2 class="h4">Fields</h2>
    <p class="fs-lead">
      A field is a label namespace: <code>status:ready</code> is the value <code>ready</code> of the field <code>status</code>. Every
      namespace found on the issues is already a field; declare one here to give it an order, a cardinality, a type or colors.
    </p>

    <div class="set-cols">
      <div class="set-main">
        <p v-if="modelError" class="epfl-alert epfl-alert-danger" role="alert">{{ modelError }}</p>

        <!-- no file yet: start from the default, one whole-file save (the raw
             route, exactly what it is for), then the form reads it -->
        <div v-else-if="configModel && !configModel.exists" class="fs-start">
          <p class="fs-lead">
            No project file yet: every <code>ns:value</code> label is a field, in no order. The default declares the statuses,
            priorities, kinds and estimates, the branch names and four views, each explained in a comment.
          </p>
          <p class="fs-startrow">
            <button type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="starting" @click="startFromDefault">
              {{ starting ? 'Writing the default…' : 'Start from the default' }}
            </button>
            <span class="gb-hint">or, in a terminal, <code>git board init</code></span>
          </p>
          <p v-if="startError" class="epfl-alert epfl-alert-danger" role="alert">{{ startError }}</p>
        </div>

        <p v-else-if="!draft.value.value" class="fs-muted">Loading…</p>

        <template v-else>
          <div class="fs-tools">
            <ConfigPreset label="+ Add a field" :presets="fieldPresets" @pick="addPreset" />
            <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" @click="addBlank">Blank field</button>
          </div>

          <ConfigCard
            v-for="f in fields"
            :key="f.key"
            class="fs-card"
            :title="f.name || 'a new field'"
            :removable="true"
            :confirm="`The field ${f.name} and its colors, descriptions and labels go from the file. The issues keep their ${f.name}:… labels, in no order.`"
            @remove="remove(f)"
          >
            <div class="fs-form">
              <label class="fs-field">
                <span>Name</span>
                <input :value="f.name" class="fs-input" :disabled="!f.isNew" spellcheck="false" aria-label="The field's name" @input="f.name = ($event.target as HTMLInputElement).value" />
              </label>
              <label class="fs-field">
                <span>Type</span>
                <select v-model="f.type" class="fs-input fs-small" aria-label="Type">
                  <option value="">guessed</option>
                  <option v-for="t in types" :key="t.id" :value="t.id">{{ t.label }}</option>
                </select>
              </label>
              <label class="fs-field fs-check">
                <input v-model="f.single" type="checkbox" />
                <span>single — one value at a time</span>
              </label>
              <label class="fs-field fs-check">
                <input v-model="f.numeric" type="checkbox" />
                <span>numeric — sorts as numbers</span>
              </label>
              <label v-if="f.type === 'number' || f.numeric" class="fs-field">
                <span>Unit</span>
                <input v-model="f.unit" class="fs-input fs-tiny" placeholder="h" spellcheck="false" aria-label="Unit" />
              </label>
            </div>

            <details class="fs-more" :open="f.options.length > 0">
              <summary>Options, in workflow order <span class="fs-count">{{ f.options.length }}</span></summary>
              <ConfigRows :rows="f.options" add-label="Add an option" @update="f.options = $event" @add="f.options = [...f.options, '']">
                <template #row="{ row, set }">
                  <input :value="row" class="fs-input fs-opt" placeholder="ready" spellcheck="false" :aria-label="`${f.name}'s option`" @input="set(inputValue($event))" />
                </template>
              </ConfigRows>
            </details>

            <details class="fs-more" :open="f.colors.length > 0">
              <summary>Colors <span class="fs-count">{{ f.colors.length }}</span></summary>
              <ConfigRows :rows="f.colors" add-label="Add a color" @update="f.colors = $event" @add="f.colors = [...f.colors, { k: '', v: 'grey' }]">
                <template #row="{ row, set }">
                  <div class="fs-pair">
                    <input :value="row.k" class="fs-input fs-opt" :placeholder="f.options[0] || 'value'" spellcheck="false" aria-label="Value" @input="set({ ...row, k: inputValue($event) })" />
                    <select :value="row.v" class="fs-input fs-tiny" aria-label="Color" @change="set({ ...row, v: inputValue($event) })">
                      <option v-for="c in palette" :key="c" :value="c">{{ c }}</option>
                      <option v-if="row.v && !palette.includes(row.v)" :value="row.v">{{ row.v }}</option>
                    </select>
                  </div>
                </template>
              </ConfigRows>
              <p class="gb-hint">A palette name, or a hex <code>#d73a4a</code> typed over one. A value without one takes the palette in turn.</p>
            </details>

            <details class="fs-more" :open="f.descriptions.length > 0">
              <summary>Descriptions <span class="fs-count">{{ f.descriptions.length }}</span></summary>
              <ConfigRows :rows="f.descriptions" add-label="Add a description" @update="f.descriptions = $event" @add="f.descriptions = [...f.descriptions, { k: '', v: '' }]">
                <template #row="{ row, set }">
                  <div class="fs-pair">
                    <input :value="row.k" class="fs-input fs-opt" spellcheck="false" aria-label="Value" @input="set({ ...row, k: inputValue($event) })" />
                    <input :value="row.v" class="fs-input fs-desc" placeholder="what the value means" aria-label="Description" @input="set({ ...row, v: inputValue($event) })" />
                  </div>
                </template>
              </ConfigRows>
              <p class="gb-hint">What the value's label says on GitHub, 100 characters at most.</p>
            </details>

            <details v-if="f.type === 'select' || !f.type" class="fs-more" :open="f.project.length > 0">
              <summary>GitHub Project names <span class="fs-count">{{ f.project.length }}</span></summary>
              <ConfigRows :rows="f.project" add-label="Add a name" @update="f.project = $event" @add="f.project = [...f.project, { k: '', v: '' }]">
                <template #row="{ row, set }">
                  <div class="fs-pair">
                    <input :value="row.k" class="fs-input fs-opt" placeholder="wont" spellcheck="false" aria-label="Value" @input="set({ ...row, k: inputValue($event) })" />
                    <input :value="row.v" class="fs-input fs-opt" placeholder="Won't" aria-label="The Project's name" @input="set({ ...row, v: inputValue($event) })" />
                  </div>
                </template>
              </ConfigRows>
              <p class="gb-hint">The Project's option name for a value, when it doesn't slug to the value.</p>
            </details>
          </ConfigCard>

          <ConfigSaveBar
            section="fields"
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


      </div>

      <ConfigRefPanel :sections="['fields', 'types', 'colors']" title="a field's keys, the types" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { getConfigFile, saveConfig } from '@/api/board';
import { fieldPresets, takeUnique } from '@/configpresets';
import { SectionDraft, configModel, configModelError, loadConfigModel, inputValue } from '@/configmodel';
import { message } from '@/issue';
import ConfigCard from '@/components/configforms/ConfigCard.vue';
import ConfigPreset from '@/components/configforms/ConfigPreset.vue';
import ConfigRefPanel from '@/components/configforms/ConfigRefPanel.vue';
import ConfigRows from '@/components/configforms/ConfigRows.vue';
import ConfigSaveBar from '@/components/configforms/ConfigSaveBar.vue';

const types = [
  { id: 'select', label: 'select — ordered values' },
  { id: 'iteration', label: 'iteration — spans of dates, sprints' },
  { id: 'person', label: 'person — forge logins, @me' },
  { id: 'number', label: 'number — with a unit' },
  { id: 'date', label: 'date — YYYY-MM-DD' },
  { id: 'issue', label: 'issue — another issue' },
  { id: 'text', label: 'text — free' },
];
const palette = ['grey', 'blue', 'purple', 'amber', 'orange', 'green', 'red', 'teal', 'pink'];

// the field as the card holds it: its maps as pairs (the rows' source of
// truth), the file's Record read back at the edges
type FieldRow = {
  key: number;
  name: string;
  isNew: boolean;
  single: boolean;
  numeric: boolean;
  type: string;
  unit: string;
  options: string[];
  colors: Pair[];
  descriptions: Pair[];
  project: Pair[];
};
type Pair = { k: string; v: string };

const modelError = configModelError;
let key = 0;

const toPairs = (m?: Record<string, string>): Pair[] => Object.entries(m ?? {}).map(([k, v]) => ({ k, v }));
const kvOf = (rows: Pair[]): Record<string, string> | undefined => {
  const out = Object.fromEntries(rows.filter((r) => r.k.trim() && r.v.trim()).map((r) => [r.k.trim(), r.v]));
  return Object.keys(out).length ? out : undefined;
};

// the draft is the rows; the remove list is the names the file has that
// the draft lost — deleted, or renamed
const draft = new SectionDraft<FieldRow[]>(
  'fields',
  (m) =>
    Object.entries(m.fields).map(([name, f]) => ({
      key: key++,
      name,
      isNew: false,
      single: f.single ?? false,
      numeric: f.numeric ?? false,
      type: f.type ?? '',
      unit: f.unit ?? '',
      options: [...(f.options ?? [])],
      colors: toPairs(f.colors),
      descriptions: toPairs(f.descriptions),
      project: toPairs(f.project),
    })),
  (a, b) => JSON.stringify(a.map(rowOf)) !== JSON.stringify(b.map(rowOf)),
  (rows) => {
    const names = rows.map((f) => f.name.trim()).filter(Boolean);
    const set: Record<string, unknown> = {};
    for (const f of rows) {
      if (!f.name.trim()) continue;
      set[f.name.trim()] = {
        single: f.single ?? false,
        numeric: f.numeric ?? false,
        type: f.type || undefined,
        unit: f.unit || undefined,
        options: f.options.filter((o) => o.trim()),
        colors: kvOf(f.colors),
        descriptions: kvOf(f.descriptions),
        project: kvOf(f.project),
      };
    }
    return { set, remove: Object.keys(configModel.value?.fields ?? {}).filter((n) => !names.includes(n)) };
  },
);

const rowOf = (f: FieldRow) => [f.name, f.single, f.numeric, f.type, f.unit, f.options, f.colors, f.descriptions, f.project];

const fields = computed({
  get: () => draft.value.value ?? [],
  set: (rows) => {
    if (draft.value.value) draft.value.value = rows;
  },
});

function remove(f: FieldRow) {
  fields.value = fields.value.filter((x) => x !== f);
}

function addBlank() {
  fields.value = [...fields.value, { key: key++, name: '', isNew: true, single: false, numeric: false, type: '', unit: '', options: [], colors: [], descriptions: [], project: [] }];
}

function addPreset(i: number) {
  const p = fieldPresets[i];
  if (!p) return;
  const taken = fields.value.map((f) => f.name);
  const f = p.value.field;
  fields.value = [
    ...fields.value,
    {
      key: key++,
      name: takeUnique(p.value.name, taken),
      isNew: false,
      single: f.single ?? false,
      numeric: f.numeric ?? false,
      type: f.type ?? '',
      unit: f.unit ?? '',
      options: [...(f.options ?? [])],
      colors: toPairs(f.colors),
      descriptions: toPairs(f.descriptions),
      project: toPairs(f.project),
    },
  ];
}

const valid = computed(() => fields.value.every((f) => !f.name.trim() || /^[A-Za-z0-9_-]+$/.test(f.name.trim())));
const state = computed(() => `${fields.value.length} field${fields.value.length === 1 ? '' : 's'} · ${fields.value.reduce((n, f) => n + f.options.length, 0)} options`);

function reset() {
  if (configModel.value) draft.reset(configModel.value);
}

// startFromDefault writes the whole default file through the raw route —
// the one whole-file save the forms make — then reads it back
const starting = ref(false);
const startError = ref('');

async function startFromDefault() {
  starting.value = true;
  startError.value = '';
  try {
    const file = await getConfigFile();
    if (!file.default) throw new Error('The default project file is not available.');
    await saveConfig(file.default, file.hash);
    await loadConfigModel();
    if (configModel.value) draft.reset(configModel.value);
  } catch (e) {
    startError.value = message(e);
  } finally {
    starting.value = false;
  }
}

async function save() {
  await draft.save();
}

onMounted(() => void draft.load());
</script>

<style scoped>
.fs { display: grid; gap: 1rem; container-type: inline-size; }
/* the forms left, the reference right, like the old editor; one column when narrow */
.set-cols { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); gap: 1rem; align-items: start; }
.set-main { display: grid; gap: 1rem; min-width: 0; }
@container (max-width: 52rem) { .set-cols { grid-template-columns: minmax(0, 1fr); } }
h2, p { margin: 0; }
.fs-lead { max-width: 42rem; color: var(--fg-muted); }
.fs-muted { color: var(--fg-muted); }
.fs-start { display: grid; gap: .75rem; padding: 1.25rem; border: 1px solid var(--border); border-left: 3px solid var(--primary); background: var(--bg); }
.fs-startrow { display: flex; align-items: center; gap: .75rem; }
.fs-tools { display: flex; align-items: center; gap: .75rem; flex-wrap: wrap; }
.fs-card { border-left-color: var(--primary); }
.fs-form { display: flex; gap: 1rem; flex-wrap: wrap; align-items: end; }
.fs-field { display: grid; gap: .25rem; font-size: .8125rem; }
.fs-field > span { color: var(--fg-muted); }
.fs-check { grid-template-columns: auto 1fr; align-items: center; display: flex; gap: .375rem; padding-bottom: .375rem; }
.fs-input { padding: .375rem .625rem; border: 1px solid var(--border); background: var(--bg); color: var(--fg); font: inherit; }
.fs-input:disabled { color: var(--fg-muted); background: var(--bg-subtle); }
.fs-input:focus { outline: 2px solid var(--fg); outline-offset: 2px; border-color: var(--fg); }
.fs-small { max-width: 14rem; }
.fs-tiny { max-width: 4.5rem; }
.fs-opt { width: 10rem; }
.fs-desc { flex: 1; min-width: 12rem; }
.fs-pair { display: flex; gap: .5rem; }
.fs-more { display: grid; gap: .5rem; }
.fs-more summary { cursor: pointer; font-size: .875rem; font-weight: 700; color: var(--fg-muted); }
.fs-more summary:hover { color: var(--fg); }
.fs-more[open] summary { margin-bottom: .25rem; }
.fs-count { display: inline-block; margin-left: .375rem; padding: 0 .375rem; border: 1px solid var(--border); font-weight: 400; font-size: .75rem; color: var(--fg-muted); }
</style>
