<!-- Settings › Sync: where your issues go, one table, a row per place: the
     git remote (origin), each forge, and the GitHub Project as a line under
     its GitHub row, each with its last check. Then Check and Sync, the whole
     sync (internal/server/whole.go): git pull, the forges, the Project, git
     push. Sync waits for a clean Check of the last 15 minutes, and is
     confirmed with its counts, with an explicit "I understand" when it
     creates many issues on a forge. Below, what a sync would do or did, per
     forge. #/settings/sync?add=1 opens "Add a forge", for one no remote
     names. -->
<template>
  <div class="set-panel sy">
    <div class="sy-head">
      <div>
        <h2 class="h4">Sync</h2>
        <p class="sy-lead">
          Your issues travel with git, like your code; the forges and the GitHub Project are mirrors, for the people who work there.
          One sync moves them all: a git pull from {{ remote }}, the forges, the Project's fields, then a git push. <code>git board sync</code> runs the same.
        </p>
      </div>
      <div class="sy-actions">
        <button type="button" class="epfl-btn epfl-btn-secondary" @click="guideOpen = true">How forges work</button>
        <button type="button" class="epfl-btn epfl-btn-secondary" @click="adding = true">+ Add a forge</button>
      </div>
    </div>
    <p v-if="error" class="epfl-alert epfl-alert-danger" role="alert">{{ error }}</p>

    <div v-if="st" class="gb-table-wrap">
      <table class="gb-table">
        <thead><tr><th>Where</th><th>Project</th><th>Sync</th><th>Last check</th><th><span class="sy-hidden">Remove</span></th></tr></thead>
        <tbody>
          <tr>
            <td><b>{{ remote }}</b> <span class="sy-kind">git</span></td>
            <td class="sy-wrap">
              <template v-if="st.remote">{{ st.remote.url }}<div v-if="st.remote.push_urls.join(' ') !== st.remote.url" class="sy-note">Pushes to {{ st.remote.push_urls.join(', ') }}</div></template>
              <template v-else>None: <code>git remote add {{ remote }} &lt;url&gt;</code>. The git steps are skipped.</template>
            </td>
            <td>{{ st.remote ? 'On' : '—' }}</td>
            <td>{{ remoteLine }}</td>
            <td />
          </tr>
          <template v-for="row in rows" :key="row.f?.name ?? 'project'">
            <tr v-if="!row.f" class="sy-sub">
              <td><span class="sy-kind">GitHub Project</span></td>
              <td class="sy-wrap">
                <template v-if="st.project.project">{{ st.project.project }}</template>
                <template v-else>None. <code>git board bridge configure --owner … --project …</code> sets one up.</template>
              </td>
              <td>{{ st.project.project ? (st.project.write ? 'Read and written' : 'Read only') : '—' }}</td>
              <td>{{ projectLine }} <router-link v-if="st.project.project" to="/conflicts">Conflicts</router-link></td>
              <td />
            </tr>
            <template v-else>
              <tr>
                <td><b>{{ row.f.name }}</b> <span class="sy-kind">{{ kindName(row.f.target) }}</span></td>
                <td class="sy-wrap">
                  <a :href="row.f.where" target="_blank" rel="noopener noreferrer">{{ row.f.project }}</a>
                  <div class="sy-note">{{ row.f.discovered ? 'found in the remotes' : 'in .git-board.toml' }}</div>
                </td>
                <td>
                  <span v-if="row.f.discovered" title="A forge found in the remotes syncs: give the forges tables in .git-board.toml to leave one out">On</span>
                  <label v-else class="sy-switch">
                    <input type="checkbox" :checked="row.f.sync" :disabled="busy === row.f.name || running" @change="toggle(row.f, ($event.target as HTMLInputElement).checked)" />
                    <span>{{ row.f.sync ? 'On' : 'Off' }}</span>
                  </label>
                </td>
                <td>{{ forgeLine(row.f) }}</td>
                <td><button v-if="!row.f.discovered" type="button" class="gb-linkbtn" :disabled="busy === row.f.name || running" @click="confirmingRemove = row.f.name">Remove</button></td>
              </tr>
              <tr v-if="confirmingRemove === row.f.name" class="sy-confirm-row">
                <td colspan="5">
                  <p role="alert"><b>Remove the forge {{ row.f.name }}?</b> Its settings go. The issues it brought stay, still linked to {{ kindName(row.f.target) }}, and the stored token stays too.</p>
                  <button type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="busy === row.f.name" @click="remove(row.f)">Remove {{ row.f.name }}</button>
                  <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" @click="confirmingRemove = ''">Cancel</button>
                </td>
              </tr>
            </template>
          </template>
        </tbody>
      </table>
    </div>
    <p v-else-if="!error" class="sy-note">Loading…</p>
    <details v-if="st?.skipped.length" class="sy-skipped">
      <summary>{{ plural(st.skipped.length, 'remote URL') }} named no forge</summary>
      <ul><li v-for="s in st.skipped" :key="s">{{ s.replace(/^skipped /, '') }}</li></ul>
    </details>

    <div class="sy-run">
      <button type="button" class="epfl-btn epfl-btn-secondary" :disabled="starting || running" @click="start(true)">{{ running && job?.dry_run ? 'Checking…' : 'Check' }}</button>
      <button type="button" class="epfl-btn epfl-btn-primary" :disabled="!canRun" @click="confirming = true">{{ running && !job?.dry_run ? 'Syncing…' : 'Sync' }}</button>
      <span class="sy-note">Check says what a sync would do, and writes nothing. Sync needs a clean check of the last 15 minutes.</span>
    </div>
    <p v-if="startError" class="epfl-alert epfl-alert-danger" role="alert"><b>Not started.</b> {{ startError }}</p>

    <!-- the last run: its steps, then per forge -->
    <section v-if="job" class="sy-report" aria-labelledby="sy-report">
      <div class="sy-report-head">
        <h3 id="sy-report" class="h5">{{ reportTitle }}</h3>
        <span class="gb-hint">{{ job.dry_run ? `Check of ${when(job.started, true)}. Nothing was written.` : running ? 'Edits wait: changes you make in git-board now are saved right after the run. It goes on if you leave this page.' : '' }}</span>
      </div>
      <ol v-if="job.steps?.length" class="sy-steps">
        <li v-for="s in job.steps" :key="s.name" :class="`sy-step-${s.state}`">
          <GbIcon :name="stepIcon[s.state]" small /> {{ stepLine(s, remote, job.dry_run) }}
        </li>
      </ol>
      <SyncProgress v-if="running && forgeStep === 'running'" :job="job" :bridges="forgeStatus?.bridges ?? []" :starting="job.dry_run ? 'Checking the forges…' : 'Syncing the forges…'" />
      <p v-if="job.state === 'failed'" class="epfl-alert epfl-alert-danger" role="alert">
        <b>{{ job.dry_run ? 'The check' : 'The sync' }} stopped{{ failedAt ? ` at ${failedAt}` : '' }}.</b> {{ failedAt ? '' : `${job.error}.` }} Nothing already written was undone.
      </p>
      <p v-else-if="job.dry_run && job.state === 'done' && job.errors" class="epfl-alert epfl-alert-danger" role="alert">
        <b>The check had {{ plural(job.errors, 'error') }}</b> (see the details below). Fix {{ job.errors === 1 ? 'it' : 'them' }} and check again: a sync needs a clean check.
      </p>
      <div v-if="bridgeRows.length" class="gb-forges">
        <article v-for="b in bridgeRows" :key="b.name" class="gb-forge">
          <header class="gb-forge-head">
            <h4 class="sy-forge-title">{{ b.label }}</h4>
            <span class="gb-grow" />
            <span v-if="b.tally?.errors" class="gb-state gb-state-err"><GbIcon name="error" small /> {{ plural(b.tally.errors, 'error') }}</span>
            <span v-else-if="b.tally?.warnings" class="gb-hint">{{ plural(b.tally.warnings, 'warning') }}</span>
          </header>
          <div class="gb-forge-body">
            <p v-if="job.dry_run && (b.tally?.to.issues ?? 0) >= bigRun" class="epfl-alert epfl-alert-danger">
              <b>Creates {{ b.tally!.to.issues.toLocaleString() }} issues on {{ b.label }}.</b>
              {{ b.label }} may email everyone watching the project, once per issue. Ask watchers to mute it first, or switch its sync off.
            </p>
            <div class="gb-counts">
              <dl class="sy-countlist">
                <dt class="gb-section-title"><GbIcon name="up" small /> To {{ b.label }}</dt>
                <div><dt>Issues {{ job.dry_run ? 'to create' : 'created' }}</dt><dd :class="{ 'gb-num-alert': (b.tally?.to.issues ?? 0) >= bigRun }">{{ n(b.tally?.to.issues) }}</dd></div>
                <div><dt>Linked issues with changes</dt><dd>{{ n(b.tally?.to.changed) }}</dd></div>
                <div><dt>Comments {{ job.dry_run ? 'to post' : 'posted' }}</dt><dd>{{ n(b.tally?.to.comments) }}</dd></div>
                <div><dt>Comment edits</dt><dd>{{ n(b.tally?.to.edits) }}</dd></div>
                <div><dt>Title, state and label changes</dt><dd>{{ n(b.tally?.to.changes) }}</dd></div>
              </dl>
              <dl v-if="job.mode !== 'send'" class="sy-countlist">
                <dt class="gb-section-title"><GbIcon name="down" small /> From {{ b.label }}</dt>
                <div><dt>Issues {{ job.dry_run ? 'to import' : 'imported' }}</dt><dd>{{ n(b.tally?.from.issues) }}</dd></div>
                <div><dt>Linked issues with changes</dt><dd>{{ n(b.tally?.from.changed) }}</dd></div>
                <div><dt>{{ job.dry_run ? 'Changes, up to' : 'Changes pulled' }}</dt><dd>{{ n((b.tally?.from.changes ?? 0) + (b.tally?.from.comments ?? 0) + (b.tally?.from.edits ?? 0)) }}</dd></div>
              </dl>
            </div>
            <p v-if="b.tally?.linked" class="gb-hint"><GbIcon name="link" small /> {{ plural(b.tally.linked, 'issue') }} found on {{ b.label }} by their git-board marker: linked, not created again.</p>
            <p v-if="b.tally?.assignable" class="gb-hint"><GbIcon name="user" small /> {{ b.tally.assignable }} {{ b.tally.assignable === 1 ? 'person' : 'people' }} can be assigned on {{ b.label }}: the assignee boxes offer them.</p>
            <p v-if="b.tally?.stays_local" class="gb-note"><GbIcon name="local" small /> {{ plural(b.tally.stays_local, 'text') }} stay local: no {{ b.label }} token for their authors.</p>
            <details v-if="b.tally?.lines.length" class="sy-lines">
              <summary>Details ({{ b.tally.lines.length }})</summary>
              <pre>{{ b.tally.lines.join('\n') }}</pre>
            </details>
          </div>
        </article>
      </div>
    </section>

    <q-dialog v-model="confirming">
      <section class="gb-dialog" role="dialog" aria-modal="true" aria-labelledby="sy-confirm-title">
        <header class="gb-dialog-head">
          <h2 id="sy-confirm-title" class="h5">Sync now?</h2>
          <button type="button" class="gb-icon-btn" aria-label="Cancel" @click="confirming = false"><GbIcon name="close" /></button>
        </header>
        <div class="gb-dialog-body">
          <p>From the check of {{ job ? when(job.started, true) : '' }}. If anything changed since, the sync does what is true then.</p>
          <ul class="gb-plain-list">
            <li v-if="st?.remote"><span><b>git pull from {{ remote }}</b>: {{ plural(st.remote.issues.receive, 'issue') }}</span></li>
            <li v-for="b in bridgeRows" :key="b.name"><span><b>{{ b.label }}</b>: {{ b.tally ? runWords(b.tally) : 'nothing to do' }}</span></li>
            <li v-if="st?.project.project"><span><b>The GitHub Project</b>: {{ (st.project.last && pendingWords(st.project.last)) || 'nothing to do' }}{{ st.project.write ? '' : ', nothing written to the Project' }}</span></li>
            <li v-if="st?.remote"><span><b>git push to {{ remote }}</b>: {{ plural(st.remote.issues.send, 'issue') }}, and what the steps before bring</span></li>
          </ul>
          <div v-for="b in bigRows" :key="b.name" class="epfl-alert epfl-alert-danger">
            <b>This creates {{ b.tally!.to.issues.toLocaleString() }} issues on {{ b.label }}, which may email everyone watching it.</b>
            Issues can’t be deleted in bulk afterwards.
            <label class="sy-check"><input v-model="understood" type="checkbox" :value="b.name" /> I understand: create {{ b.tally!.to.issues.toLocaleString() }} issues on {{ b.label }}</label>
          </div>
          <p class="gb-hint">A step that fails stops the steps after it. While it runs, edits in git-board wait and are saved right after it. It goes on if you leave this page.</p>
        </div>
        <footer class="gb-dialog-foot">
          <span class="gb-grow" />
          <button type="button" class="epfl-btn epfl-btn-secondary" @click="confirming = false">Cancel</button>
          <button type="button" class="epfl-btn epfl-btn-primary" :disabled="bigRows.some((b) => !understood.includes(b.name)) || starting" @click="runReal">Sync</button>
        </footer>
      </section>
    </q-dialog>

    <AddForgeDialog v-model="adding" @added="reload" @guide="guideOpen = true" />
    <ForgesGuide v-model="guideOpen" />
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { getWhole, removeBridge, setBridgeSync, startWhole, type Bridge, type WholeStatus } from '@/api/board';
import { forgeStatus, loadForges, onStep } from '@/forges';
import { forgeLabel, kindName } from '@/issuesync';
import { message } from '@/issue';
import { debounced, onLive } from '@/live';
import { when } from '@/me';
import { aheadWords, checkWords, pendingWords, plural, runWords, stepLine } from '@/ways';
import AddForgeDialog from '@/components/AddForgeDialog.vue';
import ForgesGuide from '@/components/ForgesGuide.vue';
import GbIcon from '@/components/GbIcon.vue';
import SyncProgress from '@/components/SyncProgress.vue';

// changed: the sync as it stands, for the Overview's line
const emit = defineEmits<{ changed: [st: WholeStatus] }>();

// a sync creating this many issues on a forge asks for an explicit "I understand"
const bigRun = 50;
const remote = 'origin'; // the whole sync's, as git-board sync's default

const route = useRoute();
const router = useRouter();
// "Add a forge" is open while the address says ?add=1; closing it drops it
const adding = computed({
  get: () => route.query.add === '1',
  set: (on) => void router.replace({ path: '/settings/sync', query: on ? { add: '1' } : {} }),
});
const guideOpen = ref(false);

const st = ref<WholeStatus | null>(null);
const error = ref('');
const startError = ref('');
const busy = ref(''); // a forge being switched or removed
const starting = ref(false);
const confirming = ref(false);
const confirmingRemove = ref('');
const understood = ref<string[]>([]);

// the job: as polled while it runs (forges.ts), else as last loaded
const job = computed(() => forgeStatus.value?.job ?? st.value?.job ?? null);
const running = computed(() => job.value?.state === 'running');
const forgeStep = computed(() => job.value?.steps?.find((s) => s.name === 'forges')?.state);
// a sync needs a clean check of the whole sync (the server also wants it fresh, 15 minutes)
const canRun = computed(() => !starting.value && job.value?.mode === 'all' && job.value.dry_run && job.value.state === 'done' && !job.value.errors);
const reportTitle = computed(() => {
  const j = job.value!;
  const what = { all: 'Sync', sync: 'The forges’ sync', send: 'Send' }[j.mode];
  if (j.dry_run) return j.state === 'running' ? 'Checking…' : 'What a sync would do';
  return j.state === 'running' ? `${what}: so far` : `${what} ${j.state === 'failed' ? 'stopped' : 'finished'} ${when(j.ended ?? j.started, true)}`;
});
// the step a run stopped at, in words; its error is on its line
const failedAt = computed(() => {
  const s = job.value?.steps?.find((x) => x.state === 'failed');
  return s ? stepLine({ ...s, note: '' }, remote, job.value!.dry_run).replace(/: failed$/, '') : '';
});
const stepIcon = { waiting: 'clock', running: 'sync', done: 'check', failed: 'error', skipped: 'forward' } as const;

const bridgeRows = computed(() => (job.value?.names ?? []).map((name) => {
  const f = st.value?.forges.find((x) => x.name === name);
  return { name, label: forgeLabel({ name, target: f?.target ?? '' }), tally: job.value?.bridges[name] };
}));
const bigRows = computed(() => bridgeRows.value.filter((b) => (b.tally?.to.issues ?? 0) >= bigRun));
const n = (v: number | undefined) => (v ?? 0).toLocaleString();

// the forges' rows, the Project's line under the first GitHub row, else last
const rows = computed(() => {
  const forges: { f?: Bridge }[] = (st.value?.forges ?? []).map((f) => ({ f }));
  const github = forges.findIndex((r) => r.f?.target === 'github');
  forges.splice(github < 0 ? forges.length : github + 1, 0, {});
  return forges;
});

const remoteLine = computed(() => {
  const r = st.value?.remote;
  if (!r) return '—';
  if (r.checked) return `${aheadWords(r.issues, 'issue')}, checked ${when(r.checked, true)}`;
  return r.tracked ? `${aheadWords(r.issues, 'issue')}, as of the last fetch or push` : 'Not checked yet';
});

function forgeLine(f: Bridge): string {
  if (!f.sync) return 'Off: left out of the sync';
  const j = job.value;
  const t = j?.bridges[f.name];
  if (!j || !t || j.state === 'running') return j?.state === 'running' && t ? 'Running…' : 'Not checked yet';
  if (j.state === 'failed') return `Stopped ${when(j.ended ?? j.started, true)}`;
  return j.dry_run ? `${checkWords({ [f.name]: t })}, checked ${when(j.ended ?? j.started, true)}` : `Synced ${when(j.ended ?? j.started, true)}`;
}

const projectLine = computed(() => {
  const p = st.value?.project;
  if (!p?.project) return '—';
  if (!p.last) return 'Not checked yet.';
  const pending = pendingWords(p.last);
  return `${plural(p.last.conflicts.length, 'conflict')}, checked ${when(p.last.checked_at, true)}${pending ? `; a sync would ${pending}` : ''}.`;
});

async function reload() {
  try {
    st.value = await getWhole();
    error.value = '';
    emit('changed', st.value);
  } catch (e) {
    error.value = message(e);
  }
}

async function start(dryRun: boolean) {
  starting.value = true;
  startError.value = '';
  try {
    forgeStatus.value = await startWhole(dryRun);
    void loadForges(); // polls while it runs
  } catch (e) {
    startError.value = message(e);
  } finally {
    starting.value = false;
  }
}

async function runReal() {
  confirming.value = false;
  await start(false);
}

async function toggle(f: Bridge, on: boolean) {
  busy.value = f.name;
  error.value = '';
  try {
    await setBridgeSync(f.name, on);
    f.sync = on;
    void loadForges(); // the header's
  } catch (e) {
    error.value = `Sync of ${f.name} not changed: ${message(e)}`;
    void reload();
  } finally {
    busy.value = '';
  }
}

async function remove(f: Bridge) {
  busy.value = f.name;
  error.value = '';
  try {
    await removeBridge(f.name);
    confirmingRemove.value = '';
    await reload();
    void loadForges();
  } catch (e) {
    error.value = `${f.name} not removed: ${message(e)}`;
  } finally {
    busy.value = '';
  }
}

// a fresh check: nothing understood yet; a run's end: the rows' lines
watch(() => job.value?.started, () => (understood.value = []));
watch(running, (now, was) => was && !now && void reload());
const reloadSoon = debounced(() => void reload());
const offLive = onLive((e) => {
  onStep(e);
  if (e.kind === 'sync' || e.kind === 'issues' || e.kind === 'settings' || e.kind === 'config') reloadSoon();
});
onBeforeUnmount(offLive);
onMounted(() => {
  void reload();
  void loadForges();
});
</script>

<style scoped>
.sy h2, .sy h3, .sy p { margin: 0; }
.sy-head { display: flex; flex-wrap: wrap; align-items: flex-end; justify-content: space-between; gap: 1rem; }
.sy-head h2 { margin-bottom: .375rem; }
.sy-lead { max-width: 44rem; color: var(--fg-muted); }
.sy-actions, .sy-run { display: flex; flex-wrap: wrap; align-items: center; gap: .75rem; }
.sy-note { font-size: .8125rem; color: var(--fg-muted); }
.sy-kind { margin-left: .25rem; font-size: .8125rem; color: var(--fg-muted); }
.sy-wrap { overflow-wrap: anywhere; }
.sy-sub td { border-top: 0; font-size: .875rem; }
.sy-sub td:first-child { padding-left: 1.75rem; }
.sy-hidden { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
.gb-table-wrap { position: relative; }
.sy-switch { display: inline-flex; align-items: center; gap: .5rem; cursor: pointer; font-weight: 700; }
.sy-switch input { width: 1.125rem; height: 1.125rem; accent-color: var(--primary-hover); }
.sy-confirm-row td { background: var(--warning-bg); }
.sy-confirm-row p { margin: 0 0 .5rem; }
.sy-confirm-row .epfl-btn + .epfl-btn { margin-left: .5rem; }
.sy-skipped { font-size: .875rem; color: var(--fg-muted); }
.sy-skipped ul { margin: .375rem 0 0; padding-left: 1.25rem; overflow-wrap: anywhere; }
.sy-report { display: grid; gap: 1rem; }
.sy-report-head { display: flex; flex-wrap: wrap; align-items: baseline; gap: .5rem 1.5rem; }
.sy-steps { display: grid; gap: .25rem; margin: 0; padding: 0; list-style: none; font-size: .9375rem; }
.sy-step-failed { color: var(--danger-fg); font-weight: 700; }
.sy-step-skipped, .sy-step-waiting { color: var(--fg-muted); }
.sy-forge-title { margin: 0; font-size: 1rem; font-weight: 700; }
.sy-countlist div { display: flex; justify-content: space-between; gap: .75rem; padding: .5rem 0; border-bottom: var(--border-w) solid var(--border-subtle); }
.sy-countlist dd { margin: 0; font-weight: var(--w-bold); font-variant-numeric: tabular-nums; }
.sy-countlist > dt { margin-bottom: .25rem; }
.sy-lines pre { max-height: 20rem; overflow: auto; padding: .75rem; background: var(--bg-subtle); font-size: var(--fs-xs); }
.sy-check { display: flex; gap: .5rem; align-items: center; margin-top: .5rem; }
.sy-check input { accent-color: var(--primary-hover); }
.gb-linkbtn { padding: 0; border: 0; background: none; color: var(--fg); text-decoration: underline; cursor: pointer; font: inherit; }
</style>
