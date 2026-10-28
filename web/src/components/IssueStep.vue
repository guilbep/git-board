<!-- The issue's frame under its title (docs/prd/time-flow.md): the step
     bar, then one column, the sidebar beside it. The column is the tabs,
     the shown step's workspace when head (the Conversation tab), then the
     flow. The step shown is the issue's (internal/cycle), or a done one
     picked on the bar; a later step is locked and says why. The panel
     fills the slots: tabs, intent (the description, folded off Intent),
     review (the Changes view, Review's on the page), flow, side. The bar
     is the project's list ([cycle] steps), a cell per entry. -->
<template>
  <p v-if="error && !(here && card)" class="gb-save-error" role="alert"><GbIcon name="error" small /> {{ error }}</p>
  <nav v-if="cycle" class="gb-stepbar" aria-label="The cycle, click a step to open it">
    <ol class="gb-bar-steps">
      <li v-for="r in rows" :key="r.id" :class="[`is-${r.state}`, { 'is-shown': r === row }]">
        <a v-if="r.state === 'done' || r.state === 'current'" class="gb-bar-step" :href="`#/issues/${id}?step=${r.id}`" :aria-current="r.state === 'current' ? 'step' : undefined" @click="jump($event, r.id)">
          <span class="gb-bar-n">{{ r.i }}</span>
          <span class="gb-bar-name">{{ r.name }}</span>
          <span class="gb-bar-line">{{ r.line }}</span>
        </a>
        <span v-else class="gb-bar-step" aria-disabled="true">
          <span class="gb-bar-n"><GbIcon v-if="r.state === 'locked'" name="lock" small /><template v-else>{{ r.i }}</template></span>
          <span class="gb-bar-name">{{ r.name }}</span>
          <span class="gb-bar-line">{{ r.line }}</span>
        </span>
      </li>
    </ol>
  </nav>

  <div class="gb-row">
    <section class="gb-column" aria-label="Conversation">
      <slot name="tabs" />
      <div class="gb-column-body">
        <template v-if="head">
          <p v-if="row || !cycle" class="gb-eyebrow" tabindex="-1">Step {{ row?.i ?? 1 }} · {{ row?.name ?? 'Intent' }}<template v-if="risk"> · {{ risk }}</template></p>
          <slot v-if="n === 1" name="intent" :folded="false" :cycle="cycle" />

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
          <CycleRisk
            v-else-if="cycle && n === 2 && (current || risk)"
            :cycle="cycle"
            :gate="row?.gate ?? ''"
            :current="current"
            :busy="busy"
            :can="has('set-risk')"
            :events="events"
            :risk-by="riskBy"
            @set="(r) => patch({ risk: r })"
            @suggest="act('suggest-risk')"
          />
          <CycleCode
            v-else-if="cycle && n === 5 && (current || cycle.evidence.branch)"
            :id="id"
            :cycle="cycle"
            :gate="row?.gate ?? ''"
            :current="current"
            :busy="busy"
            :can="has('ready-for-review')"
            :agent="agent"
            @ready="card && pressed(card.buttons[0]!)"
            @open="(branch) => pull(() => openPull(id, branch))"
            @edit="(num, title, body) => pull(() => editPull(id, num, title, body))"
          />
          <CycleVerify
            v-else-if="cycle && n === 7 && (current || cycle.done)"
            :id="id"
            :cycle="cycle"
            :status="status"
            :events="events"
            :current="current"
            :forge="forge"
            :busy="busy"
            :can="has('signoff')"
            :needs-why="needsWhy"
            @check="(i) => act('check', { index: i })"
            @signoff="(why) => act('signoff', why ? { why } : {})"
            @run="(output, commit) => act('verify', { output, commit })"
            @back="patch({ status: cycle.stages.in_progress[0] ?? '' })"
          />
          <!-- step 6 on the page is the review's own (CycleReview) -->
          <template v-else-if="cycle && (row || here) && !(n === 6 && $slots.review)">
            <CycleIntent v-if="n === 1" :cycle="cycle" :current="current" :busy="busy" :can-accept="has('no-criteria')" @save="saveCriteria" @accept="act('no-criteria')" />
            <CycleTests v-if="n === 4 && !cycle.skipped?.includes(4)" :id="id" :cycle="cycle" />
            <p v-else-if="n !== 1 && row?.state === 'done' && row.line !== row.gate" class="gb-note">{{ row.line }}</p>
            <p v-if="row" class="gb-gate">{{ row.gate }}</p>
            <template v-if="here">
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
              <!-- a refusal of the card's move: Mark ready for review names the red check -->
              <p v-if="error && card" class="gb-save-error" role="alert"><GbIcon name="error" small /> {{ error }}</p>
            </template>
          </template>
          <slot
            v-if="cycle && n === 6"
            name="review"
            :cycle="cycle"
            :current="current"
            :can="has('review') || has('to-validation')"
            :sends="button === 'brief'"
            :verify="() => patch({ status: cycle!.stages.validation[0] ?? '' })"
          />
          <slot v-if="n !== 1" name="intent" :folded="true" :cycle="cycle" />

          <p v-if="cycle && now && row !== now" class="gb-cycle-actions">
            <button type="button" class="gb-linkbtn" @click="shown = null">Back to step {{ now.i }} · {{ cycle.name }}</button>
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
        </template>
        <slot name="flow" :cycle="cycle" :failed="!cycle && !!error" :send="send" />
      </div>
    </section>
    <aside class="gb-sidebar" aria-label="Fields">
      <slot name="side" :cycle="cycle" />
    </aside>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { agentAction, editPull, getIssueAgent, openPull, type Forge, type IssueAgent, type IssueEvent } from '@/api/board';
import { cardFor, steps, statusBar, turn, withCriteria, type CardButton } from '@/cycle';
import { message } from '@/issue';
import { agentButton, agentWords } from '@/worktrees';
import { useCycle } from '@/usecycle';
import CycleCode from '@/components/CycleCode.vue';
import CycleIntent from '@/components/CycleIntent.vue';
import CyclePlan from '@/components/CyclePlan.vue';
import CycleRisk from '@/components/CycleRisk.vue';
import CycleTests from '@/components/CycleTests.vue';
import CycleVerify from '@/components/CycleVerify.vue';
import GbIcon from '@/components/GbIcon.vue';

const props = defineProps<{ id: string; status: string; labels: string[]; events: IssueEvent[]; forges?: Forge[] | undefined; head: boolean }>();
const emit = defineEmits<{ changed: []; comment: [body: string, step: string]; open: [id: string] }>();

const description = computed(() => props.events.find((e) => e.kind === 'created'));
const { cycle, error, busy, needsWhy, has, act, patch, edit, press, pull } = useCycle(
  { get id() { return props.id; }, get labels() { return props.labels; }, get description() { return description.value?.body ?? ''; } },
  () => emit('changed'),
);
const card = computed(() => (cycle.value ? cardFor(cycle.value, props.status) : null));
const cap = (s: string) => s.charAt(0).toUpperCase() + s.slice(1);
const forge = computed(() => props.forges?.filter((f) => f.number).map((f) => `${f.name} #${f.number}`)[0]);

// who set the risk: the last label change adding one, from the thread
const riskBy = computed(() => props.events.findLast((e) => e.kind === 'labels' && e.added?.some((l) => l.startsWith('risk:')))?.author.name);
const rows = computed(() => (cycle.value ? steps(cycle.value, riskBy.value, props.events) : []));
const risk = computed(() => cycle.value?.risk ?? cycle.value?.evidence.risk);

// the step shown, by id, picked on the bar; null is the issue's step, and
// the frame goes back to it when the step moves or the issue changes; the
// cycle's first read isn't a move, so a ?step= picked before it stays
const shown = ref<string | null>(null);
watch([() => props.id, () => cycle.value?.at], ([id, at], [was, wasAt]) => {
  if (id !== was || (wasAt !== undefined && at !== wasAt)) shown.value = null;
});
const now = computed(() => rows.value.find((r) => r.id === cycle.value?.at));
const row = computed(() => (shown.value === null ? now.value : rows.value.find((r) => r.id === shown.value)));
// the shown cell's kind picks the workspace; none before the first step
// opens, Intent without a cycle
const n = computed(() => (cycle.value ? (row.value?.n ?? 0) : 1));
const current = computed(() => row.value?.state === 'current');
// where the step's note and card go: the current cell, or the column
// before the list's first step opens (a list that starts at code, at ready)
const here = computed(() => current.value || (!!cycle.value && !now.value && !row.value));
const show = (id: string) => (shown.value = id === cycle.value?.at ? null : id);
// a refusal belongs to the step pressed: another step shown drops it
watch(row, () => (error.value = ''));
defineExpose({ show });
// the rail opens a step at the head of the column; a new tab or window is the link's
function jump(e: MouseEvent, step: string) {
  if (e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey) return;
  e.preventDefault();
  emit('open', step);
}

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
// the composer's Send to: the agent, once it has a worktree
const send = computed(() =>
  button.value === 'brief' ? { name: agent.value?.session?.agent || 'the agent', brief: (body: string) => void run('brief', body) } : undefined,
);
watch(cycle, () => void getIssueAgent(props.id).then((a) => (agent.value = a), () => (agent.value = null)));

async function run(action: 'go' | 'brief' | 'approve', brief: string) {
  agentBusy.value = action;
  agentNote.value = '';
  try {
    agent.value = await agentAction(props.id, action, brief);
    agentNote.value = { go: `Session ${agent.value.session?.session ?? agent.value.branch} started.`, brief: 'Sent to the agent.', approve: 'Sent to the agent: the plan is approved, it goes on with the next step.' }[action];
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

// Approve the plan on the board, then brief the agent on to the next step.
async function approve() {
  const plan = !!cycle.value?.evidence.plan; // no file: nothing to accept, the server refuses
  if ((await act('approve-plan')) && plan && button.value === 'brief') void run('approve', '');
}

// Start work with wtx and no worktree yet: wtx go, which moves the status
// once the agent is started; without wtx, the status alone
const pressed = (b: CardButton) => (b.action === 'start' && button.value === 'start' ? run('go', '') : press(b));

// Ask the agent to revise: a comment in the plan's discussion, and a
// brief when it has a worktree
function revise(body: string) {
  emit('comment', body, 'plan');
  if (button.value === 'brief') void run('brief', body);
}
</script>
