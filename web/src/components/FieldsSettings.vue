<!-- Settings › Fields: the project's ontology — a list of every field on
     the left, one field's editor on the right, each field at its own
     address (#/settings/fields/<name>). The list is the file's order:
     drag a row, or Alt+↑/↓ on it, and the save writes the tables back in
     that order. The editor's values are chips — drag to reorder, Enter to
     add, paste a list — the cardinality is "an issue can have" (one value
     / several), and the per-value details (color, description, GitHub
     Project name) sit in one table keyed by value. Form | TOML switches
     the editor to the field's own table. The file's other fields keep
     their lines, comments included. -->
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
          <div class="fl-work">
            <div class="fl-list">
              <p class="fl-listhead">
                {{ fields.length }} field{{ fields.length === 1 ? '' : 's' }} <em>— drag a row, or Alt+↑/↓, to set the order on issues</em>
              </p>
              <ol class="fl-rows">
                <li v-for="(f, i) in orderedRows" :key="f.key">
                  <router-link
                    class="fl-row"
                    :class="{ 'fl-row-sel': f.key === selectedKey, 'fl-row-hold': listFrom === i }"
                    :to="`/settings/fields/${f.name}`"
                    :aria-current="f.key === selectedKey ? 'true' : undefined"
                    draggable="true"
                    @dragstart="listFrom = i"
                    @dragover.prevent
                    @dragend="listFrom = null"
                    @drop="dropRow(i)"
                    @keydown.alt.up.prevent="moveRow(f, -1)"
                    @keydown.alt.down.prevent="moveRow(f, 1)"
                  >
                    <span class="fl-line1">
                      <span class="fl-grip" aria-hidden="true">⋮⋮</span>
                      <span class="fl-nm">{{ f.name || 'a new field' }}</span>
                      <span v-if="changed(f)" class="fl-dot" title="Unsaved changes" />
                      <span class="fl-sum">{{ summary(f) }}</span>
                    </span>
                    <span class="fl-values">{{ f.options.join(' → ') }}</span>
                  </router-link>
                </li>
              </ol>
              <p class="fl-addrow">
                <ConfigPreset label="+ Add a field" :presets="fieldPresets" @pick="addPreset" />
                <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" @click="addBlank">Blank field</button>
                <span class="gb-grow" />
                <router-link class="gb-linkbtn" to="/settings/advanced">Edit the file instead</router-link>
              </p>
            </div>

            <div v-if="selected" class="fl-ed">
              <ConfigCard
                :key="selected.key"
                class="fs-card"
                :title="selected.name || 'a new field'"
                :removable="true"
                :confirm="removeConfirm"
                @remove="remove(selected)"
              >
                <template #head>
                  <span class="fs-ns">fields.</span>
                  <input
                    ref="nameInput"
                    :value="selected.name"
                    class="fs-name"
                    :disabled="!selected.isNew"
                    spellcheck="false"
                    aria-label="The field's name"
                    @input="selected.name = ($event.target as HTMLInputElement).value"
                  />
                  <span class="fs-sum">{{ summary(selected) }}</span>
                  <span v-if="changed(selected)" class="fs-dirty">Unsaved changes</span>
                  <span class="fs-mode" role="group" aria-label="Edit as a form or as TOML">
                    <button type="button" class="fs-modebtn" :class="{ 'fs-mode-on': !tomlOpen[selected.key] }" @click="closeToml(selected)">Form</button>
                    <button type="button" class="fs-modebtn" :class="{ 'fs-mode-on': tomlOpen[selected.key] }" @click="openToml(selected)">TOML</button>
                  </span>
                </template>

                <div v-if="tomlOpen[selected.key]" class="fs-toml">
                  <textarea
                    class="fs-tomledit"
                    :value="tomlText[selected.key]"
                    spellcheck="false"
                    rows="10"
                    :aria-label="`The TOML of ${selected.name || 'the new field'}`"
                    @input="tomlInput(selected, ($event.target as HTMLTextAreaElement).value)"
                  />
                  <p v-if="tomlError[selected.key]" class="fs-tomlerr" role="alert">{{ tomlError[selected.key] }}</p>
                  <p v-else class="gb-hint">This field's table only — it applies as you type. Comments are lost when you switch back to the form.</p>
                </div>
                <div v-else class="fs-body">
                <div v-if="selected.type === 'text'" class="fs-block">
                  <span class="fs-label">Values</span>
                  <p class="gb-hint">
                    Free text: an issue writes whatever the field holds — no values to list, nothing to order. The values come back if
                    the cardinality goes back to one or several.
                  </p>
                </div>
                <div v-else class="fs-block">
                    <span class="fs-label">Values <em>— {{ numbers(selected) ? 'sorted by value' : 'drag to reorder; left is first in the workflow' }}</em></span>
                    <div class="fs-chips">
                      <span
                        v-for="(o, i) in selected.options"
                        :key="`${o}-${i}`"
                        class="fs-chip"
                        :class="{ 'fs-chip-hold': dragFrom === i, 'fs-chip-fixed': numbers(selected) }"
                        :draggable="numbers(selected) ? 'false' : 'true'"
                        @dragstart="dragFrom = i"
                        @dragover.prevent
                        @dragend="dragFrom = null"
                        @drop="dropAt(selected, i)"
                      >
                        <span class="fs-dot" :class="dotClass(selected, o)" :style="dotStyle(selected, o)" aria-hidden="true" />
                        {{ o }}
                        <button type="button" class="fs-chip-x" :aria-label="`Remove ${o}`" @click="removeOption(selected, i)">×</button>
                      </span>
                      <input
                        class="fs-add"
                        :placeholder="numbers(selected) ? 'Type a number, press Enter' : 'Type a value, press Enter'"
                        spellcheck="false"
                        :aria-label="`Add a value to ${selected.name}`"
                        @keydown="chipKey(selected, $event)"
                        @paste="chipPaste(selected, $event)"
                      />
                    </div>
                    <p v-if="numbers(selected)" class="gb-hint">
                      All values are numbers — sorted by value, filterable by range. Paste a list too: <code>0.5, 1, 2, 4, 8</code>.
                    </p>
                    <p v-else class="gb-hint">Paste a list too: <code>todo, doing, done</code> or one per line.</p>
                  </div>

                  <div class="fs-form">
                    <fieldset class="fs-cardinality">
                      <legend>An issue can have</legend>
                      <label class="fs-radio"><input type="radio" :checked="cardOf(selected) === 'one'" @change="setCard(selected, 'one')" /> one value</label>
                      <label class="fs-radio"><input type="radio" :checked="cardOf(selected) === 'many'" @change="setCard(selected, 'many')" /> several values</label>
                      <label class="fs-radio"><input type="radio" :checked="cardOf(selected) === 'text'" @change="setCard(selected, 'text')" /> any text, no list</label>
                    </fieldset>
                    <template v-if="numbers(selected)">
                      <label class="fs-field">
                        <span>Unit</span>
                        <input v-model="selected.unit" class="fs-input fs-tiny" :placeholder="selected.name === 'est' ? 'h' : ''" spellcheck="false" aria-label="Unit shown after the number" />
                      </label>
                      <button type="button" class="gb-linkbtn" @click="selected.type = 'select'">Treat them as labels instead</button>
                    </template>
                    <label v-else class="fs-field">
                      <span>Type</span>
                      <select :value="selected.type" class="fs-input fs-small" aria-label="Type" @change="selected.type = inputValue($event)">
                        <option value="">guessed — {{ inferLabel(selected) }}</option>
                        <option v-for="t in types" :key="t.id" :value="t.id">{{ t.label }}</option>
                      </select>
                    </label>
                  </div>

                  <details class="fs-more" :open="detailKeys(selected).length > 0">
                    <summary>
                      Per-value details <span class="fs-morehint">{{ detailsSummary(selected) }}</span>
                    </summary>
                    <div v-if="detailKeys(selected).length" class="fs-tablewrap">
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
                          <tr v-for="k in detailKeys(selected)" :key="k">
                            <td class="fs-tvalue">{{ k }}</td>
                            <td>
                              <select
                                :value="pairOf(selected, 'colors', k)"
                                class="fs-input fs-tiny"
                                :aria-label="`Color for ${k}`"
                                @change="setPair(selected, 'colors', k, inputValue($event))"
                              >
                                <option value="">—</option>
                                <option v-for="c in palette" :key="c" :value="c">{{ c }}</option>
                                <option v-if="pairOf(selected, 'colors', k) && !palette.includes(pairOf(selected, 'colors', k))" :value="pairOf(selected, 'colors', k)">
                                  {{ pairOf(selected, 'colors', k) }}
                                </option>
                              </select>
                            </td>
                            <td>
                              <input
                                :value="pairOf(selected, 'descriptions', k)"
                                class="fs-cell"
                                placeholder="what the value means"
                                spellcheck="false"
                                :aria-label="`Description for ${k}`"
                                @input="setPair(selected, 'descriptions', k, inputValue($event))"
                              />
                            </td>
                            <td>
                              <input
                                :value="pairOf(selected, 'project', k)"
                                class="fs-cell"
                                placeholder="if different"
                                spellcheck="false"
                                :aria-label="`GitHub Project name for ${k}`"
                                @input="setPair(selected, 'project', k, inputValue($event))"
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
            </div>
            <p v-else class="fs-muted">No field — add one below the list.</p>
          </div>

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
import { computed, nextTick, onMounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { getConfigFile, saveConfig, getFieldUsage, type ConfigField } from '@/api/board';
import { fieldPresets, takeUnique } from '@/configpresets';
import { allNumbers, inferType, type FieldType } from '@/fieldinfer';
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

// the field as the editor holds it: its maps as pairs (the rows' source
// of truth), the file's Record read back at the edges
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
// the section's draft: the rows, and the list's order as row keys — keys,
// not names, so a rename rides along and the order the user set survives
type FieldsValue = { rows: FieldRow[]; order: number[] };

const modelError = configModelError;
let key = 0;

const toPairs = (m?: Record<string, string>): Pair[] => Object.entries(m ?? {}).map(([k, v]) => ({ k, v }));
const kvOf = (rows: Pair[]): Record<string, string> | undefined => {
  const out = Object.fromEntries(rows.filter((r) => r.k.trim() && r.v.trim()).map((r) => [r.k.trim(), r.v]));
  return Object.keys(out).length ? out : undefined;
};

// what the values say the field is: the file's numbers sort by value, the
// editor keeps them that way
const sortByNumber = (options: string[]): string[] => [...options].sort((a, b) => (parseFloat(a) || 0) - (parseFloat(b) || 0));

const asRow = (name: string, f: ConfigField): FieldRow => {
  const options = [...(f.options ?? [])];
  // a numbers field's order is the values': sort it, in the file too
  const fixed = f.type === 'number' || f.numeric || (f.type !== 'select' && f.type !== 'text' && allNumbers(options));
  return {
    key: key++,
    name,
    isNew: false,
    single: f.single ?? false,
    numeric: f.numeric ?? false,
    type: f.type ?? '',
    unit: f.unit ?? '',
    options: fixed ? sortByNumber(options) : options,
    colors: toPairs(f.colors),
    descriptions: toPairs(f.descriptions),
    project: toPairs(f.project),
  };
};

// the draft is the rows and the list's order; the remove list is the
// names the file has that the draft lost — deleted, or renamed
const draft = new SectionDraft<FieldsValue>(
  'fields',
  (m) => {
    const rows = Object.entries(m.fields).map(([name, f]) => asRow(name, f));
    const byKey = new Map(rows.map((r) => [r.name, r.key]));
    const order = m.field_order.map((n) => byKey.get(n)).filter((k): k is number => k !== undefined);
    for (const r of rows) if (!order.includes(r.key)) order.push(r.key);
    return { rows, order };
  },
  (a, b) => JSON.stringify(a.rows.map(rowOf)) !== JSON.stringify(b.rows.map(rowOf)) || JSON.stringify(a.order) !== JSON.stringify(b.order),
  (value) => {
    const names = value.rows.map((f) => f.name.trim()).filter(Boolean);
    const set: Record<string, unknown> = {};
    for (const f of value.rows) {
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
    const remove = Object.keys(configModel.value?.fields ?? {}).filter((n) => !names.includes(n));
    // the list's order, as the names that will exist once this save
    // lands; sent only when it isn't the file's already
    const model = configModel.value;
    const exists = (n: string) => n in set || (n in (model?.fields ?? {}) && !remove.includes(n));
    const order = value.order.map((k) => value.rows.find((r) => r.key === k)?.name.trim()).filter((n): n is string => !!n && exists(n));
    const body: { set: Record<string, unknown>; remove: string[]; order?: string[] } = { set, remove };
    if (JSON.stringify(order) !== JSON.stringify((model?.field_order ?? []).filter(exists))) body.order = order;
    return body;
  },
);

const rowOf = (f: FieldRow) => [f.name, f.single, f.numeric, f.type, f.unit, f.options, f.colors, f.descriptions, f.project];

const fields = computed<FieldRow[]>({
  get: () => draft.value.value?.rows ?? [],
  set: (rows) => {
    if (draft.value.value) draft.value.value = { ...draft.value.value, rows };
  },
});

// the list's rows: the draft's order, then anything it doesn't name yet
const orderedRows = computed(() => {
  const rows = fields.value;
  const inOrder = (draft.value.value?.order ?? []).map((k) => rows.find((r) => r.key === k)).filter((r): r is FieldRow => !!r);
  return [...inOrder, ...rows.filter((r) => !inOrder.includes(r))];
});

function setOrder(keys: number[]) {
  if (draft.value.value) draft.value.value = { ...draft.value.value, order: keys };
}

// the file's fields as the editor holds them, for the change badge
const baseRows = computed(() => (configModel.value ? draft.from(configModel.value).rows : []));

function changed(f: FieldRow): boolean {
  const base = baseRows.value.find((x) => x.name === f.name);
  if (!base) return true;
  return JSON.stringify(rowOf(f)) !== JSON.stringify(rowOf(base));
}

// what the editor says the field is, in the page's words: the explicit
// type, else what the values and the name infer
const roles = computed(() => Object.keys(configModel.value?.roles ?? {}));

function summary(f: FieldRow): string {
  if (f.type === 'text') return 'Free text · no list';
  const t: FieldType = f.type === '' ? inferType(f.name, f.options, f.numeric, roles.value) : (f.type as FieldType);
  const card = f.single ? 'one per issue' : 'several per issue';
  switch (t) {
    case 'number':
      return `Numbers${f.unit ? ` in ${f.unit}` : ''} · ${card}`;
    case 'date':
      return `Dates · ${card}`;
    case 'person':
      return `Forge logins · ${card}`;
    case 'iteration':
      return `Sprints · ${card}`;
    case 'issue':
      return `Another issue · ${card}`;
  }
  return f.options.length ? `${f.options.length} values, in workflow order · ${card}` : `Guessed from the labels · ${card}`;
}

// the numbers mode: an explicit type decides, then the file's numeric,
// then what the values are — "treat them as labels" is type = select
function numbers(f: FieldRow): boolean {
  if (f.type && f.type !== 'number') return false;
  return f.type === 'number' || f.numeric || allNumbers(f.options);
}

// the three cardinalities: one value, several, or any text (type text)
function cardOf(f: FieldRow): 'one' | 'many' | 'text' {
  return f.type === 'text' ? 'text' : f.single ? 'one' : 'many';
}

function setCard(f: FieldRow, m: 'one' | 'many' | 'text') {
  if (m === 'text') {
    f.type = 'text';
    return;
  }
  if (f.type === 'text') f.type = '';
  f.single = m === 'one';
}

// the dropdown's "guessed" says what the guess is
function inferLabel(f: FieldRow): string {
  const labels: Record<FieldType, string> = {
    select: 'values as labels',
    number: 'numbers',
    date: 'dates',
    person: 'forge logins',
    iteration: 'sprints',
    issue: 'another issue',
    text: 'free text',
  };
  return labels[inferType(f.name, f.options, f.numeric, roles.value)];
}

// the details toggle's one line: what is set, not a bare count
function detailsSummary(f: FieldRow): string {
  const parts: string[] = [];
  if (f.colors.length) parts.push(`${f.colors.length} color${f.colors.length === 1 ? '' : 's'}`);
  if (f.descriptions.length) parts.push(`${f.descriptions.length} description${f.descriptions.length === 1 ? '' : 's'}`);
  if (f.project.length) parts.push(`${f.project.length} GitHub name${f.project.length === 1 ? '' : 's'}`);
  return parts.length ? parts.join(' · ') : 'colors, descriptions, GitHub Project names';
}

// the field at the address: #/settings/fields/<name>, none is the first
const route = useRoute();
const router = useRouter();
const routeName = computed(() => (typeof route.params.name === 'string' ? route.params.name : ''));
const selectedKey = ref<number | null>(null);

watch(
  [routeName, fields],
  ([n, rows]) => {
    const hit = n ? rows.find((r) => r.name === n) : undefined;
    if (hit) {
      selectedKey.value = hit.key;
      return;
    }
    // the address names nothing here — a bare #/settings/fields, a field
    // renamed or removed: stay on the selection while it lives, else the
    // first field
    if (!rows.some((r) => r.key === selectedKey.value)) selectedKey.value = rows[0]?.key ?? null;
  },
  { immediate: true },
);

// a rename (the TOML side does them) keeps the row and moves the address
watch(
  () => fields.value.find((r) => r.key === selectedKey.value)?.name,
  (name) => {
    const n = routeName.value;
    if (name && n && n !== name && !fields.value.some((r) => r.name === n)) {
      void router.replace({ path: `/settings/fields/${name}` });
    }
  },
);

const selected = computed(() => fields.value.find((r) => r.key === selectedKey.value) ?? null);

// the list's order: drag a row onto another, or Alt+↑/↓ on a focused row
const listFrom = ref<number | null>(null);

function dropRow(i: number) {
  const from = listFrom.value;
  listFrom.value = null;
  if (from === null || from === i) return;
  const keys = orderedRows.value.map((r) => r.key);
  const moved = keys[from];
  if (moved === undefined) return;
  keys.splice(from, 1);
  keys.splice(i, 0, moved);
  setOrder(keys);
}

function moveRow(f: FieldRow, d: number) {
  const keys = orderedRows.value.map((r) => r.key);
  const i = keys.indexOf(f.key);
  const j = i + d;
  if (i < 0 || j < 0 || j >= keys.length) return;
  keys.splice(j, 0, ...keys.splice(i, 1));
  setOrder(keys);
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
  if (!words.length) return;
  const options = [...f.options, ...words];
  f.options = numbers(f) ? sortByNumber(options) : options;
}

// the editor's other rendering: a [fields.<name>] snippet, applied as it
// is typed. While it parses, the snippet is the source of truth — a key
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

// a new field lands at the list's bottom, opened, its name focused
const nameInput = ref<HTMLInputElement | null>(null);
const focusName = ref(false);

function openNew(f: FieldRow) {
  selectedKey.value = f.key;
  void router.push({ path: `/settings/fields/${f.name}` });
  focusName.value = f.isNew;
}

watch(selectedKey, async () => {
  if (!focusName.value) return;
  focusName.value = false;
  await nextTick();
  nameInput.value?.focus();
});

function addBlank() {
  const f: FieldRow = { key: key++, name: takeUnique('new_field', fields.value.map((x) => x.name)), isNew: true, single: false, numeric: false, type: '', unit: '', options: [], colors: [], descriptions: [], project: [] };
  fields.value = [...fields.value, f];
  openNew(f);
}

function addPreset(i: number) {
  const p = fieldPresets[i];
  if (!p) return;
  const taken = fields.value.map((f) => f.name);
  const pf = p.value.field;
  const f: FieldRow = {
    key: key++,
    name: takeUnique(p.value.name, taken),
    isNew: false,
    single: pf.single ?? false,
    numeric: pf.numeric ?? false,
    type: pf.type ?? '',
    unit: pf.unit ?? '',
    options: [...(pf.options ?? [])],
    colors: toPairs(pf.colors),
    descriptions: toPairs(pf.descriptions),
    project: toPairs(pf.project),
  };
  fields.value = [...fields.value, f];
  openNew(f);
}

// how many issues carry each field's ns:value labels: the Remove
// confirm's sentence. A field no issue uses is absent, not zero.
const usage = ref<Record<string, number>>({});

const removeConfirm = computed(() => {
  const f = selected.value;
  if (!f) return '';
  const n = usage.value[f.name];
  const uses = n === undefined ? 'No issue uses it yet' : n === 1 ? 'One issue uses it' : `${n} issues use it`;
  return `${uses}. Removing the field ${f.name} takes its values, colors, descriptions and GitHub names from the file. The issues keep their ${f.name}:… labels, in no order.`;
});

const hasTomlError = computed(() => fields.value.some((f) => tomlError.value[f.key]));const valid = computed(() => {
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

onMounted(() => {
  void draft.load();
  getFieldUsage()
    .then((u) => (usage.value = u))
    .catch(() => {}); // the confirm says "No issue uses it yet" without it
});
</script>

<style scoped>
.fs { display: grid; gap: .75rem; container-type: inline-size; }
/* the work left (list over editor), the reference right; one column when narrow */
.set-cols { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 20rem); gap: .75rem; align-items: start; }
.set-forms { display: grid; gap: .75rem; min-width: 0; }
@container (max-width: 52rem) { .set-cols { grid-template-columns: minmax(0, 1fr); } }
h2, p { margin: 0; }
.fs-lead { max-width: 42rem; color: var(--fg-muted); }
.fs-muted { color: var(--fg-muted); }
.fs-start { display: grid; gap: .75rem; padding: 1.25rem; border: 1px solid var(--border); border-left: 3px solid var(--primary); background: var(--bg); }
.fs-startrow { display: flex; align-items: center; gap: .75rem; }
/* the work area: the list a fixed column, the editor the rest */
.fl-work { display: grid; grid-template-columns: 17rem minmax(0, 1fr); gap: .75rem; align-items: start; }
@container (max-width: 44rem) { .fl-work { grid-template-columns: minmax(0, 1fr); } }
/* the list: one row a field, in the file's order */
.fl-list { display: grid; gap: .5rem; min-width: 0; }
.fl-listhead { font-size: .8125rem; color: var(--fg-muted); }
.fl-listhead em { font-style: normal; }
.fl-rows { display: grid; gap: 2px; margin: 0; padding: 0; list-style: none; }
.fl-row { display: grid; gap: .125rem; min-height: 2.75rem; padding: .375rem .5rem; border: 1px solid var(--border); background: var(--bg); color: var(--fg); text-decoration: none; cursor: grab; }
.fl-row:active { cursor: grabbing; }
.fl-row:hover { border-color: var(--border-strong); }
.fl-row-sel { border-color: var(--fg); border-left: 3px solid var(--primary); padding-left: calc(.5rem - 2px); }
.fl-row-hold { opacity: .4; }
.fl-line1 { display: flex; align-items: center; gap: .375rem; min-width: 0; }
.fl-grip { color: var(--fg-muted); font-size: .75rem; }
.fl-nm { overflow: hidden; font-family: var(--font-mono); font-weight: 700; font-size: .875rem; text-overflow: ellipsis; white-space: nowrap; }
.fl-dot { width: .5rem; height: .5rem; flex: none; background: var(--danger); }
.fl-sum { overflow: hidden; font-size: .75rem; color: var(--fg-muted); text-overflow: ellipsis; white-space: nowrap; }
.fl-values { overflow: hidden; font-family: var(--font-mono); font-size: .75rem; color: var(--fg-muted); text-overflow: ellipsis; white-space: nowrap; }
.fl-addrow { display: flex; align-items: center; gap: .5rem; flex-wrap: wrap; }
/* the editor: the selected field's card */
.fl-ed { display: grid; gap: .75rem; min-width: 0; }
.fs-card { border-left-color: var(--primary); }
/* the card's head: fields.<name>, what the field is, the unsaved mark */
.fs-ns { font-size: .8125rem; color: var(--fg-muted); }
.fs-name { width: 9rem; min-height: 2rem; padding: 0 .5rem; border: 1px solid var(--border); background: var(--bg); color: var(--fg); font-family: var(--font-mono); font-size: 1rem; font-weight: 700; }
.fs-name:disabled { border-color: transparent; background: none; padding-left: 0; }
.fs-name:focus { outline: 2px solid var(--fg); outline-offset: 2px; }
.fs-sum { color: var(--fg-muted); font-size: .8125rem; }
.fs-dirty { font-size: .75rem; font-weight: 700; color: var(--danger); }
/* the editor's two renderings: the form and this field's TOML */
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
.fs-chip-fixed { cursor: default; }
.fs-chip-hold { opacity: .4; }
.fs-dot { width: .625rem; height: .625rem; background: var(--c-dot, var(--border-strong)); }
.fs-chip-x { width: 1.25rem; border: 0; background: none; color: var(--fg-muted); font: inherit; cursor: pointer; }
.fs-chip-x:hover { color: var(--danger); }
.fs-add { flex: 1 1 9rem; min-height: 1.625rem; border: 0; background: none; color: var(--fg); font: inherit; font-size: .8125rem; }
.fs-form { display: flex; gap: .75rem; flex-wrap: wrap; align-items: end; }
.fs-field { display: grid; gap: .25rem; font-size: .8125rem; }
.fs-field > span { color: var(--fg-muted); }
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
.fs-tablewrap { overflow-x: auto; border: 1px solid var(--border); }
.fs-table { width: 100%; min-width: 30rem; border-collapse: collapse; font-size: .8125rem; }
.fs-table th { padding: .375rem .5rem; border-bottom: 1px solid var(--border); background: var(--bg-subtle); text-align: left; font-size: .75rem; color: var(--fg-muted); }
.fs-table td { padding: .25rem .5rem; border-bottom: 1px solid var(--border); vertical-align: middle; }
.fs-tvalue { font-family: var(--font-mono); font-weight: 700; }
.fs-cell { width: 100%; box-sizing: border-box; min-height: 1.75rem; padding: .25rem .5rem; border: 1px solid var(--border); background: var(--bg); color: var(--fg); font: inherit; font-size: .8125rem; }
.fs-cell:focus { outline: 2px solid var(--fg); outline-offset: 2px; }
</style>
