<!-- The Code step (the canvas's C5): the branch's commits, its tests and
     the PR's checks, the PR's title and description, and Mark ready for
     review, which a person presses. -->
<template>
  <p v-if="strip" class="gb-plan-agent">{{ strip }}</p>
  <h3 class="gb-ws-title">The branch, ready for a pull request</h3>

  <div v-if="changes?.commits.length">
    <p class="gb-eyebrow">Commits on <span class="gb-mono gb-code-branch">{{ changes.branch }}</span></p>
    <ol class="gb-code-commits">
      <li v-for="c in changes.commits" :key="c.sha">
        <span class="gb-mono">{{ c.sha.slice(0, 7) }}</span><span>{{ c.subject }}</span><span class="gb-note">{{ tests.has(c.sha) ? 'step 4' : `${c.author}, ${when(c.date)}` }}</span>
      </li>
    </ol>
    <!-- ponytail: the branch's total, not each commit's; the endpoint has the total -->
    <p class="gb-note">+{{ sum('added') }} −{{ sum('deleted') }} in {{ changes.files.length }} file{{ changes.files.length === 1 ? '' : 's' }}</p>
  </div>
  <p v-else-if="!cycle.evidence.branch" class="gb-note">No branch yet: the assignee pushes one named for the issue.</p>
  <p v-if="error" class="gb-note">{{ error }}</p>

  <ul v-if="tiles.length" class="gb-tiles">
    <li v-for="t in tiles" :key="t.what">
      <span><span :class="['gb-pill', `is-${t.pill.toLowerCase()}`]">{{ t.pill }}</span> {{ t.what }}</span>
      <span class="gb-note">{{ t.line }}</span>
    </li>
  </ul>

  <form v-if="pr" class="gb-code-pr" @submit.prevent="emit('edit', pr.number, title.trim(), body)">
    <p class="gb-eyebrow">
      {{ pr.draft ? 'Draft PR' : 'PR' }} <a :href="pr.url" target="_blank" rel="noopener noreferrer">#{{ pr.number }}<span class="gb-sr">(opens in a new tab)</span></a>{{ pr.draft && cycle.stages.plans_in_pr && !cycle.skipped?.includes(3) ? ', opened at step 3 with the plan' : `, ${pr.state.toLowerCase()}` }}
    </p>
    <label class="gb-code-fld">Title<input v-model="title" class="epfl-input" :disabled="!current || busy" /></label>
    <label class="gb-code-fld">Description<textarea v-model="body" class="epfl-input" rows="4" :disabled="!current || busy" /></label>
    <p v-if="current" class="gb-cycle-actions">
      <button type="submit" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="busy || !title.trim() || (title === pr.title && body === (pr.body ?? ''))">Save on GitHub</button>
    </p>
  </form>
  <p v-else-if="cycle.evidence.branch" class="gb-cycle-actions">
    <span class="gb-note">No pull request yet.</span>
    <button v-if="current" type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="busy" @click="emit('open', cycle.evidence.branch)">Open a pull request</button>
  </p>

  <footer v-if="current" class="gb-step-foot">
    <div>
      <p class="gb-step-by">A person marks it ready</p>
      <p class="gb-note">{{ cycle.waits.anyone || !cycle.waits.login ? 'Anyone' : `${cycle.waits.login}, the assignee` }}. Ready for review moves the issue to {{ cycle.stages.in_review[0] ?? 'review' }} and opens step 6.</p>
    </div>
    <button v-if="can" type="button" class="epfl-btn epfl-btn-primary" :disabled="busy" @click="emit('ready')">Mark ready for review</button>
  </footer>
  <p v-else class="gb-gate">{{ gate }}</p>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { getChanges, type Changes, type Cycle, type IssueAgent } from '@/api/board';
import { message } from '@/issue';
import { when } from '@/when';
import { agentWords } from '@/worktrees';

// can: the viewer may mark it ready (the server's actions); current: step 5
// is the issue's step, else it is looked at from the step bar and only read
const props = defineProps<{ id: string; cycle: Cycle; gate: string; current: boolean; busy: boolean; can: boolean; agent?: IssueAgent | null }>();
const emit = defineEmits<{ ready: []; open: [branch: string]; edit: [number: number, title: string, body: string] }>();

const pr = computed(() => props.cycle.evidence.pr);
// ponytail: no "open the agent" link: a browser can't open a tmux pane; the Worktrees tab
const strip = computed(() => {
  const a = props.agent;
  return a?.worktree ? `Worktree ${a.worktree.path.replace(/^.*\//, '')} · ${agentWords(a.session, true)}` : '';
});

// the branch's commits (the Changes tab's endpoint), read again when its head moves
const changes = ref<Changes | null>(null);
const error = ref('');
watch(
  () => [props.id, props.cycle.evidence.branch, props.cycle.evidence.head],
  async () => {
    const branch = props.cycle.evidence.branch;
    if (!branch) return void (changes.value = null);
    try {
      changes.value = await getChanges(props.id, { branch });
      error.value = '';
    } catch (e) {
      error.value = `The commits: ${message(e)}`;
    }
  },
  { immediate: true },
);
const sum = (k: 'added' | 'deleted') => changes.value?.files.reduce((n, f) => n + f[k], 0) ?? 0;
const tests = computed(() => new Set((props.cycle.evidence.tests ?? []).map((t) => t.commit)));

// step 4's tests at their commit, and the PR's checks
const tiles = computed(() => {
  const e = props.cycle.evidence;
  const out = (e.tests ?? []).map((t) => ({
    pill: 'Test',
    what: t.path,
    line: t.commit ? `at ${t.commit.slice(0, 7)}${t.commit === e.head ? ', the head' : ''}` : 'not committed yet',
  }));
  const checks = pr.value?.checks ?? [];
  if (checks.length) {
    const green = checks.filter((c) => c.state === 'success').length;
    const failed = checks.some((c) => c.state === 'failure');
    out.push({ pill: failed ? 'Failed' : green === checks.length ? 'Pass' : 'Running', what: `${green} of ${checks.length} checks green`, line: `on PR #${pr.value?.number}` });
  }
  return out;
});

// the form starts from GitHub's words, and again when they change there
const title = ref('');
const body = ref('');
watch(() => [pr.value?.title, pr.value?.body], () => ((title.value = pr.value?.title ?? ''), (body.value = pr.value?.body ?? '')), { immediate: true });
</script>

<style scoped>
.gb-code-branch { text-transform: none; letter-spacing: 0; }
.gb-code-commits { margin: .5rem 0 0; padding: 0; list-style: none; font-size: .875rem; }
.gb-code-commits li { display: grid; grid-template-columns: 5.5rem minmax(0, 1fr) auto; gap: .75rem; padding: .625rem 0; border-top: 1px solid var(--border); align-items: baseline; }
.gb-code-pr { display: grid; gap: .75rem; padding: 1rem 1.25rem; border: 1px solid var(--fg); }
.gb-code-pr p { margin: 0; }
.gb-code-pr a { letter-spacing: 0; }
.gb-code-fld { display: grid; gap: .25rem; font-size: .8125rem; }
</style>
