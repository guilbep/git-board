<!-- Intent, under the description: how many criteria the server read
     there, or, when there are none and it is the viewer's turn, a box to
     write them (saved under the description's own criteria heading, or a
     new one: cycle.ts withCriteria). -->
<template>
  <p class="gb-note">{{ found }}</p>
  <form v-if="!count && current && cycle.mine" class="gb-intent-form" @submit.prevent="emit('save', text.split('\n'))">
    <textarea v-model="text" class="epfl-input" rows="4" aria-label="Acceptance criteria, one per line" placeholder="One criterion per line" :disabled="busy" />
    <p class="gb-note">Written under “{{ headings[0] }}” in the description.</p>
    <div class="gb-cycle-actions">
      <button type="submit" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="busy || !text.trim()">Save the criteria</button>
    </div>
  </form>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import type { Cycle } from '@/api/board';

const props = defineProps<{ cycle: Cycle; current: boolean; busy: boolean }>();
const emit = defineEmits<{ save: [lines: string[]] }>();

const headings = computed(() => (props.cycle.stages.criteria?.length ? props.cycle.stages.criteria : ['Acceptance criteria']));
const count = computed(() => props.cycle.evidence.criteria?.length ?? 0);
const found = computed(() => {
  if (count.value) return `${count.value} ${count.value === 1 ? 'criterion' : 'criteria'} found in the description.`;
  const hs = headings.value.map((h) => `“${h}”`);
  return `No criteria: a list under ${hs.length > 1 ? `${hs.slice(0, -1).join(', ')} or ${hs.at(-1)}` : hs[0]} in the description.`;
});
const text = ref('');
</script>

<style scoped>
.gb-intent-form { display: grid; gap: .5rem; }
.gb-intent-form .gb-note { margin: 0; }
</style>
