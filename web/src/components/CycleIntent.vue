<!-- The Intent workspace: the acceptance criteria the description lists,
     or, when there are none and it is the viewer's turn, a box to write
     them (saved under an "Acceptance criteria" heading in the description,
     cycle.ts withCriteria). -->
<template>
  <h3 class="gb-ws-title">Acceptance criteria</h3>
  <ul v-if="criteria.length" class="gb-ws-list">
    <li v-for="(c, i) in criteria" :key="i">{{ c }}</li>
  </ul>
  <template v-else>
    <p class="gb-note">None yet.</p>
    <form v-if="current && cycle.mine" class="gb-intent-form" @submit.prevent="emit('save', text.split('\n'))">
      <textarea v-model="text" class="epfl-input" rows="4" aria-label="Acceptance criteria, one per line" placeholder="One criterion per line" :disabled="busy" />
      <p class="gb-note">Written under an “Acceptance criteria” heading in the description.</p>
      <div class="gb-cycle-actions">
        <button type="submit" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="busy || !text.trim()">Save the criteria</button>
      </div>
    </form>
  </template>

  <p class="gb-gate">{{ gate }}</p>
  <template v-if="current">
    <p v-if="cycle.note" class="gb-note">{{ cycle.note }}</p>
    <p v-if="!cycle.done" class="gb-note">{{ progressLine(cycle).replace(/^.*?, /, '') }}</p>
  </template>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import type { Cycle } from '@/api/board';
import { progressLine } from '@/cycle';

const props = defineProps<{ cycle: Cycle; gate: string; current: boolean; busy: boolean }>();
const emit = defineEmits<{ save: [lines: string[]] }>();

const criteria = computed(() => props.cycle.evidence.criteria ?? []);
const text = ref('');
</script>

<style scoped>
.gb-intent-form { display: grid; gap: .5rem; }
.gb-intent-form .gb-note { margin: 0; }
</style>
