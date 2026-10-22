<!-- The issue's frame under its title (the canvas's C1 to C7): the step bar,
     the shown step as the main section, the discussion beside it. The step
     shown is the issue's (internal/cycle), or a done one picked on the bar;
     a later step is locked and says why. The panel fills the slots: intent
     (the description), review (the Changes view, step 6 on the page), talk
     (the discussion of the step shown). -->
<template>
  <p v-if="error" class="gb-save-error" role="alert"><GbIcon name="error" small /> {{ error }}</p>
  <nav v-if="cycle" class="gb-stepbar" aria-label="The development cycle">
    <ol class="gb-bar-steps">
      <li v-for="r in rows" :key="r.n" :class="[`is-${r.state}`, { 'is-shown': r.n === n }]" :aria-current="r.state === 'current' ? 'step' : undefined">
        <button type="button" :disabled="r.state === 'locked'" :aria-pressed="r.n === n" @click="show(r.n)">
          <span class="gb-bar-n"><GbIcon v-if="r.state === 'locked'" name="lock" small /><template v-else>{{ r.n }}</template></span>
          <span class="gb-bar-name">{{ r.name }}</span>
          <span class="gb-bar-line">{{ r.line }}</span>
        </button>
      </li>
    </ol>
  </nav>

  <div class="gb-cols">
    <section class="gb-shown" :aria-label="`Step ${n}: ${row?.name ?? 'Intent'}`">
      <p class="gb-eyebrow">Step {{ n }} · {{ row?.name ?? 'Intent' }}<template v-if="risk"> · {{ risk }}</template></p>
      <slot v-if="n === 1" name="intent" />

      <CyclePlan
        v-if="cycle && n === 3 && !cycle.skipped?.includes(3) && (current || cycle.evidence.plan)"
        :cycle="cycle"
        :gate="row?.gate ?? ''"
        :current="current"
        :busy="busy || !!agentBusy"
        :can="has('approve-plan')"
        :sends="button === 'brief'"
        :agent="agent"
        @approve="approve"
        @revise="revise"
      />
      <CycleVerify
        v-else-if="cycle && n === 7 && current"
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
      <template v-else-if="cycle && row">
        <CycleIntent v-if="n === 1" :cycle="cycle" :current="current" :busy="busy" @save="saveCriteria" />
        <ul v-if="n === 4 && cycle.evidence.tests?.length" class="gb-ws-list">
          <li v-for="t in cycle.evidence.tests" :key="t.path"><span class="gb-mono">{{ t.path }}</span><template v-if="t.commit"> at <span class="gb-mono">{{ t.commit.slice(0, 7) }}</span></template></li>
        </ul>
        <p v-else-if="n !== 1 && row.state === 'done' && row.line !== row.gate" class="gb-note">{{ row.line }}</p>
        <p class="gb-gate">{{ row.gate }}</p>
        <template v-if="current">
          <p v-if="cycle.note" class="gb-note">{{ cycle.note }}</p>
          <p class="gb-note">{{ cap(turn(cycle)) }}.</p>
          <div v-if="card" class="gb-cycle-actions">
            <template v-for="b in card.buttons" :key="b.label">
              <a v-if="b.action === 'review' && cycle.evidence.pr?.url" :href="cycle.evidence.pr.url" target="_blank" rel="noopener noreferrer" class="epfl-btn epfl-btn-primary epfl-btn-sm">
                {{ b.label }} #{{ cycle.evidence.pr.number }}<span class="gb-sr">(opens in a new tab)</span>
              </a>
              <button v-else-if="b.action !== 'review'" type="button" :class="['epfl-btn', 'epfl-btn-sm', b.action === 'set-risk' ? 'epfl-btn-secondary' : 'epfl-btn-primary']" :disabled="busy" @click="pressed(b)">
                {{ b.label }}
              </button>
            </template>
          </div>
        </template>
      </template>
      <slot v-if="n === 6" name="review" />

      <p v-if="cycle && n !== cycle.step" class="gb-cycle-actions">
        <button type="button" class="gb-linkbtn" @click="shown = null">Back to step {{ cycle.step }} · {{ cycle.name }}</button>
      </p>
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
      <footer v-if="bar.length" class="gb-cycle-bar" aria-label="Status">
        <span v-for="s in bar" :key="s">{{ s }}</span>
      </footer>
    </section>
    <aside class="gb-talk" aria-label="Discussion">
      <slot name="talk" :step="n" />
    </aside>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { agentAction, getIssueAgent, type Forge, type IssueAgent, type IssueEvent } from '@/api/board';
import { cardFor, steps, statusBar, turn, withCriteria, type CardButton } from '@/cycle';
import { message } from '@/issue';
import { agentButton, agentWords } from '@/worktrees';
import { useCycle } from '@/usecycle';
import CycleIntent from '@/components/CycleIntent.vue';
import CyclePlan from '@/components/CyclePlan.vue';
import CycleVerify from '@/components/CycleVerify.vue';
import GbIcon from '@/components/GbIcon.vue';

const props = defineProps<{ id: string; status: string; labels: string[]; events: IssueEvent[]; forges?: Forge[] | undefined }>();
const emit = defineEmits<{ changed: []; comment: [body: string, step: number] }>();

const description = computed(() => props.events.find((e) => e.kind === 'created'));
const { cycle, error, busy, needsWhy, has, act, patch, edit, press } = useCycle(
  { get id() { return props.id; }, get labels() { return props.labels; }, get description() { return description.value?.body ?? ''; } },
  () => emit('changed'),
);
const card = computed(() => (cycle.value ? cardFor(cycle.value, props.status) : null));
const cap = (s: string) => s.charAt(0).toUpperCase() + s.slice(1);

// who set the risk: the last label change adding one, from the thread
const riskBy = computed(() => props.events.findLast((e) => e.kind === 'labels' && e.added?.some((l) => l.startsWith('risk:')))?.author.name);
const rows = computed(() => (cycle.value ? steps(cycle.value, riskBy.value) : []));
const risk = computed(() => cycle.value?.risk ?? cycle.value?.evidence.risk);

// the step shown, picked on the bar; null is the issue's step, and the
// frame goes back to it when the step moves or the issue changes
const shown = ref<number | null>(null);
watch([() => props.id, () => cycle.value?.step], () => (shown.value = null));
const n = computed(() => shown.value ?? cycle.value?.step ?? 1);
const row = computed(() => rows.value[n.value - 1]);
const current = computed(() => !!cycle.value && n.value === cycle.value.step && !cycle.value.done);
const show = (step: number) => (shown.value = step === cycle.value?.step ? null : step);
defineExpose({ show });

// Intent's criteria go in the description, under its own heading
function saveCriteria(lines: string[]) {
  const d = description.value;
  if (d) void edit(d.id, withCriteria(d.body ?? '', lines, cycle.value?.stages.criteria), d.body ?? '');
}
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
const agentBusy = ref<'' | 'go' | 'brief' | 'approve'>('');
const agentNote = ref('');
const briefing = ref(false);
const briefText = ref('');
const button = computed(() => (agent.value ? agentButton(agent.value) : null));
watch(cycle, () => void getIssueAgent(props.id).then((a) => (agent.value = a), () => (agent.value = null)));

async function run(action: 'go' | 'brief' | 'approve', brief: string) {
  agentBusy.value = action;
  agentNote.value = '';
  try {
    agent.value = await agentAction(props.id, action, brief);
    agentNote.value = { go: `Session ${agent.value.session?.session ?? agent.value.branch} started.`, brief: 'Sent to the agent.', approve: 'Sent to the agent: the plan is approved. Accept its step 4 in its pane.' }[action];
    // go at low risk, or with the plan approved, moved the status
    if (action === 'go') emit('changed');
    briefing.value = false;
    briefText.value = '';
  } catch (e) {
    agentNote.value = message(e);
  } finally {
    agentBusy.value = '';
  }
}

// Approve the plan on the board, then brief the agent on to step 4.
// ponytail: wtx brief restarts the pane in plan mode, hence the pane's
// accept; worktrees.go's buildBrief ends that once wtx has --build
async function approve() {
  if ((await act('approve-plan')) && button.value === 'brief') void run('approve', '');
}

// Start work with wtx and no worktree yet: wtx go, which moves the status
// once the agent is started; without wtx, the status alone
const pressed = (b: CardButton) => (b.action === 'start' && button.value === 'start' ? run('go', '') : press(b));

// Ask the agent to revise: a comment in the plan's discussion, and a
// brief when it has a worktree
function revise(body: string) {
  emit('comment', body, 3);
  if (button.value === 'brief') void run('brief', body);
}
</script>
