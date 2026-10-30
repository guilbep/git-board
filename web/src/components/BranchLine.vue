<!-- The branch line under the fields (docs/design/issue-head/README.md,
     16c, 16d; decisions 3 and 5): a chip per branch whose name names the
     issue (internal/branches), colored by whose move, four of them, then
     "+N more", merged and closed folded at the end, and the plus that
     proposes the next branch in the issue's folder. A chip opens its
     branch: the gate's checks (required ones flagged), its review, Merge;
     Ask for review for a branch on GitHub without one; the push command
     for a local one. Today's Development section (IssueBranches), moved. -->
<template>
  <div class="gb-bline" role="group" aria-label="Branches">
    <GbIcon name="branch" small class="gb-bline-mark" />
    <span v-if="error" class="gb-bline-note" role="alert">Couldn’t read the branches. {{ error }}</span>
    <template v-else-if="dev">
      <span v-if="!items.length" class="gb-bline-note">No branch yet</span>
      <GbPopover v-for="it in line.chips" :key="it.name" :label="it.state.sentence" :title="it.name" :button-class="`gb-bchip is-${it.state.tone}`">
        <template #button><BranchFace :state="it.state" /></template>
        <BranchDetail :item="it" :dev="dev" :pulls="pulls" :busy="busy" @ask="ask" @merge="merge" />
      </GbPopover>
      <GbPopover v-if="folded.length" :label="foldedLabel" title="The other branches" button-class="gb-bchip is-plain">
        <template #button>{{ foldedLabel }}</template>
        <details v-for="it in folded" :key="it.name" class="gb-bfold">
          <summary :title="it.state.sentence"><BranchFace :state="it.state" /></summary>
          <BranchDetail :item="it" :dev="dev" :pulls="pulls" :busy="busy" @ask="ask" @merge="merge" />
        </details>
      </GbPopover>
      <GbPopover label="Create a branch for this issue" title="Create a branch" button-class="gb-icon-btn gb-bline-plus" @open="opened">
        <template #button><GbIcon name="plus" /></template>
        <template v-if="made">
          <p class="gb-pop-note gb-bmade" role="status">
            <GbIcon :name="made.pushed ? 'check' : 'warn'" small />
            <span v-if="made.pushed">Created <b class="gb-mono">{{ made.created }}</b> on origin. In your clone:</span>
            <span v-else>Created <b class="gb-mono">{{ made.created }}</b> here only: origin refused it. {{ made.push_error }} Push it from your clone:</span>
          </p>
          <pre class="gb-pop-cmd"><code>{{ after }}</code></pre>
          <div class="gb-bactions">
            <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" @click="copy(after, 'after')"><GbIcon :name="copied === 'after' ? 'check' : 'copy'" small /> {{ copied === 'after' ? 'Copied' : 'Copy' }}</button>
            <button type="button" class="gb-linkbtn" @click="opened">Another branch</button>
          </div>
        </template>
        <template v-else>
          <label class="epfl-label" :for="`branch-name-${id}`">Branch name</label>
          <input :id="`branch-name-${id}`" v-model="name" type="search" class="epfl-input gb-mono gb-pop-input" :name="`gb-branch-${id}`" autocomplete="off" data-1p-ignore data-lpignore="true" data-form-type="other" spellcheck="false" :disabled="creating" />
          <p class="gb-pop-note">In the issue’s folder, from <span class="gb-mono">{{ dev.start }}</span>: git-board makes it and pushes it to origin, your checkout stays as it is. Or run it yourself:</p>
          <pre class="gb-pop-cmd"><code>{{ commands }}</code></pre>
          <div class="gb-bactions">
            <button type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="!name.trim() || creating" @click="create"><GbIcon name="branch" small /> {{ creating ? 'Creating…' : 'Create' }}</button>
            <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="!name.trim()" @click="copy(commands, 'create')">
              <GbIcon :name="copied === 'create' ? 'check' : 'copy'" small /> {{ copied === 'create' ? 'Copied' : 'Copy the commands' }}
            </button>
          </div>
          <p v-if="createError" class="gb-pop-note gb-berror" role="alert">Not created. {{ createError }}</p>
        </template>
      </GbPopover>
      <span v-if="envLines.length" class="gb-envline gb-bline-envs" aria-label="Environments">
        <template v-for="(p, i) in envLines" :key="p.text">
          <router-link v-if="envLinks[i]" :to="envLinks[i]!" :class="markClass(p.mark)">{{ p.text }}</router-link>
          <span v-else :class="markClass(p.mark)">{{ p.text }}</span>
        </template>
      </span>
      <span v-if="gateError" class="gb-bline-note" role="alert">Couldn’t reach GitHub. {{ gateError }}</span>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue';
import { createBranch, getBranches, getIssueEnvironments, getPulls, mergePull, openPull, type CreatedBranch, type GatePR, type IssueBranches, type IssueEnvironment, type IssuePulls } from '@/api/board';
import { branchItems, branchLine, folderName } from '@/branchstate';
import { actThenRead } from '@/gate';
import { message } from '@/issue';
import { envLine, markClass, releasePath, tagPattern } from '@/releases';
import BranchDetail from '@/components/BranchDetail.vue';
import BranchFace from '@/components/BranchFace.vue';
import GbIcon from '@/components/GbIcon.vue';
import GbPopover from '@/components/GbPopover.vue';

const props = defineProps<{ id: string; labels: string[]; title: string }>();

// the kind and the title name the branch: reread when they change
const dev = ref<IssueBranches | null>(null);
const error = ref('');
async function load() {
  try {
    dev.value = await getBranches(props.id);
    error.value = '';
    void loadEnvs();
  } catch (e) {
    error.value = message(e);
  }
}
watch(() => [props.id, props.labels.join(' '), props.title], load, { immediate: true });

// the gate: read for the issue, after Ask for review or Merge, and every
// 15 s while GitHub still checks, only while the page shows
const pulls = ref<IssuePulls | null>(null);
const gateError = ref('');
const busy = ref('');
let timer: ReturnType<typeof setTimeout> | undefined;
async function loadPulls() {
  clearTimeout(timer);
  try {
    pulls.value = await getPulls(props.id);
    gateError.value = '';
  } catch (e) {
    gateError.value = message(e);
  }
  if (pulls.value?.pulls.some((p) => p.pr && p.pr.state === 'OPEN' && (p.pr.checking || p.pr.checks.some((c) => c.state === 'pending' || c.state === 'expected')))) later();
}
function later() {
  timer = setTimeout(() => (document.hidden ? later() : loadPulls()), 15000);
}
watch(() => props.id, () => ((pulls.value = null), void loadPulls()), { immediate: true });
onUnmounted(() => clearTimeout(timer));

const items = computed(() => (dev.value ? branchItems(dev.value.branches, pulls.value?.pulls, dev.value.base) : []));
const line = computed(() => branchLine(items.value));
const folded = computed(() => [...line.value.more, ...line.value.merged, ...line.value.closed]);
const foldedLabel = computed(() => {
  const l = line.value;
  return [l.more.length && `+${l.more.length} more`, l.merged.length && `${l.merged.length} merged`, l.closed.length && `${l.closed.length} closed`].filter(Boolean).join(' · ');
});

async function act(what: string, run: () => Promise<unknown>) {
  busy.value = what;
  try {
    gateError.value = await actThenRead(run, async () => (await loadPulls(), gateError.value));
  } finally {
    busy.value = '';
  }
}
const ask = (branch: string) => act(branch, () => openPull(props.id, branch));
function merge(pr: GatePR) {
  if (!window.confirm(`Merge ${pr.title ? `“${pr.title}”` : `#${pr.number}`} into ${pr.base} on GitHub?`)) return;
  void act(`#${pr.number}`, () => mergePull(props.id, pr.number, pr.head));
}

// the next branch, in the issue's folder (decision 3)
const proposed = computed(() => (dev.value ? folderName(dev.value.suggested, dev.value.branches.map((b) => b.name)) : ''));
const name = ref('');
const commands = computed(() => `git switch -c ${name.value.trim()} ${dev.value?.start ?? ''}\ngit push -u origin ${name.value.trim()}`);

// Create (decision 3: both, Create and Copy): the server makes it from
// the start and pushes it; the answer lists the branches, the new one too
const made = ref<CreatedBranch | null>(null);
const creating = ref(false);
const createError = ref('');
function opened() {
  made.value = null;
  createError.value = '';
  name.value = proposed.value;
}
async function create() {
  creating.value = true;
  createError.value = '';
  try {
    const res = await createBranch(props.id, name.value.trim());
    made.value = res;
    dev.value = res;
    void loadPulls();
    void loadEnvs();
  } catch (e) {
    createError.value = message(e);
  } finally {
    creating.value = false;
  }
}
// what the person runs next: switch to it, or push it
const after = computed(() => (made.value?.pushed ? `git fetch origin\ngit switch ${made.value.created}` : `git push -u origin ${made.value?.created ?? ''}`));
const copied = ref('');
async function copy(text: string, which: string) {
  try {
    await navigator.clipboard.writeText(text);
    copied.value = which;
    setTimeout(() => (copied.value = ''), 2000);
  } catch {
    window.prompt('Copy the commands', text.replace('\n', ' && '));
  }
}

// where the issue's branch has got to; without [release], nothing
const issueEnvs = ref<IssueEnvironment[]>([]);
const envLines = computed(() => envLine(issueEnvs.value));
const envLinks = computed(() => issueEnvs.value.map((e) => (e.in && tagPattern.test(e.version) ? releasePath(e.version) : '')));
async function loadEnvs() {
  issueEnvs.value = [];
  if (!dev.value?.branches.length) return;
  try {
    issueEnvs.value = await getIssueEnvironments(props.id);
  } catch {
    issueEnvs.value = [];
  }
}
</script>
