<!-- Settings › Labels: the [labels] table the project file holds — the
     plain labels the project allows, their colors and descriptions, saved
     like every form tab. (v1's comparison with GitHub's labels and its
     lint are git board lint's, a tool v2 serves with M6.) -->
<template>
  <div class="set-panel lb">
    <h2 class="h4">Labels</h2>
    <p class="lb-lead">The yes-or-no labels an issue can carry.</p>

    <div class="set-cols">
      <div class="set-forms">
        <p v-if="modelError" class="epfl-alert epfl-alert-danger" role="alert">{{ modelError }}</p>

        <!-- the [labels] table, as the forms hold it -->
        <section v-if="labelsDraft.value.value" class="lb-own" aria-labelledby="lb-own">
          <h3 id="lb-own" class="h5">The project's plain labels</h3>
          <ConfigRows :rows="labelsDraft.value.value.list" add-label="Add a label" @update="labelsDraft.value.value.list = $event" @add="labelsDraft.value.value.list = [...labelsDraft.value.value.list, '']">
            <template #row="{ row, set }">
              <input :value="row" class="lb-tag" placeholder="needs-design" spellcheck="false" aria-label="A plain label" @input="set(inputValue($event))" />
            </template>
            <template #extra>
              <ConfigPreset label="From the preset" :presets="[labelsPreset]" @pick="labelsDraft.value.value.list = [...labelsDraft.value.value.list, ...labelsPreset.value.list]" />
            </template>
          </ConfigRows>
          <p class="gb-hint">
            The order the issue's head offers them as checkboxes, never as free text. A label with a colon (<code>prio:p1</code>) is a field's value; a kind is the kind field's — both refused.
          </p>
          <details class="lb-more" :open="labelsDraft.value.value.colors.length > 0">
            <summary>Colors <span class="lb-count">{{ labelsDraft.value.value.colors.length }}</span></summary>
            <ConfigRows :rows="labelsDraft.value.value.colors" add-label="Add a color" @update="labelsDraft.value.value.colors = $event" @add="labelsDraft.value.value.colors = [...labelsDraft.value.value.colors, { k: '', v: 'grey' }]">
              <template #row="{ row, set }">
                <div class="lb-pair">
                  <input :value="row.k" class="lb-tag" list="lb-label-names" spellcheck="false" aria-label="Label" @input="set({ ...row, k: inputValue($event) })" />
                  <select :value="row.v" class="lb-tag lb-tiny" aria-label="Color" @change="set({ ...row, v: inputValue($event) })">
                    <option v-for="c in palette" :key="c" :value="c">{{ c }}</option>
                    <option v-if="row.v && !palette.includes(row.v)" :value="row.v">{{ row.v }}</option>
                  </select>
                </div>
              </template>
            </ConfigRows>
            <datalist id="lb-label-names"><option v-for="l in labelsDraft.value.value.list" :key="l" :value="l" /></datalist>
          </details>
          <details class="lb-more" :open="labelsDraft.value.value.descriptions.length > 0">
            <summary>Descriptions <span class="lb-count">{{ labelsDraft.value.value.descriptions.length }}</span></summary>
            <ConfigRows :rows="labelsDraft.value.value.descriptions" add-label="Add a description" @update="labelsDraft.value.value.descriptions = $event" @add="labelsDraft.value.value.descriptions = [...labelsDraft.value.value.descriptions, { k: '', v: '' }]">
              <template #row="{ row, set }">
                <div class="lb-pair">
                  <input :value="row.k" class="lb-tag" list="lb-label-names" spellcheck="false" aria-label="Label" @input="set({ ...row, k: inputValue($event) })" />
                  <input :value="row.v" class="lb-desc" placeholder="what the label means" aria-label="Description" @input="set({ ...row, v: inputValue($event) })" />
                </div>
              </template>
            </ConfigRows>
          </details>
          <ConfigSaveBar
            section="labels"
            :dirty="labelsDraft.dirty.value"
            :saving="labelsDraft.saving.value"
            :saved="labelsDraft.saved.value"
            :valid="true"
            :error="labelsDraft.saveError.value"
            :conflict="labelsDraft.conflict.value"
            :state="`${labelsDraft.value.value.list.length} label${labelsDraft.value.value.list.length === 1 ? '' : 's'}`"
            @save="labelsDraft.save()"
            @discard="resetLabels"
            @reload="labelsDraft.load()"
            @overwrite="labelsDraft.save(true)"
          />
        </section>

      </div>

      <ConfigRefPanel :sections="['labels']" title="the [labels] table" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted } from 'vue';
import { labelsPreset } from '@/configpresets';
import { configModel, configModelError, SectionDraft, inputValue } from '@/configmodel';
import ConfigPreset from '@/components/configforms/ConfigPreset.vue';
import ConfigRows from '@/components/configforms/ConfigRows.vue';
import ConfigRefPanel from '@/components/configforms/ConfigRefPanel.vue';
import ConfigSaveBar from '@/components/configforms/ConfigSaveBar.vue';

// the [labels] table, as the forms hold it: its maps as rows
type Pair = { k: string; v: string };
const modelError = configModelError;
const palette = ['grey', 'blue', 'purple', 'amber', 'orange', 'green', 'red', 'teal', 'pink'];
const toPairs = (m?: Record<string, string>): Pair[] => Object.entries(m ?? {}).map(([k, v]) => ({ k, v }));
const kvOf = (rows: Pair[]): Record<string, string> | undefined => {
  const out = Object.fromEntries(rows.filter((r) => r.k.trim() && r.v.trim()).map((r) => [r.k.trim(), r.v]));
  return Object.keys(out).length ? out : undefined;
};

const labelsDraft = new SectionDraft<{ list: string[]; colors: Pair[]; descriptions: Pair[] }>(
  'labels',
  (m) => ({ list: [...m.labels.list], colors: toPairs(m.labels.colors), descriptions: toPairs(m.labels.descriptions) }),
  (a, b) => JSON.stringify(a) !== JSON.stringify(b),
  (value) => ({ list: value.list.filter((l) => l.trim()), colors: kvOf(value.colors), descriptions: kvOf(value.descriptions) }),
);
function resetLabels() {
  if (configModel.value) labelsDraft.reset(configModel.value);
}
onMounted(() => void labelsDraft.load());
</script>

<style scoped>
.lb h2, .lb h3, .lb h4, .lb p, .lb ul { margin: 0; }
.lb-lead { max-width: 48rem; color: var(--fg-muted); }
.lb-own { display: grid; gap: .75rem; padding: .75rem 1rem; border: 1px solid var(--border); border-left: 3px solid var(--primary); background: var(--bg); }
.lb-tag { padding: .375rem .625rem; border: 1px solid var(--border); background: var(--bg); color: var(--fg); font: inherit; }
.lb-tag:focus, .lb-desc:focus { outline: 2px solid var(--fg); outline-offset: 2px; border-color: var(--fg); }
.lb-tiny { max-width: 6rem; }
.lb-desc { flex: 1; min-width: 12rem; padding: .375rem .625rem; border: 1px solid var(--border); background: var(--bg); color: var(--fg); font: inherit; }
.lb-pair { display: flex; gap: .5rem; }
.lb-more summary { cursor: pointer; font-size: .875rem; font-weight: 700; color: var(--fg-muted); }
.lb-more summary:hover { color: var(--fg); }
.lb-more { display: grid; gap: .5rem; }
.lb-count { display: inline-block; margin-left: .375rem; padding: 0 .375rem; border: 1px solid var(--border); font-weight: 400; font-size: .75rem; color: var(--fg-muted); }
</style>
