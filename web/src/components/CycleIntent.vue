<!-- Intent, under the description: how many criteria the server read
     there, the warnings where it reads them differently from how they
     look (cycle.Criteria), and, at step 1 for whoever may act, the list to
     edit, a row each (saved as "- [ ]" boxes under the description's own
     criteria heading, or a new one: cycle.ts withCriteria), or, with none,
     the issue accepted as it is (one from before git-board:
     intent:criteria=none). -->
<template>
  <p class="gb-note">{{ found }}</p>
  <ul v-if="cycle.evidence.criteria_problems?.length" class="gb-env-drift gb-intent-warn" aria-label="Criteria warnings">
    <li v-for="p in cycle.evidence.criteria_problems" :key="p">{{ p }}</li>
  </ul>
  <form v-if="current && (cycle.mine || canAccept)" class="gb-intent-form" @submit.prevent="save">
    <ol class="gb-intent-rows">
      <li v-for="(_, i) in rows" :key="i">
        <input v-model="rows[i]" class="epfl-input" :aria-label="`Criterion ${i + 1}`" :disabled="busy" />
        <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :aria-label="`Remove criterion ${i + 1}`" :disabled="busy" @click="rows.splice(i, 1)">✕</button>
      </li>
      <li>
        <input v-model="draft" class="epfl-input" aria-label="Add a criterion" placeholder="Add a criterion, Enter adds it" :disabled="busy" @keydown.enter.prevent="add" @paste="paste" />
      </li>
    </ol>
    <p class="gb-note">Written as checkboxes under “{{ headings[0] }}” in the description.</p>
    <div class="gb-cycle-actions">
      <button type="submit" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="busy || !changed">Save the criteria</button>
      <button v-if="canAccept && !count" type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="busy" @click="emit('accept')">Go on without criteria</button>
    </div>
  </form>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import type { Cycle } from '@/api/board';
import { criterionOf } from '@/cycle';

const props = defineProps<{ cycle: Cycle; current: boolean; busy: boolean; canAccept: boolean }>();
const emit = defineEmits<{ save: [lines: string[]]; accept: [] }>();

const headings = computed(() => (props.cycle.stages.criteria?.length ? props.cycle.stages.criteria : ['Acceptance criteria']));
const count = computed(() => props.cycle.evidence.criteria?.length ?? 0);
const found = computed(() => {
  if (count.value) return `${count.value} ${count.value === 1 ? 'criterion' : 'criteria'} found in the description.`;
  if (props.cycle.evidence.no_criteria) return 'Accepted without acceptance criteria.';
  const hs = headings.value.map((h) => `“${h}”`);
  return `No criteria: a list under ${hs.length > 1 ? `${hs.slice(0, -1).join(', ')} or ${hs.at(-1)}` : hs[0]} in the description.`;
});
// the rows, reset when the server reads other criteria (a save, an edit)
const saved = computed(() => props.cycle.evidence.criteria ?? []);
const rows = ref<string[]>([]);
const draft = ref('');
watch(() => saved.value.join('\n'), () => { rows.value = [...saved.value]; draft.value = ''; }, { immediate: true });
const next = computed(() => [...rows.value, draft.value].map((l) => criterionOf(l).text).filter(Boolean));
const changed = computed(() => next.value.length > 0 && next.value.join('\n') !== saved.value.join('\n'));
function add() {
  const t = criterionOf(draft.value).text;
  if (t) rows.value.push(t);
  draft.value = '';
}
// a pasted list: a row per line
function paste(e: ClipboardEvent) {
  const lines = (e.clipboardData?.getData('text') ?? '').split('\n').map((l) => criterionOf(l).text).filter(Boolean);
  if (lines.length < 2) return;
  e.preventDefault();
  rows.value.push(...lines);
}
const save = () => emit('save', next.value);
</script>

<style scoped>
.gb-intent-form { display: grid; gap: .5rem; }
.gb-intent-form .gb-note { margin: 0; }
.gb-intent-rows { display: grid; gap: .375rem; margin: 0; padding-left: 1.75rem; }
.gb-intent-rows li { display: flex; gap: .375rem; align-items: center; }
.gb-intent-rows .epfl-input { flex: 1; min-width: 0; }
.gb-intent-warn { display: grid; gap: .25rem; margin: 0; padding-left: 1.75rem; }
</style>
