<!-- Settings › Fields: the project's ontology, one card a field. The
     values are chips — drag to reorder, Enter to add, paste a list — the
     cardinality is "an issue can have" (one value / several), and the
     per-value details (color, description, GitHub Project name) sit in
     one table keyed by value. Add offers the presets; Remove takes the
     field's tables out. The file's other fields keep their lines,
     comments included. -->
<template>
  <div class="set-panel fs">
    <h2 class="h4">Fields</h2>
    <p class="fs-lead">
      A field is a label namespace: <code>status:ready</code> is the value <code>ready</code> of the field <code>status</code>. Every
      namespace found on the issues is already a field; declare one here to give it an order, a cardinality, a type or colors.
    </p>

    <div class="set-cols">
      <div class="set-forms">
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
            <span class="gb-grow" />
            <router-link class="gb-linkbtn" to="/settings/advanced">Edit the file instead</router-link>
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
            <template #head>
              <span class="fs-ns">fields.</span>
              <input
                :value="f.name"
                class="fs-name"
                :disabled="!f.isNew"
                spellcheck="false"
                aria-label="The field's name"
                @input="f.name = ($event.target as HTMLInputElement).value"
              />
              <span class="fs-sum">{{ summary(f) }}</span>
              <span v-if="changed(f)" class="fs-dirty">Unsaved changes</span>
              <span class="fs-mode" role="group" aria-label="Edit as a form or as TOML">
                <button type="button" class="fs-modebtn" :class="{ 'fs-mode-on': !tomlOpen[f.key] }" @click="closeToml(f)">Form</button>
                <button type="button" class="fs-modebtn" :class="{ 'fs-mode-on': tomlOpen[f.key] }" @click="openToml(f)">TOML</button>
              </span>
            </template>

            <div v-if="tomlOpen[f.key]" class="fs-toml">
              <textarea
                class="fs-tomledit"
                :value="tomlText[f.key]"
                spellcheck="false"
                rows="10"
                :aria-label="`The TOML of ${f.name || 'the new field'}`"
                @input="tomlInput(f, ($event.target as HTMLTextAreaElement).value)"
              />
              <p v-if="tomlError[f.key]" class="fs-tomlerr" role="alert">{{ tomlError[f.key] }}</p>
              <p v-else class="gb-hint">This card's table only — it applies as you type. Comments are lost when you switch back to the form.</p>
            </div>
            <div v-else class="fs-body">
              <div class="fs-block">
                <span class="fs-label">Values <em>— drag to reorder; left is first in the workflow</em></span>
                <div class="fs-chips">
                  <span
                    v-for="(o, i) in f.options"
                    :key="`${o}-${i}`"
                    class="fs-chip"
                    draggable="true"
                    :class="{ 'fs-chip-hold': dragFrom === i }"
                    @dragstart="dragFrom = i"
                    @dragover.prevent
                    @dragend="dragFrom = null"
                    @drop="dropAt(f, i)"
                  >
                    <span class="fs-dot" :class="dotClass(f, o)" :style="dotStyle(f, o)" aria-hidden="true" />
                    {{ o }}
                    <button type="button" class="fs-chip-x" :aria-label="`Remove ${o}`" @click="removeOption(f, i)">×</button>
                  </span>
                  <input
                    class="fs-add"
                    :placeholder="f.type === 'number' || f.numeric ? 'Type a number, press Enter' : 'Type a value, press Enter'"
                    spellcheck="false"
                    :aria-label="`Add a value to ${f.name}`"
                    @keydown="chipKey(f, $event)"
                    @paste="chipPaste(f, $event)"
                  />
                </div>
                <p v-if="f.type === 'number' || f.numeric" class="gb-hint">
                  All values are numbers — sorted by value, filterable by range. Paste a list too: <code>0.5, 1, 2, 4, 8</code>.
                </p>
                <p v-else class="gb-hint">Paste a list too: <code>todo, doing, done</code> or one per line.</p>
              </div>

              <div class="fs-form">
                <label class="fs-field">
                  <span>Type</span>
                  <select v-model="f.type" class="fs-input fs-small" aria-label="Type">
                    <option value="">guessed</option>
                    <option v-for="t in types" :key="t.id" :value="t.id">{{ t.label }}</option>
                  </select>
                </label>
                <fieldset class="fs-cardinality">
                  <legend>An issue can have</legend>
                  <label class="fs-radio"><input v-model="f.single" type="radio" :value="true" /> one value</label>
                  <label class="fs-radio"><input v-model="f.single" type="radio" :value="false" /> several values</label>
                </fieldset>
                <label class="fs-check">
                  <input v-model="f.numeric" type="checkbox" />
                  <span>numeric — sorts as numbers</span>
                </label>
                <label v-if="f.type === 'number' || f.numeric" class="fs-field">
                  <span>Unit</span>
                  <input v-model="f.unit" class="fs-input fs-tiny" placeholder="h" spellcheck="false" aria-label="Unit shown after the number" />
                </label>
              </div>

              <details class="fs-more" :open="detailKeys(f).length > 0">
                <summary>
                  Per-value details <span class="fs-count">{{ detailKeys(f).length }}</span>
                  <span class="fs-morehint">colors, descriptions, GitHub Project names</span>
                </summary>
                <div v-if="detailKeys(f).length" class="fs-tablewrap">
                  <table class="fs-table">
                    <thead>
                      <tr>
                        <th>Value</th>
                        <th>Color</th>
                        <th>Description</th>
                        <th>GitHub Project name</th>
                      </tr>
                    </thead>
                    <tbody>
                      <tr v-for="k in detailKeys(f)" :key="k">
                        <td class="fs-tvalue">{{ k }}</td>
                        <td>
                          <select
                            :value="pairOf(f, 'colors', k)"
                            class="fs-input fs-tiny"
                            :aria-label="`Color for ${k}`"
                            @change="setPair(f, 'colors', k, inputValue($event))"
                          >
                            <option value="">—</option>
                            <option v-for="c in palette" :key="c" :value="c">{{ c }}</option>
                            <option v-if="pairOf(f, 'colors', k) && !palette.includes(pairOf(f, 'colors', k))" :value="pairOf(f, 'colors', k)">
                              {{ pairOf(f, 'colors', k) }}
                            </option>
                          </select>
                        </td>
                        <td>
                          <input
                            :value="pairOf(f, 'descriptions', k)"
                            class="fs-cell"
                            placeholder="what the value means"
                            spellcheck="false"
                            :aria-label="`Description for ${k}`"
                            @input="setPair(f, 'descriptions', k, inputValue($event))"
                          />
                        </td>
                        <td>
                          <input
                            :value="pairOf(f, 'project', k)"
                            class="fs-cell"
                            placeholder="if different"
                            spellcheck="false"
                            :aria-label="`GitHub Project name for ${k}`"
                            @input="setPair(f, 'project', k, inputValue($event))"
                          />
                        </td>
                      </tr>
                    </tbody>
                  </table>
                </div>
                <p v-else class="gb-hint">No values yet: the details come once the values do.</p>
              </details>
            </div>
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
import { computed, onMounted, ref, watch } from 'vue';
import { getConfigFile, saveConfig } from '@/api/board';
import { fieldPresets, takeUnique } from '@/configpresets';
import { fieldToToml, tomlToField, type FieldToml } from '@/fieldtoml';
import { SectionDraft, configModel, configModelError, loadConfigModel, inputValue } from '@/configmodel';
import { message } from '@/issue';
import ConfigCard from '@/components/configforms/ConfigCard.vue';
import ConfigPreset from '@/components/configforms/ConfigPreset.vue';
import ConfigRefPanel from '@/components/configforms/ConfigRefPanel.vue';
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
type PairMap = 'colors' | 'descriptions' | 'project';

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

// the file's fields as the cards hold them, for the per-card change badge
const baseRows = computed(() => (configModel.value ? draft.from(configModel.value) : []));

function changed(f: FieldRow): boolean {
  const base = baseRows.value.find((x) => x.name === f.name);
  if (!base) return true;
  return JSON.stringify(rowOf(f)) !== JSON.stringify(rowOf(base));
}

// what the card's head says the field is, in the page's words
function summary(f: FieldRow): string {
  const card = f.single ? 'one per issue' : 'several per issue';
  if (f.type === 'number' || (f.numeric && f.options.length > 0)) return `Numbers · ${f.unit ? `in ${f.unit}` : 'no unit'} · ${card}`;
  if (f.type === 'text') return `Free text · ${card}`;
  if (f.type === 'date') return `Dates · ${card}`;
  if (f.type === 'person') return `Forge logins · ${card}`;
  if (f.type === 'iteration') return `Spans of dates, sprints · ${card}`;
  if (f.type === 'issue') return `Another issue · ${card}`;
  if (f.options.length > 0) return `${f.options.length} values, in workflow order · ${card}`;
  return 'Guessed from the labels';
}

// the values as chips: drag to reorder, Enter (or a comma) adds, paste
// splits a list; the chip's dot takes the value's color when it has one
const dragFrom = ref<number | null>(null);

function dropAt(f: FieldRow, i: number) {
  const from = dragFrom.value;
  dragFrom.value = null;
  if (from === null || from === i) return;
  const options = [...f.options];
  const moved = options[from];
  if (moved === undefined) return;
  options.splice(from, 1);
  options.splice(i, 0, moved);
  f.options = options;
}

function removeOption(f: FieldRow, i: number) {
  f.options = f.options.filter((_, j) => j !== i);
}

function chipKey(f: FieldRow, e: KeyboardEvent) {
  if (e.key !== 'Enter' && e.key !== ',') return;
  e.preventDefault();
  addChips(f, e.target as HTMLInputElement);
}

function chipPaste(f: FieldRow, e: ClipboardEvent) {
  const text = e.clipboardData?.getData('text') ?? '';
  if (!/[,\n]/.test(text)) return; // a plain word: let the input take it
  e.preventDefault();
  const el = e.target as HTMLInputElement;
  addWords(f, `${(el as HTMLInputElement).value},${text}`);
  el.value = '';
}

function addChips(f: FieldRow, el: HTMLInputElement) {
  addWords(f, el.value);
  el.value = '';
}

function addWords(f: FieldRow, text: string) {
  const words = text.split(/[,\n]/).map((s) => s.trim()).filter((s) => s && !f.options.includes(s));
  if (words.length) f.options = [...f.options, ...words];
}

// the card's other rendering: a [fields.<name>] snippet, applied as it is
// typed. While it parses, the snippet is the source of truth — a key
// absent from it leaves the field, exactly as the file would; while it
// does not, the row keeps its last good state and Save is held.
const tomlOpen = ref<Record<number, boolean>>({});
const tomlText = ref<Record<number, string>>({});
const tomlError = ref<Record<number, string | undefined>>({});

const rowToToml = (f: FieldRow): FieldToml => ({
  single: f.single,
  numeric: f.numeric,
  type: f.type || undefined,
  unit: f.unit || undefined,
  options: f.options,
  colors: kvOf(f.colors),
  descriptions: kvOf(f.descriptions),
  project: kvOf(f.project),
});

function openToml(f: FieldRow) {
  tomlOpen.value[f.key] = true;
  tomlText.value[f.key] = fieldToToml(f.name, rowToToml(f));
  tomlError.value[f.key] = undefined;
}

function closeToml(f: FieldRow) {
  tomlOpen.value[f.key] = false;
}

function tomlInput(f: FieldRow, text: string) {
  tomlText.value[f.key] = text;
  const r = tomlToField(text);
  if (!r.ok) {
    tomlError.value[f.key] = r.error;
    return;
  }
  tomlError.value[f.key] = undefined;
  applyToml(f, r.name, r.field);
}

function applyToml(f: FieldRow, name: string | undefined, t: FieldToml) {
  if (name) f.name = name;
  f.single = t.single ?? false;
  f.numeric = t.numeric ?? false;
  f.type = t.type ?? '';
  f.unit = t.unit ?? '';
  f.options = t.options ? [...t.options] : [];
  f.colors = toPairs(t.colors);
  f.descriptions = toPairs(t.descriptions);
  f.project = toPairs(t.project);
}

// a reload or a save rebuilds the rows: the snippets go with the old ones
watch(
  () => draft.base.value,
  () => {
    tomlOpen.value = {};
    tomlText.value = {};
    tomlError.value = {};
  },
);

// the per-value details: one table keyed by value — the options in their
// order, then the pair keys the options lost; a cell emptied removes its
// pair
function detailKeys(f: FieldRow): string[] {
  const keys: string[] = [];
  for (const k of [...f.options, ...f.colors.map((r) => r.k), ...f.descriptions.map((r) => r.k), ...f.project.map((r) => r.k)]) {
    if (!keys.includes(k)) keys.push(k);
  }
  return keys;
}

function pairOf(f: FieldRow, map: PairMap, k: string): string {
  return f[map].find((r) => r.k === k)?.v ?? '';
}

function setPair(f: FieldRow, map: PairMap, k: string, v: string) {
  const rows = f[map];
  const r = rows.find((x) => x.k === k);
  if (!v.trim()) {
    if (r) f[map] = rows.filter((x) => x !== r);
  } else if (r && r.v !== v) {
    f[map] = rows.map((x) => (x === r ? { k, v } : x));
  } else if (!r) {
    f[map] = [...rows, { k, v }];
  }
}

function dotClass(f: FieldRow, o: string): string {
  const c = pairOf(f, 'colors', o);
  return palette.includes(c) ? `gb-c-${c}` : '';
}

function dotStyle(f: FieldRow, o: string): Record<string, string> | undefined {
  const c = pairOf(f, 'colors', o);
  return c && !palette.includes(c) ? { '--c-dot': c } : undefined;
}

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

const hasTomlError = computed(() => fields.value.some((f) => tomlError.value[f.key]));
const valid = computed(() => {
  const names = fields.value.map((f) => f.name.trim()).filter(Boolean);
  return names.every((n) => /^[A-Za-z0-9_-]+$/.test(n)) && new Set(names).size === names.length && !hasTomlError.value;
});
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
.fs { display: grid; gap: .75rem; container-type: inline-size; }
/* the forms left, the reference right, like the old editor; one column when narrow */
.set-cols { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); gap: .75rem; align-items: start; }
.set-forms { display: grid; gap: .75rem; min-width: 0; }
@container (max-width: 52rem) { .set-cols { grid-template-columns: minmax(0, 1fr); } }
h2, p { margin: 0; }
.fs-lead { max-width: 42rem; color: var(--fg-muted); }
.fs-muted { color: var(--fg-muted); }
.fs-start { display: grid; gap: .75rem; padding: 1.25rem; border: 1px solid var(--border); border-left: 3px solid var(--primary); background: var(--bg); }
.fs-startrow { display: flex; align-items: center; gap: .75rem; }
.fs-tools { display: flex; align-items: center; gap: .75rem; flex-wrap: wrap; }
.fs-card { border-left-color: var(--primary); }
/* the card's head: fields.<name>, what the field is, the unsaved mark */
.fs-ns { font-size: .8125rem; color: var(--fg-muted); }
.fs-name { width: 9rem; min-height: 2rem; padding: 0 .5rem; border: 1px solid var(--border); background: var(--bg); color: var(--fg); font-family: var(--font-mono); font-size: 1rem; font-weight: 700; }
.fs-name:disabled { border-color: transparent; background: none; padding-left: 0; }
.fs-name:focus { outline: 2px solid var(--fg); outline-offset: 2px; }
.fs-sum { color: var(--fg-muted); font-size: .8125rem; }
.fs-dirty { font-size: .75rem; font-weight: 700; color: var(--danger); }
/* the card's two renderings: the form and this field's TOML */
.fs-mode { display: inline-flex; border: 1px solid var(--border); }
.fs-modebtn { min-height: 1.625rem; padding: 0 .5rem; border: 0; background: none; color: var(--fg-muted); font: inherit; font-size: .75rem; cursor: pointer; }
.fs-modebtn + .fs-modebtn { border-left: 1px solid var(--border); }
.fs-modebtn:hover { color: var(--fg); }
.fs-mode-on { background: var(--fg); color: var(--bg); font-weight: 700; }
.fs-toml { display: grid; gap: .375rem; }
.fs-tomledit { padding: .5rem .625rem; border: 1px solid var(--border); background: var(--bg); color: var(--fg); font-family: var(--font-mono); font-size: .8125rem; line-height: 1.5; resize: vertical; white-space: pre; }
.fs-tomledit:focus { outline: 2px solid var(--fg); outline-offset: 2px; }
.fs-tomlerr { color: var(--danger); font-size: .8125rem; }
/* the body: values as chips, the cardinality, the details' table */
.fs-body { display: grid; gap: .75rem; }
.fs-block { display: grid; gap: .375rem; }
.fs-label { font-weight: 700; font-size: .875rem; }
.fs-label em { font-style: normal; font-weight: 400; color: var(--fg-muted); }
.fs-chips { display: flex; flex-wrap: wrap; align-items: center; gap: .375rem; min-height: 2.25rem; padding: .375rem .5rem; border: 1px solid var(--border); background: var(--bg); }
.fs-chips:focus-within { border-color: var(--fg); }
.fs-chip { display: inline-flex; align-items: center; gap: .375rem; padding: 0 .125rem 0 .5rem; min-height: 1.625rem; border: 1px solid var(--border); background: var(--bg-subtle); font-family: var(--font-mono); font-size: .8125rem; cursor: grab; }
.fs-chip:active { cursor: grabbing; }
.fs-chip-hold { opacity: .4; }
.fs-dot { width: .625rem; height: .625rem; background: var(--c-dot, var(--border-strong)); }
.fs-chip-x { width: 1.25rem; border: 0; background: none; color: var(--fg-muted); font: inherit; cursor: pointer; }
.fs-chip-x:hover { color: var(--danger); }
.fs-add { flex: 1 1 9rem; min-height: 1.625rem; border: 0; background: none; color: var(--fg); font: inherit; font-size: .8125rem; }
.fs-form { display: flex; gap: .75rem; flex-wrap: wrap; align-items: end; }
.fs-field { display: grid; gap: .25rem; font-size: .8125rem; }
.fs-field > span { color: var(--fg-muted); }
.fs-check { display: flex; gap: .375rem; align-items: center; padding-bottom: .375rem; font-size: .8125rem; }
.fs-input { padding: .375rem .625rem; border: 1px solid var(--border); background: var(--bg); color: var(--fg); font: inherit; }
.fs-input:focus { outline: 2px solid var(--fg); outline-offset: 2px; border-color: var(--fg); }
.fs-small { max-width: 14rem; }
.fs-tiny { max-width: 4.5rem; }
.fs-cardinality { display: flex; gap: .375rem; align-items: center; margin: 0; padding: 0; border: 0; }
.fs-cardinality legend { padding: 0; margin-bottom: .25rem; font-size: .8125rem; font-weight: 700; color: var(--fg-muted); }
.fs-radio { display: inline-flex; align-items: center; gap: .375rem; min-height: 2rem; padding: 0 .625rem; border: 1px solid var(--border); cursor: pointer; font-size: .8125rem; }
.fs-radio:has(input:checked) { border-color: var(--fg); background: var(--fg); color: var(--bg); font-weight: 700; }
.fs-more { display: grid; gap: .5rem; }
.fs-more summary { cursor: pointer; font-size: .875rem; font-weight: 700; color: var(--fg-muted); }
.fs-more summary:hover { color: var(--fg); }
.fs-more[open] summary { margin-bottom: .25rem; }
.fs-morehint { font-weight: 400; color: var(--fg-muted); }
.fs-count { display: inline-block; margin-left: .375rem; padding: 0 .375rem; border: 1px solid var(--border); font-weight: 400; font-size: .75rem; color: var(--fg-muted); }
.fs-tablewrap { overflow-x: auto; border: 1px solid var(--border); }
.fs-table { width: 100%; min-width: 34rem; border-collapse: collapse; font-size: .8125rem; }
.fs-table th { padding: .375rem .5rem; border-bottom: 1px solid var(--border); background: var(--bg-subtle); text-align: left; font-size: .75rem; color: var(--fg-muted); }
.fs-table td { padding: .25rem .5rem; border-bottom: 1px solid var(--border); vertical-align: middle; }
.fs-tvalue { font-family: var(--font-mono); font-weight: 700; }
.fs-cell { width: 100%; box-sizing: border-box; min-height: 1.75rem; padding: .25rem .5rem; border: 1px solid var(--border); background: var(--bg); color: var(--fg); font: inherit; font-size: .8125rem; }
.fs-cell:focus { outline: 2px solid var(--fg); outline-offset: 2px; }
</style>
