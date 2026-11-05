<!-- The cycle on the issue page (experimental; api.md routes 49 and 50):
     the steps, the step's gate and whose turn it is, the branches, the
     pull request and the environments, and the controls the server
     offers the viewer now. v1's IssueStep and its Cycle* panels (Intent,
     Risk, Plan, Code, Review, Verify), lean: one box, each control one
     action of route 50, sent from the version read (If-Match). -->
<template>
  <section class="gb-cyc" aria-label="The cycle">
    <ol class="gb-cyc-cells" aria-label="The steps">
      <li v-for="c in steps" :key="c.id" :class="`is-${c.state}`" :aria-current="c.state === 'current' ? 'step' : undefined">
        <span class="gb-cyc-n">{{ c.n }}</span> {{ c.name }}
      </li>
    </ol>

    <p class="gb-cyc-head">
      <b>Step {{ cycle.step }} · {{ cycle.name }}</b><template v-if="risk"> · {{ risk }} risk</template>
      <span v-if="cycle.gate" class="gb-cyc-gate"> — {{ cycle.gate }}</span>
    </p>
    <p v-if="cycle.note" class="gb-note">{{ cycle.note }}</p>
    <p v-if="!cycle.done" class="gb-note" data-testid="cycle-waits">{{ waits }}</p>

    <ul v-if="lanes.length > 1" class="gb-cyc-lanes" aria-label="The issue's branches">
      <li v-for="l in lanes" :key="l.branch"><code>{{ l.branch }}</code> at {{ stepName(l.step) }}<template v-if="l.done">, merged</template><template v-else-if="l.closed">, closed</template></li>
    </ul>

    <!-- the pull request (the forge's gate), or the button opening one -->
    <div v-if="pr" class="gb-cyc-pr" role="group" aria-label="Pull request">
      <a :href="safeHref(pr.url)" target="_blank" rel="noopener noreferrer">{{ prLine(pr) }}</a> {{ pr.title }}
      <ul v-if="pr.checks.length" class="gb-cyc-checks">
        <li v-for="ch in pr.checks" :key="ch.name" :class="`is-${ch.state}`">{{ ch.name }}: {{ ch.state }}<template v-if="ch.required"> (required)</template></li>
      </ul>
      <p v-if="pr.waits.length" class="gb-note">Waits for {{ pr.waits.join(', ') }}.</p>
      <div class="gb-cyc-actions">
        <button v-if="pr.state === 'OPEN'" type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="busy || !pr.can_merge" @click="act('merge', { number: pr.number, sha: pr.head })">Merge #{{ pr.number }}</button>
        <button v-if="pr.state === 'OPEN'" type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :aria-expanded="editing" :disabled="busy" @click="startEdit">Edit</button>
        <button type="button" class="gb-linkbtn" :disabled="busy" @click="act('refresh')">Check again</button>
        <span v-if="cycle.pr_checked_at" class="gb-hint">read {{ new Date(cycle.pr_checked_at).toLocaleTimeString() }}</span>
      </div>
      <form v-if="editing" class="gb-cyc-form" @submit.prevent="act('edit-pull', { number: pr.number, title: prTitle, body: prBody }).then((ok) => ok && (editing = false))">
        <input v-model="prTitle" class="epfl-input" aria-label="The pull request's title" />
        <textarea v-model="prBody" class="epfl-input" rows="3" aria-label="The pull request's description" />
        <button type="submit" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="busy || !prTitle.trim()">Save</button>
      </form>
    </div>
    <div v-else-if="ev.branch && cycle.step >= 4" class="gb-cyc-actions">
      <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="busy" @click="act('open-pull', { branch: ev.branch })">Open a pull request from {{ ev.branch }}</button>
    </div>

    <p v-if="envs.length" class="gb-cyc-envs" aria-label="Environments">
      <span v-for="e in envs" :key="e.name" class="epfl-badge" :class="e.in ? 'epfl-badge-success' : 'epfl-badge-tertiary'">{{ e.name }}{{ e.version ? ` ${e.version}` : '' }}{{ e.in ? ': has it' : '' }}</span>
    </p>

    <div v-if="cycle.evidence.plan?.html && cycle.step === 3" class="gb-cyc-plan" v-html="cycle.evidence.plan.html" />

    <template v-for="c in shown" :key="c.kind === 'press' ? c.action : c.kind">
      <div v-if="c.kind === 'press'" class="gb-cyc-actions">
        <button type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="busy" @click="act(c.action)">{{ c.label }}</button>
      </div>
      <div v-else-if="c.kind === 'risk'" class="gb-cyc-actions" role="group" aria-label="The risk class">
        <button v-for="k in ['low', 'normal', 'structural']" :key="k" type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="busy" @click="act('set-risk', { risk: k })">{{ cap(k) }}</button>
      </div>
      <form v-else-if="c.kind === 'verdict'" class="gb-cyc-form" @submit.prevent>
        <textarea v-model="text" class="epfl-input" rows="2" aria-label="Your comment on the description" placeholder="Which changes? (optional to approve)" />
        <div class="gb-cyc-actions">
          <button type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="busy" @click="verdict('approve')">Approve v{{ cycle.body_version ?? 1 }}</button>
          <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="busy || !text.trim()" @click="verdict('request-changes')">Request changes</button>
        </div>
      </form>
      <form v-else-if="c.kind === 'review'" class="gb-cyc-form" aria-label="Review" @submit.prevent="review">
        <textarea v-model="text" class="epfl-input" rows="2" aria-label="Your review" placeholder="Explain the change in one line to approve it, or say what to change" />
        <div class="gb-cyc-actions">
          <select v-model="reviewState" class="epfl-input gb-cyc-select" aria-label="Your verdict">
            <option value="APPROVE">Approve</option>
            <option value="REQUEST_CHANGES">Request changes</option>
            <option value="COMMENT">Comment</option>
          </select>
          <button type="submit" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="busy || !text.trim()">Submit the review</button>
        </div>
      </form>
      <div v-else-if="c.kind === 'verify'" class="gb-cyc-verify">
        <ul v-if="checks.length" class="gb-cyc-criteria" aria-label="Acceptance criteria">
          <li v-for="k in checks" :key="k.index">
            <template v-if="k.by"><GbIcon name="check" small /> {{ k.text }} <span class="gb-hint">checked by {{ k.by }}</span></template>
            <template v-else>{{ k.text }} <button type="button" class="gb-linkbtn" :disabled="busy" :aria-label="`Check: ${k.text}`" @click="act('check', { index: k.index })">Check</button></template>
          </li>
        </ul>
        <form class="gb-cyc-form" @submit.prevent="run">
          <textarea v-model="output" class="epfl-input gb-mono" rows="3" aria-label="A run's output" :placeholder="`Paste a run's output${ev.head ? ` at ${short(ev.head)}` : ''}`" />
          <button type="submit" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="busy || !output.trim()">Record the run</button>
        </form>
        <form class="gb-cyc-form" @submit.prevent="act('signoff', why.trim() ? { why: why.trim() } : {})">
          <input v-if="!ranAtHead(cycle)" v-model="why" class="epfl-input" aria-label="Why sign off without a run" placeholder="No run at the head: say why to sign off all the same" />
          <button type="submit" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="busy || (!ranAtHead(cycle) && !why.trim())">Sign off {{ ev.status }}</button>
        </form>
      </div>
    </template>

    <p v-if="error" class="gb-save-error" role="alert">
      <GbIcon name="error" small /> {{ error }}
      <button v-if="stale" type="button" class="gb-stale-btn" @click="emit('reload')">Reload</button>
    </p>
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { cycleAct, isStale, message } from '@/api';
import type { Cycle, CycleActBody } from '@/api/cycletypes';
import { cells, controls, criteria, prLine, ranAtHead, short } from '@/cycleact';
import { STEPS } from '@/cycle';
import { safeHref } from '@/links';
import GbIcon from '@/components/GbIcon.vue';

const props = defineProps<{ id: string; cycle: Cycle }>();
const emit = defineEmits<{ changed: [cycle: Cycle]; reload: [] }>();

const ev = computed(() => props.cycle.evidence);
const steps = computed(() => cells(props.cycle));
const shown = computed(() => controls(props.cycle));
const checks = computed(() => criteria(props.cycle));
const risk = computed(() => props.cycle.risk ?? ev.value.risk);
const lanes = computed(() => ev.value.lanes ?? []);
const pr = computed(() => ev.value.pr ?? null);
const envs = computed(() => props.cycle.environments ?? []);
const cap = (s: string) => s.charAt(0).toUpperCase() + s.slice(1);
const stepName = (n: number) => STEPS[n] ?? '';
const waits = computed(() => {
  const w = props.cycle.waits;
  const who = w.anyone ? 'Anyone' : props.cycle.mine ? 'You' : w.login;
  return `${who} to ${w.what}.`;
});

const busy = ref(false);
const error = ref('');
const stale = ref(false);
const text = ref('');
const output = ref('');
const why = ref('');
const reviewState = ref<'APPROVE' | 'REQUEST_CHANGES' | 'COMMENT'>('APPROVE');
const editing = ref(false);
const prTitle = ref('');
const prBody = ref('');

// act sends one action from the version read; the cycle after it comes
// back, and the issue reloads (its comments, its status)
async function act(action: string, body: CycleActBody = {}): Promise<boolean> {
  busy.value = true;
  error.value = '';
  stale.value = false;
  try {
    const c = await cycleAct(props.id, action, props.cycle.version, body);
    text.value = output.value = why.value = '';
    emit('changed', c);
    return true;
  } catch (e) {
    stale.value = isStale(e);
    error.value = stale.value ? 'This issue changed meanwhile: nothing was written.' : message(e);
    return false;
  } finally {
    busy.value = false;
  }
}

const verdict = (v: 'approve' | 'request-changes') => act('spec-verdict', { verdict: v, on: `body@v${props.cycle.body_version ?? 1}`, body: text.value.trim() });
const review = () => act('review', { state: reviewState.value, body: text.value.trim() });
const run = () => act('verify', { output: output.value });
function startEdit() {
  editing.value = !editing.value;
  prTitle.value = pr.value?.title ?? '';
  prBody.value = pr.value?.body ?? '';
}
</script>

<style scoped>
.gb-cyc { display: grid; gap: var(--space-2); margin: 0 0 var(--space-4); padding: var(--space-3); border: var(--border-w) solid var(--border-subtle); border-radius: var(--radius); font-size: var(--fs-sm); }
.gb-cyc p { margin: 0; }
.gb-cyc-cells { display: flex; flex-wrap: wrap; gap: var(--space-1); margin: 0; padding: 0; list-style: none; }
.gb-cyc-cells li { padding: 0 var(--space-2); border: var(--border-w) solid var(--border-subtle); border-radius: var(--radius); color: var(--fg-subtle); }
.gb-cyc-cells .is-done { color: var(--fg-muted); background: var(--bg-subtle); }
.gb-cyc-cells .is-current { color: var(--fg); font-weight: var(--w-bold); border-color: var(--fg); }
.gb-cyc-cells .is-skipped { text-decoration: line-through; }
.gb-cyc-n { font-variant-numeric: tabular-nums; }
.gb-cyc-gate { color: var(--fg-muted); }
.gb-cyc-lanes, .gb-cyc-checks, .gb-cyc-criteria { margin: 0; padding-left: 1.25rem; }
.gb-cyc-checks .is-failure { color: var(--danger, #b51f1f); }
.gb-cyc-actions { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-2); }
.gb-cyc-form { display: grid; gap: var(--space-2); justify-items: start; }
.gb-cyc-form textarea, .gb-cyc-form input { width: 100%; }
.gb-cyc-select { width: auto; }
.gb-cyc-envs { display: flex; flex-wrap: wrap; gap: var(--space-1); }
.gb-cyc-verify { display: grid; gap: var(--space-2); }
</style>
