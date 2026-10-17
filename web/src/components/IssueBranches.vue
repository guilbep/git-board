<!-- The side column's Development: the branches whose name names the issue
     (internal/branches), against the base, and Create branch: the name
     the issue suggests, as commands to copy. git-board creates no branch.
     When origin is on GitHub, each branch there carries its gate (the PR
     whose head it is, pulls.go): its checks, required first, its
     approvals, and Open PR or Merge, done on GitHub as you. -->
<template>
  <section class="gb-side-sec" aria-labelledby="side-dev">
    <h3 id="side-dev" class="gb-section-title">Development</h3>
    <p v-if="error" class="gb-hint" role="alert"><b>Couldn’t read the branches.</b> {{ error }}</p>
    <template v-else-if="dev">
      <ul v-if="dev.branches.length" class="gb-dev-list">
        <li v-for="b in dev.branches" :key="`${b.remote}:${b.name}`" class="gb-dev-branch">
          <span class="gb-mono gb-dev-name">{{ b.remote ? `${b.remote}/` : '' }}{{ b.name }}</span>
          <span class="gb-dev-meta">
            <b v-if="b.merged && b.behind" class="gb-state-ok">merged into {{ dev.base }}</b>
            <b v-else-if="b.in_base && !b.merged" class="gb-state-ok" :title="`Its changes are in ${dev.base}: squashed or rebased in`">in {{ dev.base }}, squashed</b>
            <template v-else-if="b.merged">same as {{ dev.base }}</template>
            <template v-else>not in {{ dev.base }} · {{ b.ahead }} ahead, {{ b.behind }} behind</template>
          </span>
          <span class="gb-dev-meta">{{ b.last_commit.subject }} · <time :datetime="b.last_commit.date">{{ when(b.last_commit.date) }}</time></span>

          <div v-if="b.remote === 'origin' && gateOf(b.name)" class="gb-dev-gate">
            <template v-for="pr of [gateOf(b.name)!.pr]" :key="b.name">
              <a v-if="pr" :href="pr.url" target="_blank" rel="noopener noreferrer" class="gb-dev-pr">
                #{{ pr.number }} {{ prState(pr) }}<template v-if="pr.author"> · by {{ pr.author }}</template><span class="gb-sr">(opens in a new tab)</span>
              </a>
              <template v-if="pr?.state === 'OPEN'">
                <ul class="gb-dev-checks" aria-label="Checks">
                  <li v-for="c in pr.checks" :key="c.name" class="gb-dev-check">
                    <span :class="mark(c.state).cls" aria-hidden="true">{{ mark(c.state).sign }}</span>
                    <span class="gb-sr">{{ mark(c.state).word }}:</span>
                    <a v-if="c.url" :href="c.url" target="_blank" rel="noopener noreferrer">{{ c.name }}</a>
                    <template v-else>{{ c.name }}</template>
                    <span v-if="c.required" class="gb-dev-req">required</span>
                  </li>
                </ul>
                <span v-for="r in approvals(pr)" :key="r.login" class="gb-dev-meta">
                  <span class="gb-state-ok" aria-hidden="true">✓</span> {{ r.login }} approved at <span class="gb-mono">{{ r.commit.slice(0, 7) }}</span>
                  {{ r.commit === pr.head ? '(the head)' : '(an older commit)' }}
                </span>
                <span v-if="decision(pr)" class="gb-dev-meta">{{ decision(pr) }}</span>
                <button type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm gb-dev-act" :disabled="!pr.can_merge || busy !== ''" @click="merge(pr)">
                  Merge into {{ pr.base }}
                </button>
                <span v-if="pr.checking" class="gb-dev-meta" role="status">Checking…</span>
                <span v-else-if="pr.waits.length" class="gb-dev-meta">Waits for: {{ pr.waits.join(', ') }}</span>
              </template>
              <template v-if="!pr || pr.state === 'CLOSED'">
                <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm gb-dev-act" :disabled="busy !== ''" @click="open(b.name)">
                  Open PR into {{ pulls!.base }}
                </button>
                <span class="gb-dev-meta">As {{ pulls!.login }}, titled from the issue.</span>
              </template>
            </template>
          </div>
          <div v-else-if="!b.remote && pulls?.repo && !onOrigin(b.name)" class="gb-dev-gate">
            <span class="gb-dev-meta">Local only. Push it to open a PR:</span>
            <pre class="gb-dev-cmd"><code>git push -u origin {{ b.name }}</code></pre>
            <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" @click="copyText(`git push -u origin ${b.name}`, b.name)">
              <GbIcon v-if="copied === b.name" name="check" small /> {{ copied === b.name ? 'Copied' : 'Copy' }}
            </button>
          </div>
        </li>
      </ul>
      <p v-else class="gb-hint">No branch yet.</p>
      <p v-if="gateError" class="gb-hint" role="alert"><b>Couldn’t reach GitHub.</b> {{ gateError }}</p>
      <details class="gb-dev-new">
        <summary class="epfl-btn epfl-btn-secondary epfl-btn-sm"><GbIcon name="plus" small /> Create branch</summary>
        <div class="gb-dev-panel">
          <label class="epfl-label" :for="`dev-name-${id}`">Branch name</label>
          <input :id="`dev-name-${id}`" v-model="name" class="epfl-input gb-mono" autocomplete="off" data-1p-ignore data-lpignore="true" data-form-type="other" spellcheck="false" />
          <p class="gb-dev-meta">From <span class="gb-mono">{{ dev.start }}</span>. Run in the repository:</p>
          <pre class="gb-dev-cmd"><code>{{ commands }}</code></pre>
          <button type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="!name.trim()" @click="copyText(commands, 'create')">
            <GbIcon v-if="copied === 'create'" name="check" small /> {{ copied === 'create' ? 'Copied' : 'Copy' }}
          </button>
        </div>
      </details>
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue';
import { getBranches, getPulls, mergePull, openPull, type GateCheck, type GatePR, type IssueBranches, type IssuePulls } from '@/api/board';
import { message } from '@/issue';
import { actThenRead } from '@/gate';
import { when } from '@/me';
import GbIcon from '@/components/GbIcon.vue';

const props = defineProps<{ id: string; labels: string[]; title: string }>();

// the kind and the title name the branch: reread when they change
const dev = ref<IssueBranches | null>(null);
const error = ref('');
const name = ref('');
async function load() {
  try {
    dev.value = await getBranches(props.id);
    name.value = dev.value.suggested;
    error.value = '';
  } catch (e) {
    error.value = message(e);
  }
}
watch(() => [props.id, props.labels.join(' '), props.title], load, { immediate: true });

// The gate asks GitHub: read for the issue, after Open or Merge, and
// every 15 s while GitHub is still checking, and only when the page shows.
const pulls = ref<IssuePulls | null>(null);
const gateError = ref('');
const busy = ref(''); // the branch being opened, or the PR being merged
let timer: ReturnType<typeof setTimeout> | undefined;
async function loadPulls() {
  clearTimeout(timer);
  try {
    pulls.value = await getPulls(props.id);
    gateError.value = '';
  } catch (e) {
    gateError.value = message(e);
  }
  if (pulls.value?.pulls.some((p) => p.pr && waiting(p.pr))) later();
}
function later() {
  timer = setTimeout(() => (document.hidden ? later() : loadPulls()), 15000);
}
watch(() => props.id, () => { pulls.value = null; loadPulls(); }, { immediate: true });
onUnmounted(() => clearTimeout(timer));

function waiting(pr: GatePR): boolean {
  return pr.state === 'OPEN' && (pr.checking || pr.checks.some((c) => c.state === 'pending' || c.state === 'expected'));
}

const gateOf = (branch: string) => pulls.value?.pulls.find((p) => p.branch === branch);
const onOrigin = (branch: string) => dev.value?.branches.some((b) => b.remote === 'origin' && b.name === branch);

function prState(pr: GatePR): string {
  if (pr.state === 'OPEN') return pr.draft ? 'Draft' : 'Open';
  return pr.state === 'MERGED' ? 'Merged' : 'Closed';
}

const marks: Record<GateCheck['state'], { sign: string; cls: string; word: string }> = {
  success: { sign: '✓', cls: 'gb-state-ok', word: 'passed' },
  failure: { sign: '✗', cls: 'gb-state-bad', word: 'failed' },
  pending: { sign: '●', cls: 'gb-state-wait', word: 'running' },
  expected: { sign: '●', cls: 'gb-state-wait', word: 'expected' },
};
const mark = (state: GateCheck['state']) => marks[state];

const approvals = (pr: GatePR) => pr.reviews.filter((r) => r.state === 'APPROVED');

function decision(pr: GatePR): string {
  switch (pr.decision) {
    case 'APPROVED': return 'Review rule met.';
    case 'REVIEW_REQUIRED': return 'Review required.';
    case 'CHANGES_REQUESTED': return 'Changes requested.';
  }
  return '';
}

async function act(what: string, run: () => Promise<unknown>) {
  busy.value = what;
  try {
    gateError.value = await actThenRead(run, async () => (await loadPulls(), gateError.value));
  } finally {
    busy.value = '';
  }
}

const open = (branch: string) => act(branch, () => openPull(props.id, branch));

function merge(pr: GatePR) {
  if (!window.confirm(`Merge #${pr.number} into ${pr.base} on GitHub?`)) return;
  act(`#${pr.number}`, () => mergePull(props.id, pr.number, pr.head));
}

const commands = computed(() => `git switch -c ${name.value.trim()} ${dev.value?.start ?? ''}\ngit push -u origin ${name.value.trim()}`);

const copied = ref(''); // which Copy button just copied
async function copyText(text: string, which: string) {
  try {
    await navigator.clipboard.writeText(text);
    copied.value = which;
    setTimeout(() => (copied.value = ''), 2000);
  } catch {
    window.prompt('Copy the commands', text.replace('\n', ' && '));
  }
}
</script>

<style scoped>
.gb-dev-list { display: flex; flex-direction: column; gap: .75rem; margin: 0; padding: 0; list-style: none; }
.gb-dev-branch { display: flex; flex-direction: column; gap: .125rem; font-size: .875rem; }
.gb-dev-name { font-weight: 700; overflow-wrap: anywhere; }
.gb-dev-meta { font-size: .8125rem; color: var(--fg-muted); }
.gb-dev-gate { display: flex; flex-direction: column; align-items: flex-start; gap: .25rem; margin-top: .375rem; }
.gb-dev-pr { font-weight: 700; }
.gb-dev-checks { display: flex; flex-direction: column; gap: .125rem; margin: 0; padding: 0; list-style: none; font-size: .8125rem; }
.gb-dev-check { display: flex; align-items: baseline; gap: .375rem; }
.gb-dev-req { padding: 0 .25rem; border: 1px solid var(--border-strong); font-size: .6875rem; color: var(--fg-muted); }
.gb-dev-act { margin-top: .25rem; }
.gb-state-bad { color: var(--danger-fg); }
.gb-state-wait { color: var(--warning-fg); }
.gb-dev-new { margin-top: .75rem; }
.gb-dev-new summary { display: inline-flex; align-items: center; gap: .375rem; list-style: none; cursor: pointer; }
.gb-dev-new summary::-webkit-details-marker { display: none; }
.gb-dev-panel { display: flex; flex-direction: column; align-items: flex-start; gap: .5rem; margin-top: .5rem; padding: .75rem; border: var(--border-w) solid var(--border-strong); background: var(--bg); }
.gb-dev-panel .epfl-input { width: 100%; min-width: 0; box-sizing: border-box; }
.gb-dev-panel .gb-dev-meta { margin: 0; }
.gb-dev-cmd { width: 100%; box-sizing: border-box; margin: 0; padding: .5rem; background: var(--bg-subtle); font-family: var(--font-mono); font-size: .8125rem; white-space: pre-wrap; overflow-wrap: anywhere; }
.gb-dev-cmd code { color: var(--fg); }
</style>
