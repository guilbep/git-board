<!-- The Plan workspace (the canvas's E1): the plan file as it is written,
     who drafted it, the gate, Approve the plan or Ask the agent to revise
     (a comment on the issue). -->
<template>
  <template v-if="plan">
    <h3 class="gb-ws-title">{{ title }}</h3>
    <p class="gb-plan-meta">
      <span class="gb-mono">{{ plan.path }}</span>
      <span v-if="plan.status">status: {{ plan.status }}</span>
      <a v-if="pr" :href="pr.url" target="_blank" rel="noopener noreferrer">plan PR #{{ pr.number }}<span class="gb-sr">(opens in a new tab)</span></a>
    </p>
    <!-- ponytail: the whole file, clamped, rather than its summary, options and why parsed out -->
    <MarkdownBody v-if="html" :html="html" :class="['gb-clamp', { 'is-open': open }]" />
    <p class="gb-cycle-actions">
      <button type="button" class="gb-linkbtn" @click="open = !open">{{ open ? 'Fold the plan' : 'Read the whole plan' }}</button>
    </p>
  </template>
  <template v-else>
    <h3 class="gb-ws-title">No plan file yet</h3>
    <p class="gb-note">A branch named plan/&lt;issue&gt;-…, with a plan whose front matter says issue: &lt;number&gt;, in the plans directory.</p>
  </template>

  <p class="gb-gate">{{ cycle.gate }}</p>
  <p v-if="drafted" class="gb-note">{{ drafted }}</p>
  <p v-if="cycle.note" class="gb-note">{{ cycle.note }}</p>

  <div class="gb-cycle-actions">
    <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :aria-expanded="revising" :disabled="busy" @click="revising = !revising">Ask the agent to revise</button>
    <button v-if="can" type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="busy" @click="emit('approve')">Approve the plan</button>
  </div>
  <form v-if="revising" class="gb-revise" @submit.prevent="send">
    <textarea v-model="ask" class="epfl-input" rows="3" aria-label="What to revise" placeholder="What the plan should change" :disabled="busy" />
    <div class="gb-cycle-actions">
      <button type="submit" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="busy || !ask.trim()">Post the request</button>
      <span class="gb-note">A comment on the issue: the agent reads it and revises the plan.</span>
    </div>
  </form>
  <p class="gb-note">Once it’s approved, step 4 starts: the agent writes the failing test first.</p>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import type { Cycle } from '@/api/board';
import MarkdownBody from '@/components/MarkdownBody.vue';

const props = defineProps<{ cycle: Cycle; busy: boolean; can: boolean }>(); // can: the viewer may approve (the server's actions)
const emit = defineEmits<{ approve: []; revise: [body: string] }>();

const plan = computed(() => props.cycle.evidence.plan);
const pr = computed(() => props.cycle.evidence.plan_pr);
// the file's first heading is the plan's title; the file name otherwise
const h1 = /^\s*<h1[^>]*>(.*?)<\/h1>\s*/;
const title = computed(() => plan.value?.html?.match(h1)?.[1]?.replace(/<[^>]+>/g, '') || plan.value?.path.replace(/^.*\//, '') || 'Plan');
// the file's first heading is the title above: shown once
const html = computed(() => plan.value?.html?.replace(h1, '') ?? '');
const drafted = computed(() => {
  const p = plan.value;
  if (!p?.by) return '';
  const where = props.cycle.evidence.plan_branch ? ` in ${props.cycle.evidence.plan_branch}` : '';
  const reviews = pr.value?.reviews?.length ?? 0;
  return `Drafted by ${p.by}${where}${reviews ? `, ${reviews} review${reviews === 1 ? '' : 's'} so far` : ''}.`;
});

const open = ref(false);
const revising = ref(false);
const ask = ref('');
function send() {
  emit('revise', ask.value.trim());
  ask.value = '';
  revising.value = false;
}
</script>

<style scoped>
.gb-plan-meta { display: flex; flex-wrap: wrap; gap: .25rem 1rem; margin: 0; font-size: .8125rem; color: var(--fg-muted); }
.gb-revise { display: grid; gap: .5rem; }
</style>
