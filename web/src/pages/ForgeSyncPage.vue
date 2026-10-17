<!-- Where your issues go: three ways, each with its own buttons. Remotes
     carry everything with git (git bug pull, push); the forges and the
     GitHub Project are mirrors over their APIs. A forge run always starts
     from a Check (a dry run), and is confirmed with its counts, and with an
     explicit "I understand" when it creates many issues on a forge.
     Design: the settings canvas's "SyncChannels" artboard. -->
<template>
  <AppHeader :views="viewNames" active="" />
  <main class="gb-page gb-syncpage">
    <div>
      <h1 class="h3">Where your issues go</h1>
      <p class="gb-lead">Three ways, each with its own buttons. Remotes carry everything with git; forges and the Project are mirrors for the people who work there.</p>
    </div>

    <div class="gb-ways">
      <!-- 1: git remotes -->
      <section class="gb-way" aria-labelledby="way-remotes">
        <div class="epfl-eyebrow">1 · git</div>
        <div class="gb-way-head">
          <h2 id="way-remotes" class="h4">Remotes</h2>
          <a href="#/settings/remotes" class="gb-way-settings" aria-label="Remotes settings">Settings</a>
        </div>
        <p>Code, branches, issues, people.</p>
        <div v-if="remoteError" class="epfl-alert epfl-alert-danger" role="alert"><b>Not done.</b> {{ remoteError }}</div>
        <p v-if="remotes && !remotes.length" class="gb-note">No git remote. Add one with <code>git remote add origin &lt;url&gt;</code>.</p>
        <ul v-else-if="remotes" class="gb-way-list">
          <li v-for="r in remotes" :key="r.name" class="gb-way-box">
            <div class="gb-way-row"><b>{{ r.name }}</b><span class="gb-way-url">{{ r.url }}</span></div>
            <template v-if="r.tracked || r.checked">
              <div>{{ aheadWords(r.issues, 'issue') }}</div>
              <div v-if="r.people.send || r.people.receive">{{ aheadWords(r.people, 'person', 'people') }}</div>
            </template>
            <div v-else>Not checked yet</div>
            <div class="gb-note">{{ r.checked ? `Checked ${when(r.checked, true)}` : r.tracked ? 'As of the last fetch or push' : 'Check fetches from it and compares' }}</div>
            <div class="gb-way-actions">
              <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :aria-label="`Check ${r.name}`" :disabled="!!remoteBusy || running" @click="remote(r, 'fetch')">{{ remoteBusy === `${r.name} fetch` ? 'Checking…' : 'Check' }}</button>
              <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :aria-label="`Pull from ${r.name}`" :disabled="!!remoteBusy || running" @click="remote(r, 'pull')">{{ remoteBusy === `${r.name} pull` ? 'Pulling…' : 'Pull' }}</button>
              <button type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :aria-label="`Push to ${r.name}`" :disabled="!!remoteBusy || running" @click="remote(r, 'push')">{{ remoteBusy === `${r.name} push` ? 'Pushing…' : 'Push' }}</button>
            </div>
          </li>
        </ul>
        <p v-if="remoteDone" class="gb-note" role="status"><GbIcon name="check" small /> {{ remoteDone }}</p>
        <p class="gb-note">Pull and Push move issues and people, as <code>git bug pull</code> and <code>git bug push</code> do. Code goes with <code>git push</code>.</p>
      </section>

      <!-- 2: the forges' API -->
      <section class="gb-way gb-way-forges" aria-labelledby="way-forges">
        <div class="epfl-eyebrow">2 · their API</div>
        <div class="gb-way-head">
          <h2 id="way-forges" class="h4">Forges</h2>
          <a href="#/settings/forges" class="gb-way-settings" aria-label="Forges settings">Settings</a>
        </div>
        <p>A mirror of issues and comments.</p>
        <div v-if="forgeError" class="epfl-alert epfl-alert-danger" role="alert"><b>Not started.</b> {{ forgeError }}</div>
        <p v-if="status && !status.bridges.length" class="gb-note">No forge has sync on. Switch one on in <a href="#/settings/forges">Settings</a>, or add one.</p>
        <div v-else-if="status" class="gb-way-box">
          <div v-for="b in bridgeRows" :key="b.name" class="gb-way-row"><b>{{ b.name }}</b><span>{{ b.label }} · sync on</span></div>
          <div v-if="running">{{ job!.dry_run ? 'Checking…' : `${modeName(job!.mode)} running…` }}</div>
          <div v-else-if="job?.dry_run && job.state === 'done'">Last check {{ when(job.ended ?? job.started, true) }}: {{ checkWords(job.bridges) }}</div>
          <div v-else class="gb-note">Not checked yet</div>
        </div>
        <div class="gb-way-grid">
          <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="starting || running || !status?.bridges.length" @click="start(true)">{{ running && job?.dry_run ? 'Checking…' : 'Check' }}</button>
          <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="!canRun" @click="confirm('get')">Get</button>
          <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="!canRun" @click="confirm('send')">Send</button>
          <button type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="!canRun" @click="confirm('sync')">Sync</button>
        </div>
        <p class="gb-note">Check says what would come in and go out, and writes nothing. Get only brings the forges’ changes in, Send only sends yours, Sync does both.</p>
        <a href="#/settings/forges?add=1" class="gb-way-add"><GbIcon name="plus" small /> Add a forge</a>
      </section>

      <!-- 3: the GitHub Project's fields -->
      <section class="gb-way gb-way-project" aria-labelledby="way-project">
        <div class="epfl-eyebrow">3 · fields</div>
        <div class="gb-way-head">
          <h2 id="way-project" class="h4">GitHub Project</h2>
          <a href="#/settings/github-project" class="gb-way-settings" aria-label="GitHub Project settings">Settings</a>
        </div>
        <p>Status, priority, sprint, as labels.</p>
        <div v-if="projectError" class="epfl-alert epfl-alert-danger" role="alert"><b>Not done.</b> {{ projectError }}</div>
        <p v-if="project && !project.project" class="gb-note">No GitHub Project. Set one up with <code>git-board bridge configure</code>.</p>
        <template v-else-if="project">
          <div class="gb-way-box">
            <div class="gb-way-row"><b>{{ project.project }}</b><span>{{ project.write ? 'read and written' : 'read only' }}</span></div>
            <div v-if="project.last">{{ plural(project.last.conflicts.length, 'conflict') }}, checked {{ when(project.last.checked_at, true) }}</div>
            <div v-else>Not checked yet</div>
            <div v-if="project.last && pendingWords(project.last)" class="gb-note">A pull would {{ pendingWords(project.last) }}.</div>
          </div>
          <div class="gb-way-actions">
            <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="!!projectBusy || running" @click="projectRun(checkSync, 'Checking…')">{{ projectBusy === 'Checking…' ? projectBusy : 'Check' }}</button>
            <button type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="!!projectBusy || running" @click="pullFields">{{ projectBusy === 'Pulling…' ? projectBusy : project.write ? 'Sync fields' : 'Pull fields' }}</button>
            <router-link to="/conflicts" class="gb-way-link">Conflicts</router-link>
          </div>
          <p class="gb-note">{{ project.write ? 'Sync fields brings the Project’s values in as labels, and writes yours to the Project.' : 'Pull fields brings the Project’s values in as labels. Nothing is written to the Project.' }} Conflicts stay held.</p>
        </template>
      </section>
    </div>

    <!-- the forges' last run, per forge -->
    <section v-if="job && job.state !== 'failed'" aria-labelledby="counts">
      <div class="gb-run-head">
        <h2 id="counts" class="h5">{{ job.dry_run ? 'What a sync would do' : job.state === 'running' ? `${modeName(job.mode)}: so far` : `${modeName(job.mode)} finished ${when(job.ended ?? job.started, true)}` }}</h2>
        <span class="gb-hint">{{ job.dry_run ? `Check of ${when(job.started, true)}. Nothing was written.` : job.state === 'running' ? 'Edits wait: changes you make in git-board now are saved right after the run. It goes on if you leave this page.' : '' }}</span>
      </div>
      <p v-if="job.dry_run && job.state === 'done' && job.errors" class="epfl-alert epfl-alert-danger" role="alert">
        <b>The check had {{ plural(job.errors, 'error') }}</b> (see the details below). Fix {{ job.errors === 1 ? 'it' : 'them' }} and check again: a run needs a clean check.
      </p>
      <div class="gb-forges">
        <article v-for="b in bridgeRows" :key="b.name" class="gb-forge">
          <header class="gb-forge-head">
            <h3>{{ b.label }}</h3>
            <span class="gb-hint">bridge {{ b.name }}</span>
            <span class="gb-grow" />
            <span v-if="b.tally?.errors" class="gb-state gb-state-err"><GbIcon name="error" small /> {{ plural(b.tally.errors, 'error') }}</span>
            <span v-else-if="b.tally?.warnings" class="gb-hint">{{ plural(b.tally.warnings, 'warning') }}</span>
          </header>
          <div class="gb-forge-body">
            <p v-if="job.dry_run && (b.tally?.to.issues ?? 0) >= bigRun" class="epfl-alert epfl-alert-danger">
              <b>Creates {{ b.tally!.to.issues.toLocaleString() }} issues on {{ b.label }}.</b>
              {{ b.label }} may email everyone watching the project, once per issue. Ask watchers to mute it first, or leave {{ b.label }} out of the run.
            </p>
            <div class="gb-counts">
              <dl v-if="job.mode !== 'get'" class="gb-countlist">
                <dt class="gb-section-title"><GbIcon name="up" small /> To {{ b.label }}</dt>
                <div><dt>Issues {{ job.dry_run ? 'to create' : 'created' }}</dt><dd :class="{ 'gb-num-alert': (b.tally?.to.issues ?? 0) >= bigRun }">{{ n(b.tally?.to.issues) }}</dd></div>
                <div><dt>Linked issues with changes</dt><dd>{{ n(b.tally?.to.changed) }}</dd></div>
                <div><dt>Comments {{ job.dry_run ? 'to post' : 'posted' }}</dt><dd>{{ n(b.tally?.to.comments) }}</dd></div>
                <div><dt>Comment edits</dt><dd>{{ n(b.tally?.to.edits) }}</dd></div>
                <div><dt>Title, state and label changes</dt><dd>{{ n(b.tally?.to.changes) }}</dd></div>
              </dl>
              <dl v-if="job.mode !== 'send'" class="gb-countlist">
                <dt class="gb-section-title"><GbIcon name="down" small /> From {{ b.label }}</dt>
                <div><dt>Issues {{ job.dry_run ? 'to import' : 'imported' }}</dt><dd>{{ n(b.tally?.from.issues) }}</dd></div>
                <div><dt>Linked issues with changes</dt><dd>{{ n(b.tally?.from.changed) }}</dd></div>
                <div><dt>{{ job.dry_run ? 'Changes, up to' : 'Changes pulled' }}</dt><dd>{{ n((b.tally?.from.changes ?? 0) + (b.tally?.from.comments ?? 0) + (b.tally?.from.edits ?? 0)) }}</dd></div>
              </dl>
            </div>
            <p v-if="b.tally?.linked" class="gb-hint"><GbIcon name="link" small /> {{ plural(b.tally.linked, 'issue') }} found on {{ b.label }} by their git-bug marker: linked by a Get or a Sync, not created again.</p>
            <p v-if="b.tally?.stays_local" class="gb-note"><GbIcon name="local" small /> {{ plural(b.tally.stays_local, 'text') }} stay local: no {{ b.label }} token for their authors.</p>
            <details v-if="b.tally?.lines.length" class="gb-lines">
              <summary>Details ({{ b.tally.lines.length }})</summary>
              <pre>{{ b.tally.lines.join('\n') }}</pre>
            </details>
          </div>
        </article>
      </div>
      <p v-if="!job.dry_run && job.state === 'done'" class="gb-hint">The links it made are in the issues here: push them to a remote for other clones to know them.</p>
    </section>

    <section v-if="job?.state === 'failed'" class="epfl-alert epfl-alert-danger" role="alert">
      <b>{{ job.dry_run ? 'The check' : `The ${modeName(job.mode).toLowerCase()}` }} stopped.</b> {{ job.error }}
      Nothing already written was undone.
    </section>

    <q-dialog v-model="confirming">
      <section class="gb-dialog" role="dialog" aria-modal="true" aria-labelledby="confirm-title">
        <header class="gb-dialog-head">
          <h2 id="confirm-title" class="h5">{{ { sync: 'Sync with the forges?', get: 'Get the forges’ changes?', send: 'Send your changes to the forges?' }[mode] }}</h2>
          <button type="button" class="gb-icon-btn" aria-label="Cancel" @click="confirming = false"><GbIcon name="close" /></button>
        </header>
        <div class="gb-dialog-body">
          <p>From the check of {{ job ? when(job.started, true) : '' }}. If the forges changed since, the run does what is true then.</p>
          <fieldset class="gb-pick">
            <legend class="gb-hint">Forges in this run</legend>
            <label v-for="b in bridgeRows" :key="b.name"><input v-model="chosen" type="checkbox" :value="b.name" /> {{ b.label }}</label>
          </fieldset>
          <ul class="gb-plain-list">
            <li v-for="b in chosenRows" :key="b.name"><span><b>{{ b.label }}</b>: {{ b.tally ? runWords(mode, b.tally) : 'nothing to do' }}</span></li>
          </ul>
          <p v-if="mode === 'send'" class="epfl-alert epfl-alert-info">Send doesn’t get the forges’ changes first: where a title or a state changed on both sides, yours wins. Sync gets theirs first.</p>
          <p v-for="b in relinkRows" :key="`relink ${b.name}`" class="epfl-alert epfl-alert-danger">
            <b>{{ b.label }} has {{ plural(b.tally!.linked, 'issue') }} of this repository already,</b> found by their git-bug marker. Send alone would create them again: Get or Sync first, or leave {{ b.label }} out.
          </p>
          <div v-for="b in bigRows" :key="b.name" class="epfl-alert epfl-alert-danger">
            <b>This creates {{ b.tally!.to.issues.toLocaleString() }} issues on {{ b.label }}, which may email everyone watching it.</b>
            Issues can’t be deleted in bulk afterwards.
            <label class="gb-check"><input v-model="understood" type="checkbox" :value="b.name" /> I understand: create {{ b.tally!.to.issues.toLocaleString() }} issues on {{ b.label }}</label>
          </div>
          <p class="gb-hint">While it runs, edits in git-board wait and are saved right after it. It goes on if you leave this page.</p>
        </div>
        <footer class="gb-dialog-foot">
          <span class="gb-grow" />
          <button type="button" class="epfl-btn epfl-btn-secondary" @click="confirming = false">Cancel</button>
          <button type="button" class="epfl-btn epfl-btn-primary" :disabled="!chosen.length || relinkRows.length > 0 || bigRows.some((b) => !understood.includes(b.name)) || starting" @click="runReal">
            {{ modeName(mode) }}: {{ chosenRows.map((b) => b.label).join(', ') }}
          </button>
        </footer>
      </section>
    </q-dialog>
  </main>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { applySync, checkSync, getRemotes, getViews, remoteAction, startForgeSync, type ForgeMode, type Remote, type SyncStatus } from '@/api/board';
import { forgeStatus as status, loadForges } from '@/forges';
import { message } from '@/issue';
import { debounced, onLive } from '@/live';
import { when } from '@/me';
import { loadSyncStatus, syncStatus as project } from '@/sync';
import { aheadWords, checkWords, pendingWords, plural, pushWhat, runWords } from '@/ways';
import AppHeader from '@/components/AppHeader.vue';
import GbIcon from '@/components/GbIcon.vue';

// a run creating this many issues on a forge asks for an explicit "I understand"
const bigRun = 50;

const viewNames = ref<string[]>([]);

// 1: remotes
const remotes = ref<Remote[] | null>(null);
const remoteBusy = ref(''); // "origin push"
const remoteDone = ref('');
const remoteError = ref('');

async function loadRemotes() {
  try {
    remotes.value = (await getRemotes()).remotes;
  } catch (e) {
    remoteError.value = message(e);
  }
}

async function remote(r: Remote, action: 'fetch' | 'pull' | 'push') {
  if (action === 'push' && !window.confirm(`Push ${pushWhat(r)} to ${r.name} (${r.url})? Everyone who pulls from it gets them.`)) return;
  remoteBusy.value = `${r.name} ${action}`;
  remoteDone.value = remoteError.value = '';
  try {
    remotes.value = (await remoteAction(r.name, action)).remotes;
    remoteDone.value = { fetch: `Checked ${r.name}.`, pull: `Pulled from ${r.name}.`, push: `Pushed to ${r.name}.` }[action];
  } catch (e) {
    remoteError.value = message(e);
  } finally {
    remoteBusy.value = '';
  }
}

// 2: forges
const forgeError = ref('');
const starting = ref(false);
const confirming = ref(false);
const mode = ref<ForgeMode>('sync');
const chosen = ref<string[]>([]);
const understood = ref<string[]>([]);

const job = computed(() => status.value?.job ?? null);
const running = computed(() => job.value?.state === 'running');
// a run needs a clean check (the server also wants it fresh, 15 minutes)
const canRun = computed(() => !starting.value && !!job.value?.dry_run && job.value.state === 'done' && !job.value.errors);
const modeName = (m: ForgeMode) => ({ sync: 'Sync', get: 'Get', send: 'Send' })[m];

const forgeLabel = (target: string) => ({ github: 'GitHub', gitlab: 'GitLab', gitea: 'Gitea' })[target] ?? target;
const bridgeRows = computed(() =>
  (status.value?.bridges ?? []).map((b) => ({ name: b.name, label: forgeLabel(b.target), tally: job.value?.bridges[b.name] })),
);
const chosenRows = computed(() => bridgeRows.value.filter((b) => chosen.value.includes(b.name)));
const bigRows = computed(() => (mode.value === 'get' ? [] : chosenRows.value.filter((b) => (b.tally?.to.issues ?? 0) >= bigRun)));
// a send alone would create again what the check found by its marker
const relinkRows = computed(() => (mode.value === 'send' ? chosenRows.value.filter((b) => b.tally?.linked) : []));
const n = (v: number | undefined) => (v ?? 0).toLocaleString();

// a fresh check: every forge in, nothing understood yet
watch(() => job.value?.started, () => {
  chosen.value = bridgeRows.value.map((b) => b.name);
  understood.value = [];
});

function confirm(m: ForgeMode) {
  mode.value = m;
  confirming.value = true;
}

async function start(dryRun: boolean, bridges: string[] = []) {
  starting.value = true;
  forgeError.value = '';
  try {
    status.value = await startForgeSync(dryRun, mode.value, bridges);
    void loadForges(); // polls while it runs
  } catch (e) {
    forgeError.value = message(e);
  } finally {
    starting.value = false;
  }
}

async function runReal() {
  confirming.value = false;
  await start(false, chosen.value);
}

// 3: the GitHub Project
const projectBusy = ref('');
const projectError = ref('');

async function projectRun(action: () => Promise<SyncStatus>, doing: string) {
  projectBusy.value = doing;
  projectError.value = '';
  try {
    project.value = await action();
  } catch (e) {
    projectError.value = message(e);
  } finally {
    projectBusy.value = '';
  }
}

function pullFields() {
  const writes = project.value?.write ? ' and writes yours to the Project' : '';
  if (window.confirm(`This brings the Project’s values in as labels${writes}. Conflicts stay held. Go on?`)) {
    void projectRun(applySync, 'Pulling…');
  }
}

// what to send changes with every edit, and with a pull elsewhere
const reload = debounced(() => void loadRemotes());
const offLive = onLive((e) => (e.kind === 'issues' || e.kind === 'sync') && reload());
onBeforeUnmount(offLive);
// a run's end: the links it made are to push
watch(running, (now, was) => was && !now && reload());

onMounted(async () => {
  void loadForges();
  void loadSyncStatus();
  void loadRemotes();
  try {
    viewNames.value = Object.keys(await getViews()).sort();
  } catch {
    // the header shows no views
  }
});
</script>

<style scoped>
.gb-syncpage { display: grid; gap: var(--space-4); max-width: 90rem; }
.gb-syncpage h1 { margin: 0; }
.gb-lead { margin: var(--space-2) 0 0; color: var(--fg-muted); }

.gb-ways { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: var(--space-4); align-items: stretch; }
.gb-way { display: flex; flex-direction: column; gap: var(--space-3); min-width: 0; padding: var(--space-4); border: var(--border-w) solid var(--border); border-top: .25rem solid var(--fg); }
.gb-way .epfl-eyebrow { color: var(--fg-muted); }
.gb-way-forges { border-top-color: var(--primary); }
.gb-way-forges .epfl-eyebrow { color: var(--primary-hover); }
.gb-way-project { border-top-color: var(--gb-purple-fg); }
.gb-way-project .epfl-eyebrow { color: var(--gb-purple-fg); }
.gb-way > p, .gb-way .epfl-alert { margin: 0; }
.gb-way .epfl-alert { font-size: var(--fs-sm); overflow-wrap: anywhere; }
.gb-way-head { display: flex; align-items: baseline; justify-content: space-between; gap: var(--space-2); }
.gb-way-head h2 { margin: 0; }
.gb-way-settings, .gb-way-add { font-size: var(--fs-sm); }
.gb-way-add { display: inline-flex; align-items: center; gap: var(--space-1); align-self: flex-start; }
.gb-way-list { display: flex; flex-direction: column; gap: var(--space-2); margin: 0; padding: 0; list-style: none; }
.gb-way-box { display: flex; flex-direction: column; gap: var(--space-1); padding: var(--space-2) var(--space-3); background: var(--bg-subtle); font-size: var(--fs-sm); }
.gb-way-row { display: flex; flex-wrap: wrap; justify-content: space-between; gap: var(--space-1) var(--space-2); }
.gb-way-url { color: var(--fg-muted); overflow-wrap: anywhere; }
.gb-way-actions { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-2); margin-top: var(--space-1); }
.gb-way-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: var(--space-2); }
.gb-way .epfl-btn { min-height: 2.75rem; }
.gb-way-link { display: inline-flex; align-items: center; min-height: 2.75rem; padding: 0 var(--space-1); }

.gb-run-head { display: flex; flex-wrap: wrap; align-items: baseline; gap: var(--space-2) var(--space-4); margin-bottom: var(--space-3); }
.gb-run-head h2 { margin: 0; }
.gb-countlist div { display: flex; justify-content: space-between; gap: var(--space-3); padding: var(--space-2) 0; border-bottom: var(--border-w) solid var(--border-subtle); }
.gb-countlist dd { margin: 0; font-weight: var(--w-bold); font-variant-numeric: tabular-nums; }
.gb-countlist > dt { margin-bottom: var(--space-1); }
.gb-lines pre { max-height: 20rem; overflow: auto; padding: var(--space-3); background: var(--bg-subtle); font-size: var(--fs-xs); }
.gb-check { display: flex; gap: var(--space-2); align-items: center; margin-top: var(--space-2); }
.gb-check input, .gb-pick input { accent-color: var(--primary-hover); }
.gb-pick { display: flex; flex-wrap: wrap; gap: var(--space-4); margin: 0; padding: 0; border: 0; }

@media (max-width: 63.99rem) {
  .gb-ways { grid-template-columns: minmax(0, 1fr); }
}
@media (max-width: 47.99rem) {
  .gb-syncpage { padding: var(--space-3) var(--space-3) var(--space-5); }
}
</style>
