<!-- The Cycle view (the canvas's E1, E3 artboards): the spine of the seven
     steps with one line of evidence each, the workspace for the current
     step, and the status bar along the bottom. It fetches what the Issue
     view fetches: the step (usecycle.ts) and the thread it is given. -->
<template>
  <div class="gb-cycle-view" :class="{ 'is-page': page }">
    <p v-if="error" class="gb-save-error" role="alert"><GbIcon name="error" small /> {{ error }}</p>
    <template v-if="cycle">
      <p class="gb-cycle-line gb-cycle-fold" role="status">
        <span class="gb-prog" aria-hidden="true"><span v-for="i in cycle.of" :key="i" :class="['gb-tick', cycle.done || i < cycle.step ? 'is-done' : i === cycle.step ? 'is-current' : '']" /></span>
        {{ progressLine(cycle) }}<span v-if="cycle.note" class="gb-cycle-note"> · {{ cycle.note }}</span>
      </p>

      <div class="gb-cycle-body">
        <ol class="gb-spine" aria-label="The development cycle">
          <li v-for="r in rows" :key="r.n" :class="`is-${r.state}`" :aria-current="r.state === 'current' ? 'step' : undefined">
            <span class="gb-spine-n">{{ String(r.n).padStart(2, '0') }}</span>
            <span class="gb-spine-name">{{ r.name }}</span>
            <span class="gb-spine-line">{{ r.line }}</span>
          </li>
        </ol>

        <section class="gb-ws" :aria-label="`Step ${cycle.step}: ${cycle.name}`">
          <p class="gb-eyebrow">Step {{ cycle.step }} · {{ cycle.name }}<template v-if="cycle.risk"> · {{ cycle.risk }}</template></p>
          <CyclePlan v-if="cycle.step === 3 && !cycle.done" :cycle="cycle" :busy="busy || !!agentBusy" :can="has('approve-plan')" :sends="button === 'brief'" @approve="act('approve-plan')" @revise="revise" />
          <CycleVerify
            v-else-if="cycle.step === 7 && !cycle.done"
            :cycle="cycle"
            :status="status"
            :events="events"
            :busy="busy"
            :can="has('signoff')"
            :needs-why="needsWhy"
            @check="(i) => act('check', { index: i })"
            @signoff="(why) => act('signoff', why ? { why } : {})"
            @run="(output, commit) => act('verify', { output, commit })"
            @back="patch({ status: cycle.stages.in_progress[0] ?? '' })"
          />
          <template v-else>
            <h3 class="gb-ws-title">{{ cycle.done ? 'Done' : cycle.name }}</h3>
            <p class="gb-gate">{{ cycle.gate }}</p>
            <p v-if="cycle.note" class="gb-note">{{ cycle.note }}</p>
            <p v-if="!cycle.done" class="gb-note">{{ progressLine(cycle).replace(/^.*?, /, '') }}</p>
            <div v-if="card" class="gb-cycle-actions">
              <template v-for="b in card.buttons" :key="b.label">
                <a v-if="b.action === 'review' && cycle.evidence.pr?.url" :href="cycle.evidence.pr.url" target="_blank" rel="noopener noreferrer" class="epfl-btn epfl-btn-primary epfl-btn-sm">
                  {{ b.label }} #{{ cycle.evidence.pr.number }}<span class="gb-sr">(opens in a new tab)</span>
                </a>
                <button v-else-if="b.action !== 'review'" type="button" :class="['epfl-btn', 'epfl-btn-sm', b.action === 'set-risk' ? 'epfl-btn-secondary' : 'epfl-btn-primary']" :disabled="busy" @click="press(b)">
                  {{ b.label }}
                </button>
              </template>
            </div>
          </template>
        </section>
      </div>

      <footer class="gb-cycle-bar" aria-label="Status">
        <span v-for="s in bar" :key="s">{{ s }}</span>
      </footer>
      <div v-if="agent" class="gb-agent">
        <span v-if="agent.reason" class="gb-note">{{ agent.reason }}</span>
        <button v-else-if="button === 'start'" type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="!!agentBusy" @click="run('go', '')">
          {{ agentBusy === 'go' ? 'Starting…' : 'Start an agent' }}
        </button>
        <button v-else type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :aria-expanded="briefing" :disabled="!!agentBusy" @click="briefing = !briefing">Brief the agent</button>
        <span v-if="agentNote" class="gb-note" role="status">{{ agentNote }}</span>
        <form v-if="briefing && button === 'brief'" class="gb-agent-brief" @submit.prevent="run('brief', briefText.trim())">
          <textarea v-model="briefText" class="epfl-input" rows="3" aria-label="What the agent should do" :disabled="!!agentBusy" />
          <button type="submit" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="!!agentBusy || !briefText.trim()">{{ agentBusy === 'brief' ? 'Sending…' : 'Send' }}</button>
        </form>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { agentAction, getIssueAgent, type Forge, type IssueAgent, type IssueEvent } from '@/api/board';
import { cardFor, progressLine, spine, statusBar } from '@/cycle';
import { message } from '@/issue';
import { agentButton, agentWords } from '@/worktrees';
import { useCycle } from '@/usecycle';
import CyclePlan from '@/components/CyclePlan.vue';
import CycleVerify from '@/components/CycleVerify.vue';
import GbIcon from '@/components/GbIcon.vue';

const props = defineProps<{ id: string; status: string; labels: string[]; events: IssueEvent[]; forges?: Forge[] | undefined; page?: boolean }>();
const emit = defineEmits<{ changed: []; comment: [body: string] }>();

const { cycle, error, busy, needsWhy, has, act, patch, press } = useCycle(props, () => emit('changed'));
const card = computed(() => (cycle.value ? cardFor(cycle.value, props.status) : null));

// who set the risk: the last label change adding one, from the thread
const riskBy = computed(() => props.events.findLast((e) => e.kind === 'labels' && e.added?.some((l) => l.startsWith('risk:')))?.author.name);
const rows = computed(() => (cycle.value ? spine(cycle.value, riskBy.value) : []));
// what waits for the forges, as the side counts it; null without a forge
const waiting = computed(() => (props.forges?.length ? props.events.filter((e) => e.pending?.length || e.edit_pending?.length).length : null));
const bar = computed(() => {
  if (!cycle.value) return [];
  const out = statusBar(cycle.value, waiting.value);
  const a = agent.value;
  if (a?.worktree) out.push(a.worktree.path.replace(/^.*\//, ''));
  if (a && !a.reason) out.push(agentWords(a.session, !!a.worktree));
  return out;
});

// the agent on this issue (wtx, internal/server/worktrees.go): read with
// every cycle load; Start an agent without a worktree, Brief it with one
const agent = ref<IssueAgent | null>(null);
const agentBusy = ref<'' | 'go' | 'brief'>('');
const agentNote = ref('');
const briefing = ref(false);
const briefText = ref('');
const button = computed(() => (agent.value ? agentButton(agent.value) : null));
watch(cycle, () => void getIssueAgent(props.id).then((a) => (agent.value = a), () => (agent.value = null)));

async function run(action: 'go' | 'brief', brief: string) {
  agentBusy.value = action;
  agentNote.value = '';
  try {
    agent.value = await agentAction(props.id, action, brief);
    agentNote.value = action === 'go' ? `Session ${agent.value.session?.session ?? agent.value.branch} started.` : 'Sent to the agent.';
    briefing.value = false;
    briefText.value = '';
  } catch (e) {
    agentNote.value = message(e);
  } finally {
    agentBusy.value = '';
  }
}

// Ask the agent to revise: the comment (M2), and a brief when it has a worktree
function revise(body: string) {
  emit('comment', body);
  if (button.value === 'brief') void run('brief', body);
}
</script>
