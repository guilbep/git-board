<!-- The Plan workspace (the canvas's E1): the plan file as it is written,
     who drafted it, the gate, Approve the plan or Ask the agent to revise
     (a comment on the issue). -->
<template>
  <p v-if="strip" class="gb-plan-agent">
    <template v-for="(w, i) in strip.words" :key="w"><template v-if="i"> · </template>{{ w }}</template>
    <template v-if="strip.draft"> · <a :href="strip.draft.url" target="_blank" rel="noopener noreferrer">Draft PR #{{ strip.draft.number }}, the plan only<span class="gb-sr">(opens in a new tab)</span></a></template>
  </p>
  <template v-if="plan">
    <h3 class="gb-ws-title">{{ title }}</h3>
    <p class="gb-plan-meta">
      <span class="gb-mono">{{ plan.path }}</span>
      <span v-if="plan.issue">issue: {{ plan.issue }}</span>
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
    <p class="gb-note">Start an agent: it drafts the plan in plan mode, and once you accept it in its pane, commits it to the plans directory with issue: &lt;number&gt; in its front matter. A branch named plan/&lt;issue&gt;-… works too.</p>
  </template>

  <p class="gb-gate">{{ gate }}</p>
  <p v-if="drafted || approval" class="gb-note">{{ drafted }}<template v-if="approval"> Approved by {{ approval.by }}, {{ on(approval.at) }}.</template></p>
  <template v-if="current">
    <p v-if="cycle.note" class="gb-note">{{ cycle.note }}</p>

    <div class="gb-cycle-actions">
      <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :aria-expanded="revising" :disabled="busy" @click="revising = !revising">Ask the agent to revise</button>
      <button v-if="can" type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="busy" @click="emit('approve')">Approve the plan</button>
    </div>
    <form v-if="revising" class="gb-revise" @submit.prevent="send">
      <textarea v-model="ask" class="epfl-input" rows="3" aria-label="What to revise" placeholder="What the plan should change" :disabled="busy" />
      <div class="gb-cycle-actions">
        <button type="submit" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="busy || !ask.trim()">Post the request</button>
        <span class="gb-note">{{ sends ? 'Posted on the issue and sent to the agent.' : 'Posted on the issue.' }}</span>
      </div>
    </form>
    <p class="gb-note">{{ approver }}: step 4 starts<template v-if="sends">, and the agent is briefed to write the failing test, in the same branch: accept that in its pane</template>.</p>
  </template>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import type { Cycle, IssueAgent } from '@/api/board';
import { on } from '@/when';
import { agentWords } from '@/worktrees';
import MarkdownBody from '@/components/MarkdownBody.vue';

// can: the viewer may approve (the server's actions); sends: the request also briefs the agent;
// current: step 3 is the issue's step, else it is looked at from the step bar and only read
// agent: the issue's wtx agent, for the strip above the plan (C3)
const props = defineProps<{ cycle: Cycle; gate: string; current: boolean; busy: boolean; can: boolean; sends?: boolean; agent?: IssueAgent | null }>();
const emit = defineEmits<{ approve: []; revise: [body: string] }>();

const plan = computed(() => props.cycle.evidence.plan);
const approval = computed(() => props.cycle.evidence.plan_approval);
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

// the worktree, the agent's state, and the draft PR that holds the plan
const strip = computed(() => {
  const a = props.agent;
  const d = props.cycle.evidence.pr;
  const draft = d?.state === 'OPEN' && d.draft ? d : null;
  if (!a?.worktree && !draft) return null;
  const words = a?.worktree ? [`Worktree ${a.worktree.path.replace(/^.*\//, '')}`, agentWords(a.session, true)] : [];
  return { words, draft };
});
const approver = computed(() => {
  const w = props.cycle.waits;
  if (w.role !== 'plan' || props.can) return 'You approve';
  return `${w.login || 'The plan role'} approves`;
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
.gb-plan-agent { margin: 0; padding: .375rem .75rem; border-radius: 4px; background: var(--bg-subtle); font-size: .8125rem; color: var(--fg-muted); }
</style>
