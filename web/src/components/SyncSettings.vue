<!-- Settings › Sync: where your issues go, one table, a row per place: each
     git remote and each forge (GET /api/sync). A remote's row fetches,
     pulls or pushes at once (POST /api/remotes/<name>/<action>); a forge's
     row switches its sync on or off, or removes its table (PATCH
     /api/config). Then Check and Sync: a run in the background (POST
     /api/sync), its steps as they come (progress events). Check is the
     forges' dry run; Sync the whole sync: git pull, the forges, git push,
     confirmed first, with "allow creating" for creations a forge holds.
     #/settings/sync?add=1 opens "Add a forge", for one no remote names. -->
<template>
  <div class="set-panel sy">
    <div class="sy-head">
      <div>
        <h2 class="h4">Sync</h2>
        <p class="sy-lead">
          Where your issues go besides this clone: one Sync pulls {{ remote }}, syncs the forges, then pushes. Each row does it for its side alone.
          The board is the one truth — each forge and the GitHub Project mirror the fields you give them, and every write either side accepts is recorded, so switching one on, off or round moves only what changed since.
          The forges are kept in <code>.git-board.toml</code>; the tokens, in git's credential helper. <button type="button" class="gb-linkbtn" @click="guideOpen = true">How forges work</button>
        </p>
      </div>
      <div class="sy-actions">
        <button type="button" class="epfl-btn epfl-btn-secondary" @click="adding = true">+ Add a forge</button>
      </div>
    </div>
    <p v-if="error" class="epfl-alert epfl-alert-danger" role="alert">{{ error }}</p>

    <div v-if="st" class="gb-table-wrap">
      <table class="gb-table">
        <thead><tr><th>Where</th><th>Project</th><th>Sync</th><th>Last check</th><th>Actions</th><th><span class="sy-hidden">Remove</span></th></tr></thead>
        <tbody>
          <tr v-if="!st.remotes.length">
            <td><b>{{ remote }}</b> <span class="sy-kind">git</span></td>
            <td colspan="5" class="sy-wrap">{{ kept ? `None: not needed, ${kept}.` : `None: this clone has no ${remote} remote, so the git steps are skipped.` }}</td>
          </tr>
          <tr v-for="r in st.remotes" :key="r">
            <td><b>{{ r }}</b> <span class="sy-kind">git</span></td>
            <td class="sy-wrap"><span>{{ forgeOf(r) }}</span></td>
            <td>{{ kept ? '—' : 'On' }}</td>
            <td v-if="kept" class="sy-off" aria-disabled="true" :title="noGit('comparing the board with a remote')">Not compared: {{ kept }}</td>
            <td v-else>{{ remoteLine[r] ?? 'Not checked from here yet' }}</td>
            <td class="sy-acts">
              <button v-for="a in remoteActs" :key="a.action" type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :title="noGit(a.needs) || a.command" :disabled="!!rowBusy || running || !!noGit(a.needs)" @click="remoteDo(r, a.action)">
                {{ rowBusy === `${r} ${a.action}` ? `${a.label}…` : a.label }}
              </button>
            </td>
            <td />
          </tr>
          <template v-for="f in forges" :key="f.name">
            <tr>
              <td><b>{{ f.name }}</b> <span class="sy-kind">{{ kindName(f.kind) }}</span></td>
              <td class="sy-wrap">
                <span>{{ f.project }}</span>
                <div class="sy-note">
                  {{ f.discovered ? 'found in the remotes' : 'in .git-board.toml' }}
                  <template v-if="placesLine(f.name)"> · fields live here: {{ placesLine(f.name) }} · <router-link to="/settings/fields">change it in Fields</router-link></template>
                </div>
              </td>
              <td>
                <span v-if="f.discovered" title="A forge found in the remotes syncs: give the forges tables in .git-board.toml to leave one out">On</span>
                <label v-else class="sy-switch">
                  <input type="checkbox" :checked="f.sync" :disabled="busy === f.name || running" @change="toggle(f.name, ($event.target as HTMLInputElement).checked)" />
                  <span>{{ f.sync ? 'On' : 'Off' }}</span>
                </label>
                <div v-if="!f.discovered" class="sy-note">
                  <select class="sy-way" :value="f.way" :disabled="busy === f.name || running" :aria-label="`The way of ${f.name}`" @change="setWay(f.name, ($event.target as HTMLSelectElement).value as ForgeWay)">
                    <option value="both">both ways</option>
                    <option value="push">push only, a backup</option>
                    <option value="pull">read only</option>
                  </select>
                </div>
              </td>
              <td>{{ f.login ? `as ${f.login}` : 'no token found' }}</td>
              <td />
              <td><button v-if="!f.discovered" type="button" class="gb-linkbtn" :disabled="busy === f.name || running" @click="confirmingRemove = f.name">Remove</button></td>
            </tr>
            <tr v-if="confirmingRemove === f.name" class="sy-confirm-row">
              <td colspan="6">
                <p role="alert"><b>Remove the forge {{ f.name }}?</b> Its settings go. The issues it brought stay, still linked to {{ kindName(f.kind) }}, and the stored token stays too.</p>
                <button type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="busy === f.name" @click="remove(f.name)">Remove {{ f.name }}</button>
                <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" @click="confirmingRemove = ''">Cancel</button>
              </td>
            </tr>
          </template>
        </tbody>
      </table>
    </div>
    <p v-else-if="!error" class="sy-note">Loading…</p>

    <div class="sy-run">
      <button type="button" class="epfl-btn epfl-btn-secondary" :disabled="starting || running || !forges.length" @click="start({ scope: 'forges', dry_run: true })">{{ running && run?.dry_run ? 'Checking…' : 'Check' }}</button>
      <button type="button" class="epfl-btn epfl-btn-primary" :disabled="starting || running" @click="confirming = true">{{ running && !run?.dry_run ? 'Syncing…' : 'Sync' }}</button>
      <span class="sy-note">Check says what a sync with the forges would do, and writes nothing. Sync pulls, syncs the forges, and pushes.</span>
    </div>
    <p v-if="startError" class="epfl-alert epfl-alert-danger" role="alert"><b>Not started.</b> {{ startError }}</p>

    <!-- the run in progress, or the last one: its steps as they came -->
    <section v-if="run" class="sy-report" aria-labelledby="sy-report">
      <div class="sy-report-head">
        <h3 id="sy-report" class="h5">{{ reportTitle }}</h3>
        <span class="gb-hint">{{ run.dry_run ? 'A check: nothing was written.' : running ? 'Changes you make meanwhile are saved as you make them. It goes on if you leave this page.' : '' }}</span>
      </div>
      <SyncProgress v-if="running" :run="run" />
      <p v-if="run.state === 'failed'" class="epfl-alert epfl-alert-danger" role="alert"><b>{{ run.dry_run ? 'The check' : 'The sync' }} stopped.</b> {{ run.error }}</p>
      <p v-else-if="run.report" class="gb-hint">{{ reportWords(run.report) }}</p>
      <details v-if="run.steps.length" class="sy-lines" :open="running">
        <summary>Steps ({{ run.steps.length }})</summary>
        <ol class="sy-steps">
          <li v-for="(s, i) in run.steps" :key="i" :class="s.state ? `sy-step-${s.state}` : ''">{{ stepWords(s) }}</li>
        </ol>
      </details>
    </section>

    <q-dialog v-model="confirming">
      <section class="gb-dialog" role="dialog" aria-modal="true" aria-labelledby="sy-confirm-title">
        <header class="gb-dialog-head">
          <h2 id="sy-confirm-title" class="h5">Sync now?</h2>
          <button type="button" class="gb-icon-btn" aria-label="Cancel" @click="confirming = false"><GbIcon name="close" /></button>
        </header>
        <div class="gb-dialog-body">
          <ul class="gb-plain-list">
            <li><span><b>git pull from {{ remote }}</b>{{ kept ? `: skipped, ${kept}` : '' }}</span></li>
            <li v-for="f in forges.filter((x) => x.sync)" :key="f.name"><span><b>{{ f.name }}</b>: {{ wayWords(f.way) }}</span></li>
            <li><span><b>git push to {{ remote }}</b>{{ kept ? `: skipped, ${kept}` : '' }}</span></li>
          </ul>
          <label class="sy-check"><input v-model="allowCreate" type="checkbox" /> Create issues on a forge never synced with, or 50 or more at once (a forge may email everyone watching it, once per issue)</label>
          <p class="gb-hint">A failed pull stops the sync; a forge that fails is told, and the push still runs. It goes on if you leave this page.</p>
        </div>
        <footer class="gb-dialog-foot">
          <span class="gb-grow" />
          <button type="button" class="epfl-btn epfl-btn-secondary" @click="confirming = false">Cancel</button>
          <button type="button" class="epfl-btn epfl-btn-primary" :disabled="starting" @click="runReal">Sync</button>
        </footer>
      </section>
    </q-dialog>

    <ForgeTokens :key="forges.map((f) => f.name).join(',')" @changed="reload" />

    <AddForgeDialog v-model="adding" :found="forges.filter((f) => f.discovered)" @added="reload" @guide="guideOpen = true" />
    <ForgesGuide v-model="guideOpen" />
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { message, patchConfig, remoteAction, startSync, type SyncRequest, type SyncStatus } from '@/api';
import { configModel, loadConfigModel, takeFile } from '@/configmodel';
import { keptIn, needsGit } from '@/datastore';
import { placesSummary } from '@/fieldplaces';
import { datastore, loadDatastore } from '@/kept';
import { onLive } from '@/live';
import { setForgeWay, type ForgeWay } from '@/api/sync';
import { loadSync, syncStatus } from '@/sync';
import { reportWords, remoteWords, stepWords } from '@/ways';
import AddForgeDialog from '@/components/AddForgeDialog.vue';
import ForgeTokens from '@/components/ForgeTokens.vue';
import ForgesGuide from '@/components/ForgesGuide.vue';
import GbIcon from '@/components/GbIcon.vue';
import SyncProgress from '@/components/SyncProgress.vue';

// changed: the sync as it stands, for the menu's line
const emit = defineEmits<{ changed: [st: SyncStatus] }>();

const remote = 'origin'; // the whole sync's, as git board sync's default

const route = useRoute();
const router = useRouter();
// "Add a forge" is open while the address says ?add=1; closing it drops it
const adding = computed({
  get: () => route.query.add === '1',
  set: (on) => void router.replace({ path: '/settings/sync', query: on ? { add: '1' } : {} }),
});
const guideOpen = ref(false);

const st = syncStatus;
const forges = computed(() => st.value?.forges ?? []);
const run = computed(() => st.value?.run ?? null);
const running = computed(() => run.value?.state === 'running');
const error = ref('');
const startError = ref('');
const busy = ref(''); // a forge being switched or removed
const rowBusy = ref(''); // "origin fetch"
const starting = ref(false);
const confirming = ref(false);
const confirmingRemove = ref('');
const allowCreate = ref(false);
const remoteLine = ref<Record<string, string>>({});

// a board kept elsewhere than git: what needs git is disabled, saying why
const kept = computed(() => keptIn(datastore.value));
const noGit = (what: string) => needsGit(datastore.value, what);
const kindName = (k: string) => ({ github: 'GitHub', gitlab: 'GitLab' })[k] ?? k;
// the forge a remote's URL names, when one does
const forgeOf = (r: string) => {
  const f = forges.value.find((x) => x.discovered);
  return r === remote && f ? `${kindName(f.kind)} · ${f.project}` : '';
};

const remoteActs = [
  { action: 'fetch', label: 'Fetch', command: 'git board fetch', needs: 'fetching the board from a remote' },
  { action: 'pull', label: 'Pull', command: 'git board pull', needs: 'git board pull' },
  { action: 'push', label: 'Push', command: 'git board push', needs: 'git board push' },
] as const;

const reportTitle = computed(() => {
  const r = run.value!;
  const what = r.scope === 'all' ? 'The sync' : r.dry_run ? 'The check' : 'The forges’ sync';
  if (r.state === 'running') return `${what}: so far`;
  return `${what} ${r.state === 'failed' ? 'stopped' : 'finished'} ${new Date(r.ended_at ?? r.started_at).toLocaleString()}`;
});

// where each field of a forge lives, from the file's model
function placesLine(forge: string): string {
  const m = configModel.value;
  const f = m?.forges.find((x) => x.name === forge);
  if (!m || !f) return '';
  return placesSummary(f, m.fields, m.field_order, Object.keys(m.roles ?? {}), m.github_project?.number ?? 0);
}

async function reload() {
  error.value = '';
  try {
    await Promise.all([loadSync(), loadConfigModel(), loadDatastore()]);
    if (st.value) emit('changed', st.value);
  } catch (e) {
    error.value = message(e);
  }
}

async function remoteDo(name: string, action: 'fetch' | 'pull' | 'push') {
  rowBusy.value = `${name} ${action}`;
  try {
    remoteLine.value = { ...remoteLine.value, [name]: remoteWords(await remoteAction(name, action)) };
  } catch (e) {
    remoteLine.value = { ...remoteLine.value, [name]: `${action} failed: ${message(e)}` };
  } finally {
    rowBusy.value = '';
  }
}

// a forge's table, written from the version the file was read at
async function writeForge(name: string, value: unknown) {
  busy.value = name;
  error.value = '';
  try {
    await loadConfigModel();
    takeFile(await patchConfig({ [`forges.${name}${value === null ? '' : '.sync'}`]: value }, configModel.value?.hash ?? ''));
    confirmingRemove.value = '';
    await loadSync();
  } catch (e) {
    error.value = message(e);
  } finally {
    busy.value = '';
  }
}
const toggle = (name: string, on: boolean) => writeForge(name, on);
const remove = (name: string) => writeForge(name, null);

// the way of a mirror, in words, and set: push only sends, read only takes
const wayWords = (way: string) => ({ push: 'push only, a backup', pull: 'read only', both: 'both ways' })[way] ?? way;

async function setWay(name: string, way: ForgeWay) {
  busy.value = name;
  error.value = '';
  try {
    await setForgeWay(name, way);
    await loadSync();
  } catch (e) {
    error.value = message(e);
  } finally {
    busy.value = '';
  }
}

async function start(r: SyncRequest) {
  starting.value = true;
  startError.value = '';
  try {
    if (st.value) st.value = { ...st.value, run: await startSync(r) };
  } catch (e) {
    startError.value = message(e);
  } finally {
    starting.value = false;
  }
}
async function runReal() {
  confirming.value = false;
  await start({ scope: 'all', remote, allow_create: allowCreate.value });
  allowCreate.value = false;
}

// a run started or ended, here or elsewhere: the status again
const off = onLive((e) => {
  if (e.kind === 'sync' || e.kind === 'config') void loadSync();
}, { own: true });
onBeforeUnmount(off);
onMounted(reload);
</script>

<style scoped>
.set-panel { display: flex; flex-direction: column; gap: 1.25rem; max-width: 64rem; }
.sy h2, .sy h3, .sy p { margin: 0; }
.sy-head { display: flex; flex-wrap: wrap; align-items: flex-end; justify-content: space-between; gap: 1rem; }
.sy-head h2 { margin-bottom: .375rem; }
.sy-lead { max-width: 44rem; color: var(--fg-muted); }
.sy-actions, .sy-run { display: flex; flex-wrap: wrap; align-items: center; gap: .75rem; }
.sy-note { font-size: .8125rem; color: var(--fg-muted); }
.sy-off { color: var(--fg-muted); }
.sy-kind { margin-left: .25rem; font-size: .8125rem; color: var(--fg-muted); }
.sy-wrap { overflow-wrap: anywhere; }
.sy-hidden { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
.gb-table-wrap { position: relative; }
.gb-table-wrap > .gb-table { min-width: 46rem; }
.sy-switch { display: inline-flex; align-items: center; gap: .5rem; cursor: pointer; font-weight: 700; }
.sy-switch input { width: 1.125rem; height: 1.125rem; accent-color: var(--primary-hover); }
.sy-way { max-width: 11rem; padding: .125rem .25rem; font: inherit; font-size: .8125rem; color: var(--fg); }
.sy-confirm-row td { background: var(--warning-bg); }
.sy-confirm-row p { margin: 0 0 .5rem; }
.sy-confirm-row .epfl-btn + .epfl-btn { margin-left: .5rem; }
.sy-report { display: grid; gap: 1rem; }
.sy-report-head { display: flex; flex-wrap: wrap; align-items: baseline; gap: .5rem 1.5rem; }
.sy-steps { display: grid; gap: .25rem; margin: .5rem 0 0; padding: 0; list-style: none; font-size: .875rem; }
.sy-step-failed { color: var(--danger-fg); font-weight: 700; }
.sy-step-skipped { color: var(--fg-muted); }
.sy-lines summary { cursor: pointer; }
.sy-check { display: flex; gap: .5rem; align-items: center; margin-top: .5rem; }
.sy-check input { accent-color: var(--primary-hover); }
.gb-linkbtn { padding: 0; border: 0; background: none; color: var(--fg); text-decoration: underline; cursor: pointer; font: inherit; }
.sy-acts { white-space: nowrap; }
.sy-acts .epfl-btn + .epfl-btn { margin-left: .25rem; }
</style>
