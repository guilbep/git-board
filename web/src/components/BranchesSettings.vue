<!-- Settings › Branches: the [branches] table — the base an issue's branch
     starts from, its name template ({kind}, {number}, {n}, {slug}), and
     the kinds as [branches.kinds]: a kind's label is the {kind} of its
     branch, bug = bug, always. Add offers the kinds the default file ships. -->
<template>
  <div class="set-panel bs">
    <h2 class="h4">Branches</h2>
    <p class="bs-lead">How an issue's branch is named, and the base it starts from.</p>

    <div class="set-cols">
      <div class="set-forms">
        <p v-if="modelError" class="epfl-alert epfl-alert-danger" role="alert">{{ modelError }}</p>
        <p v-else-if="!draft.value.value" class="bs-muted">Loading…</p>

        <template v-else>
          <div class="bs-form">
            <label class="bs-field">
              <span>Base</span>
              <input :value="draft.value.value.base" class="bs-input" placeholder="empty: the remote's default branch" spellcheck="false" aria-label="The base branch" @input="set({ base: ($event.target as HTMLInputElement).value })" />
            </label>
            <label class="bs-field bs-grow">
              <span>Name</span>
              <input :value="draft.value.value.name" class="bs-input" placeholder="empty: {number}/{n}-{slug}" spellcheck="false" aria-label="The branch's name template" @input="set({ name: ($event.target as HTMLInputElement).value })" />
            </label>
          </div>
          <p class="gb-hint">
            The template's slots: <code>{kind}</code> the branch kind, <code>{number}</code> the issue's number, <code>{n}</code> the
            project's issue count, <code>{slug}</code> the title's words, dashed. Empty is the default.
          </p>

          <h3 class="h5">Kinds</h3>
          <ConfigRows :rows="kindRows" add-label="Add a kind" @update="kindRows = $event" @add="kindRows = [...kindRows, { k: '', v: '' }]">
            <template #extra>
              <ConfigPreset label="From the preset" :presets="[branchKindsPreset]" @pick="kindRows = [...kindRows, ...Object.entries(branchKindsPreset.value).map(([k, v]) => ({ k, v }))] as Pair[]" />
            </template>
            <template #row="{ row, set: setRow }">
              <div class="bs-pair">
                <input :value="row.k" class="bs-input bs-opt" list="bs-kind-names" placeholder="e.g. bug" spellcheck="false" aria-label="The issue's kind" @input="setRow({ ...row, k: inputValue($event) })" />
                <span v-if="notAKind(row.k)" class="bs-bad">not a kind</span>
              </div>
            </template>
          </ConfigRows>
          <p v-if="strangers.length" class="gb-hint bs-bad-line" role="status">
            {{ strangers.join(', ') }} {{ strangers.length === 1 ? 'is not a value' : 'are not values' }} of the kind field
            ({{ fieldKinds.join(', ') }}): no issue can carry {{ strangers.length === 1 ? 'it' : 'them' }}. Remove {{ strangers.length === 1 ? 'it' : 'them' }} here, or add
            {{ strangers.length === 1 ? 'it' : 'them' }} in <router-link to="/settings/fields/kind">Fields</router-link>.
          </p>
          <datalist id="bs-kind-names">
            <option v-for="k in fieldKinds" :key="k" :value="k" />
          </datalist>
          <p class="gb-hint">
            The issue's kind is its <code>kind:</code> label, else its first flat label listed here; its branch starts with
            it: <code>{kind}</code> is bug for a bug, always. The list knows the kinds the kind field holds.
          </p>

          <ConfigSaveBar
            section="branches"
            :dirty="draft.dirty.value"
            :saving="draft.saving.value"
            :saved="draft.saved.value"
            :valid="true"
            :error="draft.saveError.value"
            :conflict="draft.conflict.value"
            :state="state"
            @save="draft.save()"
            @discard="reset"
            @reload="draft.load()"
          />
        </template>


      </div>

      <ConfigRefPanel :sections="['branches']" title="the branch table" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { branchKindsPreset } from '@/configpresets';
import { SectionDraft, configModel, configModelError, inputValue } from '@/configmodel';
import ConfigPreset from '@/components/configforms/ConfigPreset.vue';
import ConfigRefPanel from '@/components/configforms/ConfigRefPanel.vue';
import ConfigRows from '@/components/configforms/ConfigRows.vue';
import ConfigSaveBar from '@/components/configforms/ConfigSaveBar.vue';

type Pair = { k: string; v: string };
type Branches = { base: string; name: string; kinds: Pair[] };

const modelError = configModelError;

const toPairs = (m?: Record<string, string>): Pair[] => Object.entries(m ?? {}).map(([k, v]) => ({ k, v }));
const kvOf = (rows: Pair[]): Record<string, string> | undefined => {
  const out = Object.fromEntries(rows.filter((r) => r.k.trim()).map((r) => [r.k.trim(), r.k.trim()]));
  return Object.keys(out).length ? out : undefined;
};

// a kind's branch word is the kind itself, always (the parse refuses
// bug = "fix"), so a row is its key
// the kinds the kind field holds, for the datalist
const fieldKinds = computed(() => configModel.value?.fields['kind']?.options ?? []);
// a key the kind field doesn't hold: no issue can carry it (the flat kinds'
// field is made of these keys, so it never says so)
const notAKind = (k: string) => fieldKinds.value.length > 0 && k.trim() !== '' && !fieldKinds.value.includes(k.trim());
const strangers = computed(() => kindRows.value.map((r) => r.k.trim()).filter(notAKind));

const draft = new SectionDraft<Branches>(
  'branches',
  (m) => ({ base: m.branches.base ?? '', name: m.branches.name ?? '', kinds: toPairs(m.branches.kinds) }),
  (a, b) => a.base !== b.base || a.name !== b.name || JSON.stringify(a.kinds) !== JSON.stringify(b.kinds),
  (value) => ({ base: value.base || undefined, name: value.name || undefined, kinds: kvOf(value.kinds) }),
);

function set(patch: Partial<{ base: string; name: string }>) {
  if (draft.value.value) draft.value.value = { ...draft.value.value, ...patch };
}

const kindRows = computed({
  get: () => draft.value.value?.kinds ?? [],
  set: (rows) => {
    if (draft.value.value) draft.value.value = { ...draft.value.value, kinds: rows };
  },
});

const state = computed(() => `${kindRows.value.length} kind${kindRows.value.length === 1 ? '' : 's'} → ${draft.value.value?.name || '{number}/{n}-{slug}'}`);

function reset() {
  if (configModel.value) draft.reset(configModel.value);
}

onMounted(() => void draft.load());
</script>

<style scoped>
.set-panel { display: grid; gap: .75rem; container-type: inline-size; }
/* the forms left, the reference right, like the old editor; one column when narrow */
.set-cols { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); gap: .75rem; align-items: start; }
.set-forms { display: grid; gap: .75rem; min-width: 0; }
@container (max-width: 52rem) { .set-cols { grid-template-columns: minmax(0, 1fr); } }
h2, h3, p { margin: 0; }
.bs-lead { max-width: 42rem; color: var(--fg-muted); }
.bs-muted { color: var(--fg-muted); }
.bs-form { display: flex; gap: .75rem; flex-wrap: wrap; }
.bs-field { display: grid; gap: .25rem; font-size: .8125rem; }
.bs-field > span { color: var(--fg-muted); }
.bs-grow { flex: 1; min-width: 14rem; }
.bs-input { padding: .375rem .625rem; border: 1px solid var(--border); background: var(--bg); color: var(--fg); font: inherit; }
.bs-input:focus { outline: 2px solid var(--fg); outline-offset: 2px; border-color: var(--fg); }
.bs-opt { width: 10rem; }
.bs-pair { display: flex; align-items: center; gap: .5rem; }
.bs-bad { font-size: .75rem; font-weight: 700; color: var(--danger-fg); white-space: nowrap; }
.bs-bad-line { color: var(--danger-fg); }
</style>
