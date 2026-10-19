<!-- The issue's step in the development cycle, under the title: the
     progress line for everyone, and the card for whoever's turn it is
     (the server decides: internal/cycle). -->
<template>
  <div v-if="cycle" class="gb-cycle">
    <p class="gb-cycle-line" role="status">
      <span class="gb-prog" aria-hidden="true"><span v-for="i in cycle.of" :key="i" :class="['gb-tick', cycle.done || i < cycle.step ? 'is-done' : i === cycle.step ? 'is-current' : '']" /></span>
      {{ progressLine(cycle) }}<span v-if="cycle.note" class="gb-cycle-note"> · {{ cycle.note }}</span>
    </p>

    <div v-if="card" class="gb-cycle-card" role="region" :aria-label="card.title">
      <p class="gb-cycle-title">{{ card.title }}</p>
      <p class="gb-cycle-gate">{{ card.gate }}</p>

      <div v-if="has('verify')" class="gb-cycle-run">
        <ul v-if="cycle.evidence.criteria?.length" class="gb-cycle-criteria" aria-label="Acceptance criteria">
          <li v-for="(c, i) in cycle.evidence.criteria" :key="i">
            <label>
              <input type="checkbox" :checked="!!checkedBy(i)" :disabled="!!checkedBy(i) || busy" @change="act('check', { index: i })" />
              {{ c }}<span v-if="checkedBy(i)" class="gb-cycle-by"> · by {{ checkedBy(i) }}</span>
            </label>
          </li>
        </ul>
        <textarea v-model="output" class="epfl-input" rows="3" aria-label="The run's output" placeholder="Paste what you ran and what it printed" :disabled="busy" />
        <div class="gb-cycle-actions">
          <input v-model="commit" class="epfl-input gb-mono" aria-label="At commit" :placeholder="cycle.evidence.head?.slice(0, 7) || 'commit'" :disabled="busy" />
          <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="busy || !output.trim()" @click="postRun">Post the run</button>
        </div>
      </div>

      <div class="gb-cycle-actions">
        <template v-for="b in card.buttons" :key="b.label">
          <a v-if="b.action === 'review' && cycle.evidence.pr?.url" :href="cycle.evidence.pr.url" target="_blank" rel="noopener noreferrer" class="epfl-btn epfl-btn-primary epfl-btn-sm">
            {{ b.label }} #{{ cycle.evidence.pr.number }}<span class="gb-sr">(opens in a new tab)</span>
          </a>
          <button v-else-if="b.action === 'review'" type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" @click="toDevelopment">{{ b.label }}</button>
          <button v-else type="button" :class="['epfl-btn', 'epfl-btn-sm', b.action === 'set-risk' ? 'epfl-btn-secondary' : 'epfl-btn-primary']" :disabled="busy" @click="press(b)">
            {{ b.label }}
          </button>
        </template>
      </div>
      <form v-if="needsWhy" class="gb-cycle-actions" @submit.prevent="act('signoff', { why })">
        <input v-model="why" class="epfl-input" aria-label="Why sign off without a run" placeholder="Why sign off without a run" :disabled="busy" />
        <button type="submit" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="busy || !why.trim()">Sign off anyway</button>
      </form>
    </div>
    <p v-if="error" class="gb-save-error" role="alert"><GbIcon name="error" small /> {{ error }}</p>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { cardFor, progressLine } from '@/cycle';
import { useCycle } from '@/usecycle';
import GbIcon from '@/components/GbIcon.vue';

const props = defineProps<{ id: string; status: string; labels: string[] }>();
const emit = defineEmits<{ changed: [] }>();

const { cycle, error, busy, output, commit, why, needsWhy, has, checkedBy, act, press, postRun } = useCycle(props, () => emit('changed'));
const card = computed(() => (cycle.value ? cardFor(cycle.value, props.status) : null));

function toDevelopment() {
  document.getElementById('side-dev')?.scrollIntoView({ behavior: 'smooth', block: 'start' });
}
</script>

<style scoped>
.gb-cycle { display: grid; gap: .5rem; margin-top: .75rem; }
.gb-cycle-line { display: flex; flex-wrap: wrap; align-items: center; gap: .25rem .75rem; margin: 0; font-size: .875rem; font-weight: 700; }
.gb-cycle-note { font-weight: 400; color: var(--fg-muted); }
.gb-cycle-card { display: grid; gap: .5rem; padding: .625rem .75rem; border: 1px solid var(--border); border-left: 3px solid var(--fg); background: var(--bg-subtle); font-size: .875rem; }
.gb-cycle-title { margin: 0; font-weight: 700; }
.gb-cycle-gate { margin: 0; font-size: .8125rem; color: var(--fg-muted); }
.gb-cycle-run { display: grid; gap: .5rem; }
.gb-cycle-criteria { display: grid; gap: .25rem; margin: 0; padding: 0; list-style: none; }
.gb-cycle-by { color: var(--fg-muted); }
.gb-cycle-actions { display: flex; flex-wrap: wrap; align-items: center; gap: .5rem; }
.gb-cycle-actions .epfl-input { flex: 1 1 10rem; min-width: 0; }
</style>
