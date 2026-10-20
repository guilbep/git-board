<!-- Cut a release (internal/release Draft, Cut): from what the followed
     environment runs, a version and where it goes (a candidate, or the
     final), the checks, the notes drafted from the issues, and what the cut
     does. Cutting starts a job: the dialog closes into the page's
     ReleaseJob. The canvas's Cut a release board. -->
<template>
  <section class="gb-dialog gb-cut" role="dialog" aria-modal="true" aria-labelledby="cut-title">
    <header class="gb-dialog-head">
      <h2 id="cut-title">Cut a release</h2>
      <button type="button" class="gb-icon-btn" aria-label="Close" :disabled="sending" @click="emit('close')"><GbIcon name="close" /></button>
    </header>

    <div v-if="!draft" class="gb-dialog-body">
      <p v-if="error" class="epfl-alert epfl-alert-danger" role="alert">{{ error }}</p>
      <p v-else class="gb-hint" role="status">Drafting the release: the issues merged since the last final, and the checks…</p>
    </div>

    <form v-else class="gb-dialog-body" @submit.prevent="cut">
      <fieldset class="gb-cut-from">
        <legend class="gb-section-title gb-cut-legend">From</legend>
        <label class="epfl-check gb-cut-opt">
          <input type="radio" name="cut-from" checked />
          <span v-if="fromEnv">
            <b>What runs in {{ fromEnv.name }}:</b> <span class="gb-mono">{{ sha7(draft.from) }}</span> · {{ draft.fromVersion }}<template v-if="since"> · deployed {{ when(since.date, true) }}</template><br />
            <span class="gb-note">A release is made from what {{ fromEnv.name }} has tested.</span>
          </span>
          <span v-else>
            <b>The {{ trunkName }} branch's head:</b> <span class="gb-mono">{{ sha7(draft.from) }}</span> · {{ draft.fromVersion }}<br />
            <span class="gb-note">{{ followed ? `${followed.name}'s overlays don't say one commit they run` : 'No environment follows the trunk' }}, so a release is made from its head.</span>
          </span>
        </label>
        <label v-if="fromEnv && draft.undeployed.length" class="epfl-check gb-cut-opt gb-cut-off">
          <input type="radio" name="cut-from" disabled />
          <span>
            The {{ trunkName }} branch's head: {{ merges(draft.undeployed) }} not deployed yet<br />
            <span class="gb-note">Deploy {{ fromEnv.name }} first to release them.</span>
          </span>
        </label>
      </fieldset>

      <div class="gb-cut-version">
        <div class="gb-field">
          <label class="epfl-label" for="cut-version">Version</label>
          <input id="cut-version" ref="versionInput" v-model="version" class="epfl-input" autocomplete="off" data-1p-ignore :aria-invalid="!tag || finalTaken" aria-describedby="cut-hint" />
        </div>
        <div v-if="candidateEnv || lastEnv" class="gb-field">
          <span id="cut-kind" class="epfl-label">Goes to</span>
          <div class="gb-seg" role="group" aria-labelledby="cut-kind">
            <button v-if="candidateEnv" type="button" :aria-pressed="!final" @click="final = false">A candidate for {{ candidateEnv.name }}</button>
            <button v-if="lastEnv" type="button" :aria-pressed="final" @click="final = true">A final release for {{ lastEnv.name }}</button>
          </div>
        </div>
      </div>
      <p id="cut-hint" class="gb-hint gb-cut-hint">{{ hint[0] }} <b v-if="hint[1]" class="gb-fg">{{ hint[1] }}</b>{{ hint[2] }}</p>

      <section aria-labelledby="cut-checks">
        <div class="gb-side-head gb-cut-head">
          <h3 id="cut-checks" class="gb-section-title">Before it is cut</h3>
          <span v-if="loading" class="gb-note" role="status">Checking again…</span>
        </div>
        <ul class="gb-checks" :class="{ 'gb-cut-stale': loading }">
          <li v-for="c in draft.checks" :key="c.text">
            <GbIcon :name="checkIcon(c.ok).name" :class="checkIcon(c.ok).cls" />
            <span>{{ c.text }}<small v-if="c.detail">{{ c.detail }}</small></span>
          </li>
        </ul>
        <p v-if="error" class="epfl-alert epfl-alert-danger gb-cut-gap" role="alert">{{ error }}</p>
      </section>

      <section aria-labelledby="cut-notes-h">
        <div class="gb-side-head gb-cut-head">
          <h3 id="cut-notes-h" class="gb-section-title">Notes</h3>
          <span class="gb-note">{{ draft.issues.length ? `Drafted from the ${plural(draft.issues.length, 'issue')} merged` : 'No issue merged' }} since {{ draft.previous?.name ?? 'the start' }}</span>
        </div>
        <label for="cut-notes" class="gb-sr">Release notes</label>
        <div class="gb-composer">
          <textarea id="cut-notes" v-model="notes" class="gb-textarea gb-cut-notes" />
        </div>
      </section>

      <section aria-labelledby="cut-what">
        <h3 id="cut-what" class="gb-section-title gb-cut-head">What happens</h3>
        <ol class="gb-plan">
          <li>
            <span class="gb-plan-body">Push the tag <span class="gb-mono">{{ tag || '…' }}</span> on <span class="gb-mono">{{ sha7(draft.from) }}</span> to {{ remote }}<small v-if="next">If someone took that name meanwhile, git-board offers {{ next }}.</small></span>
          </li>
          <li v-if="binaries.length">
            <span class="gb-plan-body">Build the archives in a clean checkout of the tag and publish them as a GitHub release<small>{{ binaries.join(', ') }}</small></span>
          </li>
          <li v-if="!binaries.length">
            <span class="gb-plan-body">Name the images {{ tag || '…' }}<template v-if="registries.length"> in {{ list.format(registries) }}</template><template v-if="chart">, and package the chart as {{ tag.slice(1) || '…' }}</template></span>
          </li>
          <li v-if="deploy && toEnv">
            <span class="gb-plan-body">Point {{ toEnv.name }} at them in {{ list.format([...new Set(toEnv.targets.map((t) => repoName(t.repo)))]) }}<small>Argo deploys {{ toEnv.name }} from there.</small></span>
          </li>
          <li><span class="gb-plan-body">Keep the release in git, with its notes</span></li>
        </ol>
        <p v-if="!deploy && toEnv" class="gb-note gb-cut-gap">{{ toEnv.name }}'s overlays don't say which commit they run yet, so this cuts without deploying. Adopt them on the Releases page to deploy there.</p>
      </section>
      <p v-if="sendError" class="epfl-alert epfl-alert-danger" role="alert"><b>Not cut.</b> {{ sendError }}</p>
    </form>

    <footer class="gb-dialog-foot">
      <span class="gb-note gb-grow">If your laptop closes halfway, the next git-board to open, yours or anyone's, finishes it.</span>
      <button type="button" class="epfl-btn epfl-btn-secondary" :disabled="sending" @click="emit('close')">Cancel</button>
      <button type="button" class="epfl-btn epfl-btn-primary" :disabled="!ready" @click="cut">{{ draft && tag ? cutLabel({ ...draft, tag }, deploy, toEnv?.name ?? '') : 'Cut' }}</button>
    </footer>
  </section>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue';
import { cutRelease, getDraft, type CutRequest, type Draft, type EnvState, type ReleaseJob, type ReleaseRow } from '@/api/board';
import { message } from '@/issue';
import { checkIcon, cutHint, cutLabel, finalOf, newest, nextTag, plural, registryName, repoName, sha7, tagPattern, takenOf } from '@/releases';
import { when } from '@/when';
import GbIcon from '@/components/GbIcon.vue';

// busy: a job runs, or the page waits for an answer; remote, trunk: as
// the server names them (Releases); sending: the cut's POST is out
const props = defineProps<{ envs: EnvState[]; binaries: string[]; releases: ReleaseRow[]; remote: string; trunk: string; busy: boolean }>();
const emit = defineEmits<{ close: []; started: [job: ReleaseJob, cut: CutRequest] }>();
const sending = defineModel<boolean>('sending', { default: false });

const followed = computed(() => props.envs.find((e) => e.role === 'follow'));
const candidateEnv = computed(() => props.envs.find((e) => e.role === 'candidate'));
const lastEnv = computed(() => props.envs.find((e) => e.role === 'last'));

const final = ref(!candidateEnv.value); // without a candidate environment, every cut is a final
const draft = ref<Draft | null>(null);
const loading = ref(false);
const error = ref(''); // the draft failed
const sendError = ref('');
const version = ref('');
const notes = ref('');
const versionInput = ref<HTMLInputElement | null>(null);
const list = new Intl.ListFormat('en', { type: 'conjunction' });

// the draft, again when "Goes to" changes: the tag and the checks depend
// on it; what was typed in Version and Notes stays. A slower answer to an
// earlier ask is dropped.
let asked = 0;
async function fetchDraft() {
  const n = ++asked;
  loading.value = true;
  error.value = '';
  try {
    const d = await getDraft(final.value);
    if (n !== asked) return;
    const was = draft.value;
    if (!was || version.value === finalOf(was.proposed)) version.value = finalOf(d.proposed);
    if (!was || notes.value === was.notes) notes.value = d.notes;
    draft.value = d;
    if (!was) void nextTick(() => versionInput.value?.focus());
  } catch (e) {
    if (n === asked) error.value = message(e);
  } finally {
    if (n === asked) loading.value = false;
  }
}
watch(final, fetchDraft);
onMounted(fetchDraft);

const fromEnv = computed(() => (followed.value && followed.value.commit === draft.value?.from ? followed.value : undefined));
const since = computed(() => newest(fromEnv.value?.targets ?? []));
const toEnv = computed(() => (final.value ? lastEnv.value : candidateEnv.value));
// a cut deploys where it goes, unless that environment isn't adopted: the
// engine would refuse the deploy, so it only cuts
const deploy = computed(() => !!toEnv.value?.adopted);
const typed = computed(() => version.value.trim().replace(/^v/, ''));
// the draft's tag while Version is its proposal; else worked out here
const tag = computed(() => {
  const d = draft.value;
  if (!d) return '';
  return typed.value === finalOf(d.proposed) && !loading.value ? d.tag : nextTag(typed.value, final.value, props.releases);
});
// what of that version is cut already: rc.1, rc.2; a final, itself
const taken = computed(() => takenOf(typed.value, final.value, props.releases));
const finalTaken = computed(() => final.value && taken.value.length > 0);
const hint = computed(() => (draft.value ? cutHint(draft.value, tag.value, taken.value, final.value) : ['', '', '']));
// the trunk, as the server names it; else the followed environment's name
const trunkName = computed(() => props.trunk || followed.value?.name || 'trunk');
// the candidate after this one, offered when the tag is taken
const next = computed(() => {
  const rc = tagPattern.exec(tag.value)?.[4];
  return rc ? `rc.${+rc + 1}` : '';
});
// where images go, as the releases' records say: ghcr, quay
const registries = computed(() => [...new Set(props.releases.flatMap((r) => r.record?.artifacts.map((a) => registryName(a.registry)) ?? []))]);
const chart = computed(() => props.envs.some((e) => e.targets.some((t) => t.deployed.chart)));
const ready = computed(() => !!draft.value && !!tag.value && !finalTaken.value && !loading.value && !sending.value && !props.busy);
// the merges not deployed, three at most
const merges = (b: string[]) => `${plural(b.length, 'merge')} (${[...b.slice(0, 3).map((s) => `“${s}”`), ...(b.length > 3 ? [`${b.length - 3} more`] : [])].join(', ')})`;

// the cut, of the commit shown; the notes only when changed, so a commit
// already recorded (a candidate's) keeps its own
async function cut() {
  if (!ready.value || !draft.value) return;
  const req: CutRequest = { version: typed.value, final: final.value, commit: draft.value.from, notes: notes.value === draft.value.notes ? '' : notes.value, deploy: deploy.value };
  sending.value = true;
  sendError.value = '';
  try {
    emit('started', await cutRelease(req), req);
  } catch (e) {
    sendError.value = message(e);
  } finally {
    sending.value = false;
  }
}
</script>

<style scoped>
.gb-cut { width: min(56rem, calc(100vw - 2rem)); max-height: calc(100vh - 2rem); overflow-y: auto; }
.gb-cut .gb-dialog-body { gap: 1.5rem; }
.gb-cut-from { font-size: 1rem; display: flex; flex-direction: column; gap: .5rem; margin: 0; padding: 0; border: 0; min-width: 0; }
.gb-cut-legend { margin-bottom: .5rem; padding: 0; }
.gb-cut-opt { align-items: flex-start; }
.gb-cut-opt input { margin-top: .2rem; }
.gb-cut-off { color: var(--fg-muted); }
.gb-cut-version { display: grid; grid-template-columns: 12rem minmax(0, 1fr); gap: 1rem 1.25rem; align-items: end; }
.gb-cut-hint { display: block; margin-top: -.75rem; }
.gb-cut-head { margin-bottom: .75rem; }
.gb-cut-stale { opacity: .5; }
.gb-cut-notes { min-height: 13rem; font-family: var(--font-mono); font-size: .8125rem; }
.gb-cut-gap { margin-top: .75rem; }
@media (max-width: 47.99rem) {
  .gb-cut-version { grid-template-columns: minmax(0, 1fr); }
}
</style>
