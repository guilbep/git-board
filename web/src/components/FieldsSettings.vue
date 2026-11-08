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
    <p class="fs-lead">What an issue can say about itself: each field's values, their order, and how many an issue holds.</p>

    <div class="set-cols">
      <div class="set-forms">
        <p v-if="modelError" class="epfl-alert epfl-alert-danger" role="alert">{{ modelError }}</p>

        <!-- no file yet: the default is written from here -->
        <div v-else-if="configModel && !configModel.exists" class="fs-start">
          <p class="fs-lead">
            No project file yet: every <code>ns:value</code> label is a field, in no order.
            <button type="button" class="gb-linkbtn" :disabled="starting" @click="start">{{ starting ? 'Writing…' : 'Write the default file' }}</button>
            — the statuses, priorities, kinds and estimates, the branch names and four views, each explained in a comment.
          </p>
          <p v-if="startError" class="epfl-alert epfl-alert-danger" role="alert"><b>Not written.</b> {{ startError }}</p>
        </div>

        <p v-else-if="!draft.value.value" class="fs-muted">Loading…</p>

        <template v-else>
          <!-- the file changed under the page (a pull, another tab): the
               draft's edits are against the old text -->
          <div v-if="fileChanged" class="epfl-alert epfl-alert-warning fl-changed" role="alert">
            <span>The file changed since the page read it. Reloading takes the draft's edits away.</span>
            <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" @click="takeFile">Reload the file</button>
            <button type="button" class="gb-linkbtn" @click="fileChanged = false">Keep editing</button>
          </div>

          <div class="fl-work" :class="{ 'fl-work-sel': !!routeName }">
            <div class="fl-list">
              <p class="fl-listhead">
                {{ fields.length }} field{{ fields.length === 1 ? '' : 's' }} <em>— drag a row, or Alt+↑/↓, to set the order on issues</em>
              </p>
              <input
                v-if="fields.length > 12"
                v-model="filter"
                class="fs-input fl-filter"
                placeholder="Find a field or a value…"
                spellcheck="false"
                aria-label="Find a field"
              />
              <ol class="fl-rows">
                <li v-for="(f, i) in visibleRows" :key="f.key">
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
                      <span
                        v-if="tomlError[f.key]"
                        class="fl-warn"
                        role="img"
                        :aria-label="`Its TOML has an error: ${tomlError[f.key]}`"
                        :title="tomlError[f.key]"
                      >!</span>
                      <span v-if="changed(f)" class="fl-dot" title="Unsaved changes" />
                      <span class="fl-sum">{{ summary(f) }}</span>
                    </span>
                    <span class="fl-values">{{ valueless(f) ? '' : f.options.join(' → ') }}</span>
                    <span v-if="(configModel?.forges.length || project?.project) && f.name.trim()" class="fl-places">
                      <span v-for="pl in placeChips(f)" :key="pl.forge" class="fl-place" :class="{ 'fl-place-own': pl.own, 'fl-place-missing': pl.missing }" :title="`On ${pl.title}: ${pl.text}${pl.missing ? ' — the GitHub Project has no such column (its last check)' : ''}`">
                        <GbIcon :name="pl.kind === 'gitlab' ? 'gitlab' : 'github'" small />{{ pl.text }}
                      </span>
                    </span>
                  </router-link>
                </li>
              </ol>
              <p class="fl-addrow">
                <ConfigPreset label="+ Add a field" :presets="presetsShown" @pick="addPreset" />
                <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" @click="addBlank">Blank field</button>
                <span class="gb-grow" />
                <router-link class="gb-linkbtn" to="/settings/advanced">Edit the file instead</router-link>
              </p>
            </div>

            <div v-if="selected" class="fl-ed">
              <router-link class="fl-back" to="/settings/fields">← All fields</router-link>
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
                  <p v-else-if="renamedFrom[selected.key]" class="gb-hint">
                    Renamed from <code>{{ renamedFrom[selected.key] }}</code> — the issues keep their old
                    <code>{{ renamedFrom[selected.key] }}:…</code> labels; lint finds them.
                  </p>
                  <p v-else class="gb-hint">This field's table only — it applies as you type. Comments are lost when you switch back to the form.</p>
                  <template v-if="placesToml(selected.name, draft.value.value.places)">
                    <p class="gb-hint">Where it lives, written with it (edit it in the form):</p>
                    <pre class="fs-places-toml">{{ placesToml(selected.name, draft.value.value.places) }}</pre>
                  </template>
                </div>
                <div v-else class="fs-body">
                <!-- the type first: it decides the rest of the form -->
                <label v-if="!numbers(selected)" class="fs-field fs-type">
                  <span>Type</span>
                  <select :value="selected.type" class="fs-input fs-small" aria-label="Type" @change="setType(selected, inputValue($event))">
                    <option value="">guessed — {{ inferLabel(selected) }}</option>
                    <option v-for="t in types" :key="t.id" :value="t.id">{{ t.label }}</option>
                  </select>
                </label>
                <!-- a date, a person, an issue, a sprint: no values of its own -->
                <div v-if="valueless(selected)" class="fs-block">
                  <span class="fs-label">Values</span>
                  <p class="gb-hint">
                    {{ bareHint[kindOf(selected)] }}
                    <router-link v-if="kindOf(selected) === 'iteration'" class="gb-linkbtn" to="/settings/sprints">Sprints</router-link>
                  </p>
                  <p v-if="selected.options.length" class="gb-hint fs-drop">
                    The file lists {{ selected.options.length }} value{{ selected.options.length === 1 ? '' : 's' }} for it ({{ selected.options.join(', ') }}): saving takes them out, with their colors and descriptions.
                  </p>
                </div>
                <div v-else-if="selected.type === 'text'" class="fs-block">
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
                        tabindex="0"
                        :aria-label="`${o} — Alt+Left or Alt+Right moves it`"
                        @keydown.alt.left.prevent="!numbers(selected) && moveOption(selected, i, -1)"
                        @keydown.alt.right.prevent="!numbers(selected) && moveOption(selected, i, 1)"
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
                      <label v-if="!valueless(selected)" class="fs-radio"><input type="radio" :checked="cardOf(selected) === 'text'" @change="setCard(selected, 'text')" /> any text, no list</label>
                    </fieldset>
                    <template v-if="numbers(selected)">
                      <label class="fs-field">
                        <span>Unit</span>
                        <input v-model="selected.unit" class="fs-input fs-tiny" :placeholder="selected.name === 'est' ? 'h' : ''" spellcheck="false" aria-label="Unit shown after the number" />
                      </label>
                      <button type="button" class="gb-linkbtn" @click="selected.type = 'select'">Treat them as labels instead</button>
                    </template>
                  </div>

                  <details v-if="!valueless(selected)" class="fs-more" :open="detailKeys(selected).length > 0">
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
                            <th>Also read as</th>
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
                                :value="aliasOf(selected, k)"
                                class="fs-cell"
                                placeholder="a retired name, folded in"
                                spellcheck="false"
                                :aria-label="`Old names of ${k}, comma apart`"
                                @input="setAlias(selected, k, inputValue($event))"
                              />
                            </td>
                          </tr>
                        </tbody>
                      </table>
                      <p class="gb-hint">Also read as: a value's retired names (<code>could</code> → <code>couldhave</code>). An issue or a forge reading the old name stores the current one; Adopt rewrites the labels.</p>
                    </div>
                    <p v-else class="gb-hint">No values yet: the details come once the values do.</p>
                  </details>
                  <!-- where it lives on each forge, saved with the field -->
                  <FieldPlaces
                    v-if="configModel?.forges.length && selected.name.trim()"
                    :field="selected.name.trim()"
                    :fields="fieldsWith(selected)"
                    :forges="configModel.forges"
                    :places="draft.value.value.places"
                    :roles="Object.keys(configModel.roles ?? {})"
                    :project-number="configModel.github_project?.number ?? 0"
                    :options="valueless(selected) ? [] : selected.options"
                    @set="setPlace"
                  />
                </div>
              </ConfigCard>
              <!-- a place changed and saved: the issues move when you say so -->
              <section v-if="migrating.length" class="fs-migrate" aria-labelledby="fs-mig">
                <h3 id="fs-mig" class="h5">Move the issues to their new place</h3>
                <ul>
                  <li v-for="m in migrating" :key="m.field">
                    <code>{{ m.field }}</code>: <template v-if="m.plans">{{ m.plans.map((p) => `${p.forge}, ${p.issues} issue${p.issues === 1 ? '' : 's'}${p.labels ? ` (${p.labels} labels come off)` : ''}`).join(' · ') || 'nothing to move' }}</template><template v-else>counting…</template>
                    <button type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="!!migrateBusy" @click="runMigration(m.field)">{{ migrateBusy === m.field ? 'Migrating…' : 'Run the migration' }}</button>
                  </li>
                </ul>
                <p class="fs-muted">Nothing on the forges changes until you run it. Same as <code>git board sync --migrate &lt;field&gt;</code>. <button type="button" class="gb-linkbtn" @click="migrating = []">Not now</button></p>
              </section>
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
            @overwrite="draft.save(true)"
          />
        </template>
      </div>

      <ConfigRefPanel :sections="['fields', 'types', 'colors']" title="a field's keys, the types" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { onLive } from '@/live';
import { getLabels, getProject, initConfig, migrateField, type MigratePlan, type ProjectStatus } from '@/api';
import type { ConfigField } from '@/configparse';
import { fieldPresets, takeUnique } from '@/configpresets';
import { allNumbers, inferType, type FieldType } from '@/fieldinfer';
import { fieldToToml, tomlToField, type FieldToml } from '@/fieldtoml';
import { SectionDraft, configModel, configModelError, loadConfigModel, inputValue } from '@/configmodel';
import { fieldTarget, targetLabels, type ForgeMapping } from '@/fieldtargets';
import { columnFor, placesBody, placesOf, placesToml, samePlaces, type Places } from '@/fieldplaces';
import FieldPlaces from '@/components/FieldPlaces.vue';
import { projectMark } from '@/projectview';
import GbIcon from '@/components/GbIcon.vue';
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
  aliases: Pair[];
  colors: Pair[];
  descriptions: Pair[];
  project: Pair[];
};
type Pair = { k: string; v: string };
type PairMap = 'aliases' | 'colors' | 'descriptions' | 'project';
// the section's draft: the rows, and the list's order as row keys — keys,
// not names, so a rename rides along and the order the user set survives
// places: where each field lives on each forge (fieldplaces.ts), saved with the fields
type FieldsValue = { rows: FieldRow[]; order: number[]; places: Places };

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
    aliases: toPairs(f.aliases),
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
    return { rows, order, places: placesOf(m.forges) };
  },
  // the order by name: each from() numbers its rows afresh
  (a, b) => JSON.stringify(a.rows.map(rowOf)) !== JSON.stringify(b.rows.map(rowOf)) || JSON.stringify(orderNames(a)) !== JSON.stringify(orderNames(b)) || !samePlaces(a.places, b.places),
  (value) => {
    const names = value.rows.map((f) => f.name.trim()).filter(Boolean);
    const set: Record<string, unknown> = {};
    const saved = configModel.value?.fields ?? {};
    for (const f of value.rows) {
      if (!f.name.trim()) continue;
      // a field as the file has it is left alone: its lines stay as
      // written, and the kinds' flat field ([branches.kinds], no table of
      // its own) is not turned into a [fields.kind] table
      const was = saved[f.name.trim()];
      if (was && JSON.stringify(rowOf(asRow(f.name.trim(), was))) === JSON.stringify(rowOf(f))) continue;
      set[f.name.trim()] = rowToToml(f);
    }
    const remove = Object.keys(configModel.value?.fields ?? {}).filter((n) => !names.includes(n));
    // the list's order, as the names that will exist once this save
    // lands; sent only when it isn't the file's already
    const model = configModel.value;
    const exists = (n: string) => n in set || (n in (model?.fields ?? {}) && !remove.includes(n));
    const order = value.order.map((k) => value.rows.find((r) => r.key === k)?.name.trim()).filter((n): n is string => !!n && exists(n));
    const body: { set: Record<string, unknown>; remove: string[]; order?: string[]; forges?: unknown } = { set, remove };
    if (JSON.stringify(order) !== JSON.stringify((model?.field_order ?? []).filter(exists))) body.order = order;
    // where the fields live: what changed on each forge, in the same write
    const forges = placesBody(model?.forges ?? [], value.places, remove);
    if (forges) body.forges = forges;
    return body;
  },
);

const orderNames = (v: FieldsValue) => v.order.map((k) => v.rows.find((r) => r.key === k)?.name);
const rowOf = (f: FieldRow) => [f.name, f.single, f.numeric, f.type, f.unit, f.options, f.aliases, f.colors, f.descriptions, f.project];

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

// the list, filtered once it is long (the box shows past 12 fields)
const filter = ref('');
const visibleRows = computed(() => {
  const q = filter.value.trim().toLowerCase();
  if (!q) return orderedRows.value;
  return orderedRows.value.filter((r) => r.name.toLowerCase().includes(q) || r.options.some((o) => o.toLowerCase().includes(q)));
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

// what the field is: its type, else the guess
function kindOf(f: FieldRow): FieldType {
  return (f.type || inferType(f.name, f.options, f.numeric, roles.value)) as FieldType;
}

// the types whose values are not a list the file keeps
function valueless(f: FieldRow): boolean {
  return ['date', 'person', 'issue', 'iteration'].includes(kindOf(f));
}

const bareHint: Partial<Record<FieldType, string>> = {
  date: 'A date per issue, YYYY-MM-DD, picked on the issue: red once past, amber within a week. No values to list.',
  person: 'Forge logins, @me for you: the people come from the forges and the roles. No values to list.',
  issue: 'Another issue, #12: its page links it, and the other issue shows it back (Sub-issues, Blocks). No values to list.',
  iteration: 'A sprint: the values are the sprints, with their dates, and @current is the running one. They are set in',
};

// the numbers mode: an explicit type decides, then the file's numeric,
// then what the values are — "treat them as labels" is type = select
function numbers(f: FieldRow): boolean {
  if (f.type && f.type !== 'number') return false;
  return f.type === 'number' || f.numeric || allNumbers(f.options);
}

// where a field lives on each forge, as the draft has it: a chip each in
// the list, outlined when it isn't the type's default, marked when the
// GitHub Project's last check found no such column. Without [forges]
// tables, the server's places say the Project's columns (GET
// /api/github-project).
function placeChips(f: FieldRow): { forge: string; kind: string; title: string; text: string; own: boolean; missing: boolean }[] {
  const m = configModel.value;
  const v = draft.value.value;
  if (!m || !v) return [];
  const name = f.name.trim();
  const places = project.value?.places ?? [];
  if (!m.forges.length) {
    return places.filter((p) => p.field === name && p.to === 'project').map((p) => ({ forge: p.forge, kind: p.kind, title: 'GitHub', text: `Project › ${p.column ?? name}`, own: p.to !== p.default, missing: p.on_project === false }));
  }
  const roles = Object.keys(m.roles ?? {});
  const projectNumber = m.github_project?.number ?? 0;
  const fields = fieldsWith(f);
  return m.forges.map((forge) => {
    const to = fieldTarget({ ...forge, fields: v.places[forge.name] ?? {} }, fields, name, roles, projectNumber);
    const def = fieldTarget({ ...forge, fields: {} }, fields, name, roles, projectNumber);
    const column = v.places[forge.name]?.[name]?.field;
    const mark = to === 'project' ? projectMark(places, forge.name, name) : null;
    const text = to === 'project' ? `Project › ${column ?? mark?.column ?? columnFor(name)}` : (targetLabels[to] ?? to);
    return { forge: forge.name, kind: forge.kind, title: forge.kind === 'gitlab' ? 'GitLab' : 'GitHub', text, own: to !== def, missing: !!mark?.missing && (column ?? mark.column) === mark.column };
  });
}
// the model's fields with this one as drafted: its type decides its places
function fieldsWith(f: FieldRow): Record<string, ConfigField> {
  return { ...(configModel.value?.fields ?? {}), [f.name.trim()]: rowToToml(f) as unknown as ConfigField };
}
function setPlace(forge: string, field: string, mapping: ForgeMapping | null) {
  const v = draft.value.value;
  if (!v) return;
  const places = { ...v.places, [forge]: { ...v.places[forge] } };
  if (mapping) places[forge]![field] = mapping;
  else delete places[forge]![field];
  draft.value.value = { ...v, places };
}

// a new field typed as a date or a sprint starts at one value an issue: an
// issue has one due date, sits in one sprint
function setType(f: FieldRow, t: string) {
  f.type = t;
  if (f.isNew && (t === 'date' || t === 'iteration')) f.single = true;
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
  return parts.length ? parts.join(' · ') : 'colors, descriptions, old names';
}

// the field at the address: #/settings/fields/<name>, none is the first
const route = useRoute();
const router = useRouter();
// the address's field, on this tab alone: the page keeps every tab mounted
// (an old #/settings/forge-fields/<forge> names a forge, not a field)
const routeName = computed(() => (route.params.tab === 'fields' && typeof route.params.name === 'string' ? route.params.name : ''));
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
  const keys = visibleRows.value.map((r) => r.key);
  const moved = keys[from];
  if (moved === undefined) return;
  keys.splice(from, 1);
  keys.splice(i, 0, moved);
  setOrder(keys);
}

function moveRow(f: FieldRow, d: number) {
  const keys = visibleRows.value.map((r) => r.key);
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

// a focused chip moves with Alt+←/→, the keyboard's drag
function moveOption(f: FieldRow, i: number, d: number) {
  const j = i + d;
  if (j < 0 || j >= f.options.length) return;
  const options = [...f.options];
  const moved = options[i];
  if (moved === undefined) return;
  options.splice(j, 0, ...options.splice(i, 1));
  f.options = options;
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
const renamedFrom = ref<Record<number, string | undefined>>({});

// the field as the file gets it: a date, a person, an issue or a sprint
// has no values of its own, so its options and their details go — and its
// type is written once the values were what said it (a guess from the name
// stays a guess)
function rowToToml(f: FieldRow): FieldToml {
  const bare = valueless(f);
  const kind = kindOf(f);
  return {
    single: f.single,
    numeric: f.numeric,
    type: f.type || (bare && inferType(f.name, [], f.numeric, roles.value) !== kind ? kind : undefined),
    unit: f.unit || undefined,
    options: bare ? [] : f.options.filter((o) => o.trim()),
    aliases: bare ? undefined : kvOf(f.aliases),
    colors: bare ? undefined : kvOf(f.colors),
    descriptions: bare ? undefined : kvOf(f.descriptions),
    project: bare ? undefined : kvOf(f.project),
  };
}

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
  const from = f.name;
  applyToml(f, r.name, r.field);
  // a header rename is worth its own sentence: the issues keep the old key
  if (r.name && r.name !== from) renamedFrom.value[f.key] = from;
  else delete renamedFrom.value[f.key];
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
    renamedFrom.value = {};
  },
);

// the file changed under the page (a pull, another tab, a terminal): with
// edits in the draft a banner asks; with none, the file is the truth
const fileChanged = ref(false);

async function checkFile() {
  await loadConfigModel();
  if (configModel.value && configModel.value.hash !== draft.base.value) {
    if (draft.dirty.value) fileChanged.value = true;
    else draft.reset(configModel.value);
  }
}

const offLive = onLive((e) => {
  if (e.kind === 'config') void checkFile();
});
onBeforeUnmount(offLive);

async function takeFile() {
  fileChanged.value = false;
  await draft.load();
}

// the per-value details: one table keyed by value — the options in their
// order, then the pair keys the options lost; a cell emptied removes its
// pair
function detailKeys(f: FieldRow): string[] {
  const keys: string[] = [];
  for (const k of [...f.options, ...f.colors.map((r) => r.k), ...f.descriptions.map((r) => r.k), ...f.project.map((r) => r.k), ...f.aliases.map((r) => r.v)]) {
    if (!keys.includes(k)) keys.push(k);
  }
  return keys;
}

// the field's aliases, as the details table's "Also read as" cell holds
// them: one value's retired names, comma apart (could → couldhave)
function aliasOf(f: FieldRow, k: string): string {
  return f.aliases.filter((r) => r.v === k).map((r) => r.k).join(', ');
}

function setAlias(f: FieldRow, k: string, text: string) {
  const olds = text.split(',').map((s) => s.trim()).filter(Boolean);
  const rest = f.aliases.filter((r) => r.v !== k);
  f.aliases = [...rest, ...olds.map((old) => ({ k: old, v: k }))];
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
  const f: FieldRow = { key: key++, name: takeUnique('new_field', fields.value.map((x) => x.name)), isNew: true, single: false, numeric: false, type: '', unit: '', options: [], aliases: [], colors: [], descriptions: [], project: [] };
  fields.value = [...fields.value, f];
  openNew(f);
}

// a preset the list already holds says so, and opens that field instead
const presetsShown = computed(() =>
  fieldPresets.map((p) => (fields.value.some((f) => f.name === p.value.name) ? { ...p, label: `${p.label} · already declared`, why: 'opens it' } : p)),
);

function addPreset(i: number) {
  const p = fieldPresets[i];
  if (!p) return;
  const have = fields.value.find((f) => f.name === p.value.name);
  if (have) {
    selectedKey.value = have.key;
    void router.push({ path: `/settings/fields/${have.name}` });
    return;
  }
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
    aliases: toPairs(pf.aliases),
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

// the places a save moved: their migration (route 54), counted, run when
// asked
const migrating = ref<{ field: string; plans: MigratePlan[] | null }[]>([]);
const migrateBusy = ref('');
async function save() {
  const before = configModel.value;
  const moved = before && draft.value.value ? Object.values(placesBody(before.forges, draft.value.value.places) ?? {}).flatMap((p) => [...Object.keys(p.set), ...p.remove]) : [];
  await draft.save();
  if (draft.dirty.value || draft.saveError.value) return;
  migrating.value = [...new Set(moved)].map((field) => ({ field, plans: null }));
  for (const m of migrating.value)
    migrateField(m.field, true)
      .then((r) => (m.plans = r.plans ?? []))
      .catch(() => (m.plans = []));
}
function runMigration(field: string) {
  migrateBusy.value = field;
  migrateField(field)
    .then((r) => {
      if (r.error) draft.saveError.value = r.error;
      else migrating.value = migrating.value.filter((m) => m.field !== field);
    })
    .catch((e: unknown) => (draft.saveError.value = message(e)))
    .finally(() => (migrateBusy.value = ''));
}

// the GitHub Project's last check, where the fields it holds are found:
// the server's, read again each time the tab shows (the page keeps the
// tabs mounted); a failure just marks nothing
const project = ref<ProjectStatus | null>(null);
const loadProject = () => getProject().then((p) => (project.value = p)).catch(() => {});
watch(() => route.params.tab, (tab) => tab === 'fields' && void loadProject());

// the default project file, written as `git board init` does, from here
const starting = ref(false);
const startError = ref('');
async function start() {
  starting.value = true;
  startError.value = '';
  try {
    await initConfig();
    await draft.load();
  } catch (e) {
    startError.value = message(e);
  } finally {
    starting.value = false;
  }
}

onMounted(() => {
  void loadProject();
  void draft.load();
  // how many issues carry each field's values (GET /api/labels): a value
  // counted once each, so an issue with two of a field's counts twice
  getLabels('')
    .then((ls) => {
      const u: Record<string, number> = {};
      for (const l of ls) if (l.field) u[l.field] = (u[l.field] ?? 0) + l.open + l.closed;
      usage.value = u;
    })
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
/* the work area: the list a fixed column, the editor the rest */
.fl-work { display: grid; grid-template-columns: 17rem minmax(0, 1fr); gap: .75rem; align-items: start; }
/* a phone (under ~640px): the list and the editor are separate screens —
   the address decides which one; the editor carries the way back */
.fl-back { display: none; align-items: center; justify-self: start; }
@container (max-width: 40rem) {
  .fl-work { grid-template-columns: minmax(0, 1fr); }
  .fl-work-sel .fl-list { display: none; }
  .fl-work:not(.fl-work-sel) .fl-ed { display: none; }
  .fl-work-sel .fl-back { display: inline-flex; }
}
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
.fl-nm { flex: none; max-width: 100%; overflow: hidden; font-family: var(--font-mono); font-weight: 700; font-size: .875rem; text-overflow: ellipsis; white-space: nowrap; }
.fl-dot { width: .5rem; height: .5rem; flex: none; background: var(--danger); }
.fl-sum { flex: 1 1 0; min-width: 0; overflow: hidden; font-size: .75rem; color: var(--fg-muted); text-overflow: ellipsis; white-space: nowrap; }
.fl-values { overflow: hidden; font-family: var(--font-mono); font-size: .75rem; color: var(--fg-muted); text-overflow: ellipsis; white-space: nowrap; }
.fl-addrow { display: flex; align-items: center; gap: .5rem; flex-wrap: wrap; }
.fl-filter { max-width: 15rem; }
.fl-warn { flex: none; box-sizing: border-box; width: 1rem; height: 1rem; padding: 0; border-radius: 50%; background: var(--warning); color: var(--bg); font: 700 .6875rem/1rem var(--font-mono); text-align: center; cursor: help; }
.fl-changed { display: flex; align-items: center; gap: .75rem; flex-wrap: wrap; }
/* the editor: the selected field's card */
.fl-ed { display: grid; gap: .75rem; min-width: 0; }
.fs-card { border-left-color: var(--primary); }
/* the card's head: fields.<name>, what the field is, the unsaved mark */
.fs-ns { font-size: .8125rem; color: var(--fg-muted); }
.fs-name { width: 9rem; min-height: 2rem; padding: 0 .5rem; border: 1px solid var(--border); background: var(--bg); color: var(--fg); font-family: var(--font-mono); font-size: 1rem; font-weight: 700; }
.fs-name:disabled { border-color: transparent; background: none; padding-left: 0; }
.fs-name:focus { outline: 2px solid var(--fg); outline-offset: 2px; }
.fs-sum { color: var(--fg-muted); font-size: .8125rem; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
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
.fs-type { justify-self: start; }
.fs-drop { color: var(--fg); }
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
.fl-place-missing { border-color: var(--warning); color: var(--warning-fg, var(--fg)); }
.fl-places { display: flex; flex-wrap: wrap; gap: .25rem; margin-top: .25rem; }
.fl-place { display: inline-flex; align-items: center; gap: .25rem; padding: 0 .375rem; border: var(--border-w) solid var(--border-subtle); color: var(--fg-muted); font-size: .75rem; }
.fl-place-own { border-color: var(--fg); color: var(--fg); }
.fs-migrate { display: grid; gap: .5rem; padding: .75rem 1rem; border: var(--border-w) solid var(--border); border-left: .1875rem solid var(--warning); background: var(--warning-bg); }
.fs-migrate ul { display: grid; gap: .5rem; margin: 0; padding-left: 1.125rem; }
.fs-migrate li { display: flex; flex-wrap: wrap; align-items: center; gap: .5rem; }
.fs-places-toml { margin: 0; padding: .5rem .75rem; background: var(--bg-subtle); font-size: .75rem; white-space: pre-wrap; }
</style>
