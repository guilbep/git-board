<!-- One branch, opened from its chip (README 16c, decision 5): where it
     stands against the base, its last commit; the gate, GitHub's PR whose
     head it is (internal/ghpr): its checks, required ones flagged, its
     approvals and decision, Merge into the base once GitHub says it can.
     Without a gate on GitHub: Ask for review, which opens one. Local only:
     the push command. -->
<template>
  <div class="gb-bdetail">
    <p class="gb-bdetail-line">
      <template v-if="b.merged && b.behind">Merged into {{ dev.base }}.</template>
      <template v-else-if="b.in_base && !b.merged">In {{ dev.base }}, squashed or rebased.</template>
      <template v-else-if="b.merged">Same as {{ dev.base }}: no commit yet.</template>
      <template v-else>{{ b.ahead }} ahead of {{ dev.base }}, {{ b.behind }} behind.</template>
      Last: {{ b.last_commit.subject }}, <time :datetime="b.last_commit.date">{{ when(b.last_commit.date) }}</time>.
    </p>
    <template v-if="pr">
      <p class="gb-bdetail-line">
        The gate: <a :href="pr.url" target="_blank" rel="noopener noreferrer">GitHub #{{ pr.number }}<span class="gb-sr">(opens in a new tab)</span></a>,
        {{ prState }}<template v-if="pr.author">, by {{ pr.author }}</template>.
      </p>
      <template v-if="pr.state === 'OPEN'">
        <ul v-if="pr.checks.length" class="gb-bdetail-checks" aria-label="Checks">
          <li v-for="c in pr.checks" :key="c.name" :class="`is-${c.state}`">
            <GbIcon :name="mark(c.state).icon" small />
            <span class="gb-sr">{{ mark(c.state).word }}:</span>
            <a v-if="c.url" :href="c.url" target="_blank" rel="noopener noreferrer">{{ c.name }}</a>
            <template v-else>{{ c.name }}</template>
            <span v-if="c.required" class="gb-bdetail-req">required</span>
          </li>
        </ul>
        <p v-for="r in approvals" :key="r.login" class="gb-bdetail-line">
          <GbIcon name="check" small /> {{ r.login }} approved <span class="gb-mono">{{ r.commit.slice(0, 7) }}</span>{{ r.commit === pr.head ? ', the head' : ', an older commit' }}.
        </p>
        <p v-if="decision" class="gb-bdetail-line">{{ decision }}</p>
        <p v-if="pr.checking" class="gb-bdetail-line" role="status">GitHub is checking…</p>
        <p v-else-if="pr.waits.length" class="gb-bdetail-line">Waits for {{ pr.waits.join(', ') }}.</p>
        <button type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="!pr.can_merge || busy !== ''" @click="emit('merge', pr)">Merge into {{ pr.base }}</button>
      </template>
    </template>
    <template v-else-if="gate">
      <p class="gb-bdetail-line">Not in review yet. Asking opens its gate on GitHub, as {{ pulls?.login }}, titled from the issue.</p>
      <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="busy !== ''" @click="emit('ask', item.name)">Ask for review</button>
    </template>
    <template v-else-if="local">
      <p class="gb-bdetail-line">Local only. Push it to ask for a review:</p>
      <pre class="gb-pop-cmd"><code>git push -u origin {{ item.name }}</code></pre>
      <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" @click="copy">
        <GbIcon :name="copied ? 'check' : 'copy'" small /> {{ copied ? 'Copied' : 'Copy' }}
      </button>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import type { GateCheck, GatePR, IssueBranches, IssuePulls } from '@/api/board';
import type { BranchItem } from '@/branchstate';
import { needsPush } from '@/gate';
import { when } from '@/me';
import GbIcon from '@/components/GbIcon.vue';

const props = defineProps<{ item: BranchItem; dev: IssueBranches; pulls: IssuePulls | null; busy: string }>();
const emit = defineEmits<{ ask: [branch: string]; merge: [pr: GatePR] }>();

const b = computed(() => props.item.branch);
const pr = computed(() => props.item.pr);
const gate = computed(() => props.pulls?.pulls.find((p) => p.branch === props.item.name));
const local = computed(() => props.dev.branches.filter((x) => x.name === props.item.name).some((x) => needsPush(x, props.dev.branches, props.pulls?.repo ?? '')));

const prState = computed(() => {
  const p = pr.value;
  if (!p) return '';
  if (p.state === 'OPEN') return p.draft ? 'a draft' : 'open';
  return p.state === 'MERGED' ? 'merged' : 'closed';
});

const marks: Record<GateCheck['state'], { icon: 'check' | 'close' | 'clock'; word: string }> = {
  success: { icon: 'check', word: 'passed' },
  failure: { icon: 'close', word: 'failed' },
  pending: { icon: 'clock', word: 'running' },
  expected: { icon: 'clock', word: 'expected' },
};
const mark = (state: GateCheck['state']) => marks[state];
const approvals = computed(() => pr.value?.reviews.filter((r) => r.state === 'APPROVED') ?? []);
const decision = computed(() => {
  switch (pr.value?.decision) {
    case 'APPROVED': return 'The review rule is met.';
    case 'REVIEW_REQUIRED': return 'A review is required.';
    case 'CHANGES_REQUESTED': return 'Changes were asked.';
  }
  return '';
});

const copied = ref(false);
async function copy() {
  const text = `git push -u origin ${props.item.name}`;
  try {
    await navigator.clipboard.writeText(text);
    copied.value = true;
    setTimeout(() => (copied.value = false), 2000);
  } catch {
    window.prompt('Copy the command', text);
  }
}
</script>
