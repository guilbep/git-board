<!-- The Verify workspace (the canvas's E3): the full run, the acceptance
     criteria checked one by one, the plan marked delivered, the sign-offs
     one tile per validation status, then Sign off <status> or Send back
     to in-progress. -->
<template>
  <h3 class="gb-ws-title">Run it, read it, then sign it off</h3>

  <section class="gb-vsec" aria-labelledby="v-run">
    <h4 id="v-run" class="gb-vsec-title">The full run</h4>
    <template v-if="run">
      <p class="gb-note">{{ run.by }}<template v-if="run.commit"> at <span class="gb-mono">{{ run.commit.slice(0, 7) }}</span></template>, {{ when(run.at, true) }}</p>
      <MarkdownBody v-if="runHtml" :html="runHtml" :class="['gb-clamp', { 'is-open': open }]" />
      <p v-if="runHtml" class="gb-cycle-actions">
        <button type="button" class="gb-linkbtn" @click="open = !open">{{ open ? 'Fold the output' : 'Read the whole output' }}</button>
      </p>
    </template>
    <template v-else>
      <p class="gb-note">No run yet at this commit.</p>
      <textarea v-model="output" class="epfl-input" rows="3" aria-label="The run's output" placeholder="Paste what you ran and what it printed" :disabled="busy" />
      <div class="gb-cycle-actions">
        <input v-model="commit" class="epfl-input gb-mono" aria-label="At commit" :placeholder="cycle.evidence.head?.slice(0, 7) || 'commit'" :disabled="busy" />
        <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="busy || !output.trim()" @click="postRun">Post the run</button>
      </div>
    </template>
  </section>

  <section class="gb-vsec" aria-labelledby="v-criteria">
    <h4 id="v-criteria" class="gb-vsec-title">Acceptance criteria, from the issue</h4>
    <ul v-if="cycle.evidence.criteria?.length" class="gb-vlist">
      <li v-for="(c, i) in cycle.evidence.criteria" :key="i">
        <label>
          <input type="checkbox" :checked="!!checkedBy(i)" :disabled="!!checkedBy(i) || busy" @change="emit('check', i)" />
          {{ c }}<span v-if="checkedBy(i)" class="gb-note"> · checked by {{ checkedBy(i) }}</span>
        </label>
      </li>
    </ul>
    <p v-else class="gb-note">None written: a "## Acceptance criteria" list in the description.</p>
    <p class="gb-vline" :class="{ 'is-done': delivered }">
      <span aria-hidden="true">{{ delivered ? '✓' : '○' }}</span>
      <template v-if="delivered">Plan marked delivered<template v-if="cycle.evidence.plan?.by"> by {{ cycle.evidence.plan.by }}</template> · <span class="gb-mono">{{ cycle.evidence.plan?.path }}</span></template>
      <template v-else-if="cycle.evidence.plan">Plan not marked delivered · <span class="gb-mono">{{ cycle.evidence.plan.path }}</span></template>
      <template v-else>No plan file</template>
    </p>
  </section>

  <section class="gb-vsec" aria-labelledby="v-so">
    <h4 id="v-so" class="gb-vsec-title">Sign-offs</h4>
    <ul v-if="so.length" class="gb-so">
      <li v-for="t in so" :key="t.status" :class="`is-${t.state}`">
        <span class="gb-so-status">{{ t.status }}</span>
        <span v-if="t.by">{{ t.by }}</span>
        <span class="gb-so-when">{{ t.when }}</span>
      </li>
    </ul>
    <p v-else class="gb-note">No validation status configured: [cycle] validation in the project file.</p>
  </section>

  <p class="gb-gate">{{ cycle.gate }}</p>
  <p v-if="cycle.note" class="gb-note">{{ cycle.note }}</p>
  <div class="gb-cycle-actions">
    <button v-if="can" type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="busy" @click="emit('signoff')">Sign off {{ status }}</button>
    <button v-if="cycle.stages.in_progress.length" type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="busy" @click="emit('back')">Send back to {{ cycle.stages.in_progress[0] }}</button>
  </div>
  <form v-if="needsWhy" class="gb-cycle-actions" @submit.prevent="emit('signoff', why)">
    <input v-model="why" class="epfl-input" aria-label="Why sign off without a run" placeholder="Why sign off without a run" :disabled="busy" />
    <button type="submit" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="busy || !why.trim()">Sign off anyway</button>
  </form>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import type { Cycle, IssueEvent } from '@/api/board';
import { tiles } from '@/cycle';
import { when } from '@/when';
import MarkdownBody from '@/components/MarkdownBody.vue';

const props = defineProps<{ cycle: Cycle; status: string; events: IssueEvent[]; busy: boolean; can: boolean; needsWhy: boolean }>(); // can: the viewer may sign off (the server's actions)
const emit = defineEmits<{ check: [index: number]; signoff: [why?: string]; run: [output: string, commit: string]; back: [] }>();

// the newest run, and its comment in the thread (the output is its body)
const run = computed(() => props.cycle.evidence.runs?.at(-1));
const runEvent = computed(() => props.events.findLast((e) => e.meta?.['verify:run']));
// the comment opens with "Run at <sha>", said above already
const runHtml = computed(() => runEvent.value?.html?.replace(/^\s*<p>Run at [^<]*<\/p>\s*/, '') ?? '');
const checkedBy = (i: number) => props.cycle.evidence.checks?.find((c) => c.index === i)?.by;
const delivered = computed(() => props.cycle.evidence.plan?.status === 'delivered');
const so = computed(() => tiles(props.cycle, props.status));

const open = ref(false);
const output = ref('');
const commit = ref('');
const why = ref('');
function postRun() {
  emit('run', output.value, commit.value.trim());
  output.value = commit.value = '';
}
</script>

<style scoped>
.gb-vsec { display: grid; gap: .5rem; }
.gb-vsec-title { margin: 0; font-size: .875rem; }
.gb-vlist { display: grid; gap: .25rem; margin: 0; padding: 0; list-style: none; font-size: .875rem; }
.gb-vline { display: flex; gap: .5rem; margin: 0; font-size: .875rem; color: var(--fg-muted); }
.gb-vline.is-done { color: var(--gb-done); }
</style>
