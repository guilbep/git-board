<!-- The Verify step (the canvas's C7): the criteria of step 1 checked one by
     one with who and when, the run at the merged head, the plan marked
     delivered, then Sign off <status>, which the role's person presses once
     every criterion is checked, or Send back to in-progress. -->
<template>
  <h3 class="gb-ws-title">Is it done, as step 1 said?</h3>
  <p class="gb-note">The criteria of step 1, ticked in the description · green codes are not proof</p>

  <ul v-if="cycle.evidence.criteria?.length" class="gb-crit">
    <li v-for="(c, i) in cycle.evidence.criteria" :key="i">
      <input :id="`crit-${i}`" type="checkbox" :checked="!!checks[i]" :disabled="!current || !!checks[i] || busy" @change="emit('check', i)" />
      <label :for="`crit-${i}`">
        <b>{{ c }}</b>
        <span v-if="checks[i]" class="gb-note">{{ [checks[i]!.by, when(checks[i]!.at, true)].filter(Boolean).join(' · ') }}<template v-if="checks[i]!.commit"> · at <span class="gb-mono">{{ checks[i]!.commit!.slice(0, 7) }}</span>{{ checks[i]!.commit === merge ? ', the merge' : '' }}</template></span>
        <span v-else class="gb-note">Not checked yet</span>
      </label>
    </li>
  </ul>
  <p v-else class="gb-note">None written: a "## Acceptance criteria" list in the description.</p>

  <ul class="gb-tiles">
    <li>
      <template v-if="run">
        <span>Run at <span class="gb-mono">{{ run.commit?.slice(0, 7) || 'no commit' }}</span>{{ run.merge ? ', the merged head' : '' }}</span>
        <span class="gb-note">{{ [run.by, when(run.at, true)].filter(Boolean).join(', ') }}<template v-if="merge && !run.merge"> · none at the merge <span class="gb-mono">{{ merge.slice(0, 7) }}</span> yet</template></span>
        <button v-if="runHtml" type="button" class="gb-linkbtn" @click="open = !open">{{ open ? 'Fold the output' : 'Read the output' }}</button>
      </template>
      <template v-else>
        <span>No run <template v-if="merge">at the merged head <span class="gb-mono">{{ merge.slice(0, 7) }}</span></template><template v-else>yet</template></span>
        <span class="gb-note">Paste what you ran and what it printed.</span>
      </template>
    </li>
    <li>
      <template v-if="cycle.evidence.plan">
        <span>Plan {{ delivered ? 'marked' : 'not marked' }} <b>delivered</b></span>
        <span class="gb-note gb-mono">{{ cycle.evidence.plan.path }}</span>
      </template>
      <span v-else>No plan file</span>
    </li>
  </ul>
  <MarkdownBody v-if="open && runHtml" :html="runHtml" />
  <template v-if="current && (!run || (merge && !run.merge))">
    <textarea v-model="output" class="epfl-input" rows="3" aria-label="The run's output" placeholder="Paste what you ran and what it printed" :disabled="busy" />
    <div class="gb-cycle-actions">
      <input v-model="commit" class="epfl-input gb-mono" aria-label="At commit" :placeholder="(merge || cycle.evidence.head)?.slice(0, 7) || 'commit'" :disabled="busy" />
      <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="busy || !output.trim()" @click="postRun">Post the run</button>
    </div>
  </template>

  <ul v-if="so.length > 1" class="gb-so">
    <li v-for="t in so" :key="t.status" :class="`is-${t.state}`">
      <span class="gb-so-status">{{ t.status }}</span>
      <span v-if="t.by">{{ t.by }}</span>
      <span class="gb-so-when">{{ t.when }}</span>
    </li>
  </ul>

  <template v-if="current">
    <p class="gb-vbar">
      <span><span class="gb-eyebrow">Sign-off</span> <b>{{ label }}</b> · {{ holder }}</span>
      <span class="gb-note">{{ !total ? 'no criteria written' : left ? `waits for ${left} of ${total} criteria` : `${total} of ${total} criteria checked` }}</span>
    </p>
    <p v-if="cycle.note" class="gb-note">{{ cycle.note }}</p>
    <footer class="gb-step-foot">
      <div>
        <p class="gb-step-by">Signed off by the role's person</p>
        <p class="gb-note">{{ signoffLine(cycle, status, forge) }}</p>
      </div>
      <div class="gb-cycle-actions">
        <button v-if="cycle.stages.in_progress.length" type="button" class="epfl-btn epfl-btn-secondary" :disabled="busy" @click="emit('back')">Send back to {{ cycle.stages.in_progress[0] }}</button>
        <button v-if="can" type="button" class="epfl-btn epfl-btn-primary" :disabled="busy || left > 0" @click="emit('signoff')">Sign off {{ label }}</button>
      </div>
    </footer>
    <form v-if="needsWhy" class="gb-cycle-actions" @submit.prevent="emit('signoff', why)">
      <input v-model="why" class="epfl-input" aria-label="Why sign off without a run" placeholder="Why sign off without a run" :disabled="busy" />
      <button type="submit" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="busy || !why.trim()">Sign off anyway</button>
    </form>
  </template>
  <p v-else class="gb-gate">{{ cycle.gate }}</p>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import type { Cycle, IssueEvent } from '@/api/board';
import { criteriaLeft, mergedRun, signoffLine, tiles } from '@/cycle';
import { when } from '@/when';
import MarkdownBody from '@/components/MarkdownBody.vue';

// can: the viewer may sign off (the server's actions); current: step 7 is the
// issue's step, else it is looked at once done and only read; forge: the
// issue's copy the closing sign-off closes too ("GitHub #37")
const props = defineProps<{ cycle: Cycle; status: string; events: IssueEvent[]; current: boolean; busy: boolean; can: boolean; needsWhy: boolean; forge?: string | undefined }>();
const emit = defineEmits<{ check: [index: number]; signoff: [why?: string]; run: [output: string, commit: string]; back: [] }>();

const merge = computed(() => (props.cycle.evidence.pr?.state === 'MERGED' ? props.cycle.evidence.pr.merge_commit : undefined));
const run = computed(() => mergedRun(props.cycle));
// the run's comment in the thread (the output is its body), less its "Run at <sha>" line
const runHtml = computed(() => {
  const e = props.events.findLast((e) => e.meta?.['verify:run'] && (!run.value?.commit || e.meta['verify:commit'] === run.value.commit));
  return e?.html?.replace(/^\s*<p>Run at [^<]*<\/p>\s*/, '') ?? '';
});
const checks = computed(() => Object.fromEntries((props.cycle.evidence.checks ?? []).map((c) => [c.index, c])));
const total = computed(() => props.cycle.evidence.criteria?.length ?? 0);
const left = computed(() => criteriaLeft(props.cycle));
const delivered = computed(() => props.cycle.evidence.plan?.status === 'delivered');
const so = computed(() => tiles(props.cycle, props.status));
// with no validation status, the sign-off closes the issue: it is named for the closing status
const label = computed(() => (props.cycle.stages.validation.length ? props.status : props.cycle.stages.closing || props.status));
const holder = computed(() => {
  const role = props.cycle.stages.roles?.[`signoff:${props.status}`]?.replace(/-/g, ' ');
  const login = props.cycle.evidence.people?.[`signoff:${props.status}`];
  return [role && `the ${role}`, login].filter(Boolean).join(', ') || 'anyone';
});

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
.gb-crit { margin: 0; padding: 0; list-style: none; font-size: .875rem; }
.gb-crit li { display: grid; grid-template-columns: 1.75rem minmax(0, 1fr); gap: .625rem; padding: .875rem 0; border-top: 1px solid var(--border); }
.gb-crit input { width: 1.125rem; height: 1.125rem; margin: .125rem 0 0; accent-color: var(--fg); }
.gb-crit label { display: grid; gap: .25rem; }
.gb-vbar { display: flex; flex-wrap: wrap; justify-content: space-between; align-items: center; gap: .5rem; margin: 0; padding: .875rem 1rem; background: var(--bg-subtle); font-size: .875rem; }
.gb-vbar .gb-eyebrow { color: var(--fg-muted); margin-right: .5rem; }
</style>
