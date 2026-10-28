<!-- Settings › Sync: where your issues go, one table, a row per place: the
     git remote (origin), each forge, and the GitHub Project as a line under
     its GitHub row, each with its last check. Then Check and Sync, the whole
     sync (internal/server/whole.go): git pull, the forges, the Project, git
     push. Sync waits for a clean Check of the last 15 minutes, and is
     confirmed with its counts, with an explicit "I understand" when it
     creates issues on a forge never synced with, or 50 or more at once:
     the server holds them without it. Below, what a sync would do or did, per
     forge. #/settings/sync?add=1 opens "Add a forge", for one no remote
     names. The Project's line sets it up, as git board project configure
     does: a number or the Project's address, saved, then checked at once. -->
<template>
  <div class="set-panel sy">
    <div class="sy-head">
      <div>
        <h2 class="h4">Sync</h2>
        <p class="sy-lead">
          Your issues travel with git, like your code; the forges and the GitHub Project are mirrors, for the people who work there.
          One sync moves them all: a git pull from {{ remote }}, the forges, the Project's fields, then a git push. <code>git board sync</code> runs the same.
          Each row has its own actions, for its side alone: their tooltips name the command that does the same.
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
        <thead><tr><th>Where</th><th>Project</th><th>Sync</th><th>Last check</th><th>Actions</th><th><span class="sy-hidden">Remove</span></th></tr></thead>
        <tbody>
          <tr>
            <td><b>{{ remote }}</b> <span class="sy-kind">git</span></td>
            <td class="sy-wrap">
              <template v-if="st.remote">{{ st.remote.url }}<div v-if="st.remote.push_urls.join(' ') !== st.remote.url" class="sy-note">Pushes to {{ st.remote.push_urls.join(', ') }}</div></template>
              <template v-else>None: <code>git remote add {{ remote }} &lt;url&gt;</code>. The git steps are skipped.</template>
            </td>
            <td>{{ st.remote ? 'On' : '—' }}</td>
            <td>{{ remoteLine }}</td>
            <td class="sy-acts">
              <template v-if="st.remote">
                <button v-for="a in remoteActs" :key="a.action" type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :title="a.command" :disabled="!!rowBusy || running" @click="remoteDo(a.action)">
                  {{ rowBusy === a.action ? `${a.label}…` : a.label }}
                </button>
              </template>
            </td>
            <td />
          </tr>
          <template v-for="row in rows" :key="row.f?.name ?? 'project'">
            <template v-if="!row.f">
              <tr class="sy-sub">
                <td><span class="sy-kind">GitHub Project</span></td>
                <td class="sy-wrap">
                  <template v-if="st.project.project && !editingProject">
                    {{ projectName }}<button type="button" class="gb-linkbtn sy-small" :disabled="projectBusy || running" @click="changeProject">Change</button>
                    <div v-if="st.project.last?.title" class="sy-note">{{ st.project.last.title }}</div>
                  </template>
                  <form v-else class="sy-project" @submit.prevent="saveProject">
                    <label for="sy-project">Project number</label>
                    <input id="sy-project" v-model="projectInput" class="epfl-input" placeholder="40" autocomplete="off" data-1p-ignore />
                    <button type="submit" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="!projectDraft || projectBusy || running">Save</button>
                    <button v-if="st.project.project" type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" @click="editingProject = false">Cancel</button>
                    <p class="sy-note sy-project-help">
                      {{ projectHint }} In a terminal: <code>git board project configure --owner {{ projectDraft?.owner || githubOwner || '…' }} --project {{ projectDraft?.number ?? '…' }}</code>.
                      <button type="button" class="gb-linkbtn" @click="openProjectDoc">[github-project] in the configuration guide</button>
                    </p>
                  </form>
                  <div v-if="projectError" class="sy-err" role="alert">{{ projectError }}</div>
                </td>
                <td>{{ st.project.project ? (st.project.write ? 'Read and written' : 'Read only') : '—' }}</td>
                <td>{{ projectLine }} <router-link v-if="st.project.project" to="/conflicts">Conflicts</router-link></td>
                <td class="sy-acts">
                  <template v-if="st.project.project && !editingProject">
                    <button v-for="a in projectActs" :key="a.label" type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :title="a.command" :disabled="projectBusy || running" @click="projectDo(a.side)">{{ a.label }}</button>
                  </template>
                </td>
                <td><button v-if="st.project.project" type="button" class="gb-linkbtn" :disabled="projectBusy || running" @click="confirmingRemove = projectKey">Remove</button></td>
              </tr>
              <tr v-if="confirmingRemove === projectKey" class="sy-confirm-row">
                <td colspan="6">
                  <p role="alert"><b>Remove the GitHub Project {{ projectName }}?</b> The sync leaves it out from then on. The labels it set stay on the issues.</p>
                  <button type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="projectBusy" @click="removeProject">Remove the Project</button>
                  <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" @click="confirmingRemove = ''">Cancel</button>
                </td>
              </tr>
            </template>
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
                <td class="sy-acts">
                  <template v-if="row.f.sync">
                    <button
                      v-for="a in forgeActs" :key="a.mode" type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm"
                      :title="checked ? a.command : `Run Check first: ${a.command} follows a clean check of the last 15 minutes`"
                      :disabled="!checked || starting || running" @click="forgeDo(row.f.name, a.mode)"
                    >{{ a.label }}</button>
                  </template>
                </td>
                <td><button v-if="!row.f.discovered" type="button" class="gb-linkbtn" :disabled="busy === row.f.name || running" @click="confirmingRemove = row.f.name">Remove</button></td>
              </tr>
              <tr v-if="confirmingRemove === row.f.name" class="sy-confirm-row">
                <td colspan="6">
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
        <b>{{ job.dry_run ? 'The check' : 'The sync' }} stopped{{ failedAt ? ` at ${failedAt}` : '' }}.</b> {{ failedAt ? '' : `${job.error}.` }}{{ job.dry_run ? '' : ' Nothing already written was undone.' }}
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
          <div v-if="forgeStepOf?.state === 'skipped'" class="gb-forge-body">
            <p class="gb-hint">Skipped: {{ forgeStepOf.note }}.</p>
          </div>
          <div v-else class="gb-forge-body">
            <p v-if="b.tally?.left" class="epfl-alert epfl-alert-warning"><b>Left out of the run:</b> {{ b.tally.left }}</p>
            <p v-if="(b.tally?.held ?? 0) > 0" class="epfl-alert epfl-alert-danger">
              <b>{{ job.dry_run ? 'Would create' : 'Didn’t create' }} {{ b.tally!.held.toLocaleString() }} issues on {{ b.label }} without your consent</b>
              (the first sync with it, or 50 or more at once). {{ b.label }} may email everyone watching the project, once per issue: ask watchers to mute it first, or switch its sync off.
            </p>
            <div class="gb-counts">
              <dl v-if="job.mode !== 'get'" class="sy-countlist">
                <dt class="gb-section-title"><GbIcon name="up" small /> To {{ b.label }}</dt>
                <div><dt>Issues {{ job.dry_run ? 'to create' : 'created' }}</dt><dd :class="{ 'gb-num-alert': (b.tally?.held ?? 0) > 0 }">{{ n(b.tally?.to.issues) }}</dd></div>
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

    <StartOver @done="afterStartOver" />

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
            <b>This creates {{ b.tally!.held.toLocaleString() }} issues on {{ b.label }}, which may email everyone watching it.</b>
            Issues can’t be deleted in bulk afterwards. Without your consent, the sync creates none there and does the rest.
            <label class="sy-check"><input v-model="understood" type="checkbox" :value="b.name" /> I understand: create {{ b.tally!.held.toLocaleString() }} issues on {{ b.label }}</label>
          </div>
          <p class="gb-hint">A failed pull stops the sync; a forge or the Project that fails is told, and the push still runs. While it runs, edits in git-board wait and are saved right after it. It goes on if you leave this page.</p>
        </div>
        <footer class="gb-dialog-foot">
          <span class="gb-grow" />
          <button type="button" class="epfl-btn epfl-btn-secondary" @click="confirming = false">Cancel</button>
          <button type="button" class="epfl-btn epfl-btn-primary" :disabled="bigRows.some((b) => !understood.includes(b.name)) || starting" @click="runReal">Sync</button>
        </footer>
      </section>
    </q-dialog>

    <q-dialog v-model="projectDocOpen">
      <section class="gb-dialog" role="dialog" aria-modal="true" aria-labelledby="sy-doc-title">
        <header class="gb-dialog-head">
          <h2 id="sy-doc-title" class="h5">{{ projectDoc?.title ?? 'GitHub Project' }}</h2>
          <button type="button" class="gb-icon-btn" aria-label="Close" @click="projectDocOpen = false"><GbIcon name="close" /></button>
        </header>
        <div class="gb-dialog-body"><div v-if="projectDoc" class="gb-md sy-doc" v-html="projectDoc.html" /></div>
      </section>
    </q-dialog>

    <AddForgeDialog v-model="adding" @added="reload" @guide="guideOpen = true" />
    <ForgesGuide v-model="guideOpen" />
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { applySync, checkSync, getConfigDocs, getWhole, remoteAction, removeBridge, removeGithubProject, setBridgeSync, setGithubProject, startForgeSync, startWhole, type Bridge, type DocSection, type WholeStatus } from '@/api/board';
import { forgeStatus, loadForges, onStep } from '@/forges';
import { forgeLabel, kindName } from '@/issuesync';
import { message } from '@/issue';
import { debounced, onLive } from '@/live';
import { when } from '@/me';
import { loadRemotes } from '@/remotes';
import { aheadWords, checkWords, pendingWords, plural, readProject, runWords, stepLine } from '@/ways';
import AddForgeDialog from '@/components/AddForgeDialog.vue';
import ForgesGuide from '@/components/ForgesGuide.vue';
import GbIcon from '@/components/GbIcon.vue';
import StartOver from '@/components/StartOver.vue';
import SyncProgress from '@/components/SyncProgress.vue';

// changed: the sync as it stands, for the Overview's line
const emit = defineEmits<{ changed: [st: WholeStatus] }>();

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
const forgeStepOf = computed(() => job.value?.steps?.find((s) => s.name === 'forges'));
const forgeStep = computed(() => forgeStepOf.value?.state);
const pullStep = computed(() => job.value?.steps?.find((s) => s.name === 'pull'));
// a sync needs a clean check of the whole sync (the server also wants it fresh, 15 minutes)
const canRun = computed(() => !starting.value && job.value?.mode === 'all' && job.value.dry_run && job.value.state === 'done' && !job.value.errors);
const reportTitle = computed(() => {
  const j = job.value!;
  const what = { all: 'Sync', sync: 'The forges’ sync', send: 'Send', get: 'The forges’ pull' }[j.mode];
  if (j.dry_run) return j.state === 'running' ? 'Checking…' : 'What a sync would do';
  return j.state === 'running' ? `${what}: so far` : `${what} ${j.state === 'failed' ? 'stopped' : 'finished'} ${when(j.ended ?? j.started, true)}`;
});
// the step a run stopped at, in words; its error is on its line
const failedAt = computed(() => {
  const s = job.value?.steps?.find((x) => x.state === 'failed');
  return s ? stepLine({ ...s, note: '' }, remote, job.value!.dry_run).replace(/: failed$/, '') : '';
});
const stepIcon = { waiting: 'clock', running: 'sync', done: 'check', failed: 'error', skipped: 'forward' } as const;

// each side's actions, one side alone: their commands are their titles
const remoteActs = [
  { action: 'fetch', label: 'Fetch', command: 'git board fetch' },
  { action: 'pull', label: 'Pull', command: 'git board pull' },
  { action: 'push', label: 'Push', command: 'git board push' },
] as const;
const forgeActs = [
  { mode: 'get', label: 'Pull', command: 'git board forge pull' },
  { mode: 'send', label: 'Push', command: 'git board forge push' },
  { mode: 'sync', label: 'Sync', command: 'git board forge sync' },
] as const;
const projectActs = [
  { side: 'labels', label: 'Pull', command: 'git board project pull' },
  { side: 'project', label: 'Push', command: 'git board project push' },
  { side: undefined, label: 'Sync', command: 'git board project sync' },
] as const;
const rowBusy = ref(''); // the origin row's action, running
// a forge's own run follows a clean check, as Sync does (the server also
// wants it fresh, 15 minutes)
const checked = computed(() => !!job.value?.dry_run && job.value.state === 'done' && !job.value.errors);

const bridgeRows = computed(() => (job.value?.names ?? []).map((name) => {
  const f = st.value?.forges.find((x) => x.name === name);
  return { name, label: forgeLabel({ name, target: f?.target ?? '' }), tally: job.value?.bridges[name] };
}));
// the forges whose creations wait for consent (the server holds them: the
// first sync with a forge, or 50 or more at once): an explicit "I understand"
const bigRows = computed(() => bridgeRows.value.filter((b) => (b.tally?.held ?? 0) > 0));
const n = (v: number | undefined) => (v ?? 0).toLocaleString();

// the forges' rows, the Project's line under the first GitHub row, else last
const rows = computed(() => {
  const forges: { f?: Bridge }[] = (st.value?.forges ?? []).map((f) => ({ f }));
  const github = forges.findIndex((r) => r.f?.target === 'github');
  forges.splice(github < 0 ? forges.length : github + 1, 0, {});
  return forges;
});

// a step's note, short: its first clause
const short = (note: string) => (note.split(/[:;]/)[0] ?? '').trim();

const remoteLine = computed(() => {
  const r = st.value?.remote;
  if (!r) return '—';
  const pull = pullStep.value;
  if (pull?.state === 'failed') return `Failed ${when(job.value!.ended ?? job.value!.started, true)}: ${short(pull.note ?? '')}`;
  if (r.checked) return `${aheadWords(r.issues, 'issue')}, checked ${when(r.checked, true)}`;
  return r.tracked ? `${aheadWords(r.issues, 'issue')}, as of the last fetch or push` : 'Not checked yet';
});

function forgeLine(f: Bridge): string {
  if (!f.sync) return 'Off: left out of the sync';
  const j = job.value;
  const t = j?.bridges[f.name];
  if (!j || !t || j.state === 'running') return j?.state === 'running' && t ? 'Running…' : 'Not checked yet';
  if (forgeStep.value === 'skipped') return `Skipped ${when(j.ended ?? j.started, true)}: ${forgeStepOf.value?.note ?? ''}`;
  if (t.left) return `Left out ${when(j.ended ?? j.started, true)}: ${short(t.left)}`;
  if (j.state === 'failed' && forgeStep.value === 'failed') return `Failed ${when(j.ended ?? j.started, true)}`;
  return j.dry_run ? `${checkWords({ [f.name]: t })}, checked ${when(j.ended ?? j.started, true)}` : `Synced ${when(j.ended ?? j.started, true)}`;
}

const projectLine = computed(() => {
  const p = st.value?.project;
  if (!p?.project) return '—';
  if (!p.last) return projectBusy.value ? 'Checking…' : checkFailed.value ? `Failed ${when(checkFailed.value, true)}.` : 'Not checked yet.';
  const pending = pendingWords(p.last);
  return `${plural(p.last.conflicts.length, 'conflict')}, checked ${when(p.last.checked_at, true)}${pending ? `; a sync would ${pending}` : ''}.`;
});

// the Project's line: its field, open while none is set or on Change; a
// bare number is a Project of the GitHub row's owner, EPFL-ENAC of
// EPFL-ENAC/wiki
const projectKey = '[github-project]'; // confirmingRemove's, no forge's name
const editingProject = ref(false);
const projectInput = ref('');
const projectBusy = ref(false);
const projectError = ref('');
const checkFailed = ref(''); // when the check after a save failed
const projectDocOpen = ref(false);
const projectDoc = ref<DocSection | null>(null);
const githubOwner = computed(() => st.value?.forges.find((f) => f.target === 'github')?.project.split('/')[0] ?? '');
const projectDraft = computed(() => readProject(projectInput.value, githubOwner.value));
const projectName = computed(() => st.value?.project.project.replace('/', ' #') ?? '');
const projectHint = computed(() => {
  if (!githubOwner.value) return 'No GitHub forge to take the owner from: paste the Project’s address.';
  return projectInput.value.trim() && !projectDraft.value ? 'A number, or the Project’s address on GitHub.' : '';
});

function changeProject() {
  const [owner, number] = (st.value?.project.project ?? '').split('/');
  projectInput.value = owner === githubOwner.value ? (number ?? '') : '';
  editingProject.value = true;
}

async function saveProject() {
  const p = projectDraft.value;
  if (!p || !st.value) return;
  projectBusy.value = true;
  projectError.value = checkFailed.value = '';
  try {
    st.value.project = await setGithubProject(p.owner, p.number);
    editingProject.value = false;
    projectInput.value = '';
    // checked at once: the Project answers with its title, or why not
    await checkSync().catch((e) => {
      checkFailed.value = new Date().toISOString();
      projectError.value = `${projectName.value} didn’t answer: ${message(e)}`;
    });
    await reload();
  } catch (e) {
    projectError.value = `Not saved: ${message(e)}`;
  } finally {
    projectBusy.value = false;
  }
}

async function removeProject() {
  projectBusy.value = true;
  projectError.value = '';
  try {
    await removeGithubProject();
    confirmingRemove.value = '';
    await reload();
  } catch (e) {
    projectError.value = `Not removed: ${message(e)}`;
  } finally {
    projectBusy.value = false;
  }
}

async function openProjectDoc() {
  projectDocOpen.value = true;
  try {
    projectDoc.value ??= (await getConfigDocs()).find((d) => d.id === 'github') ?? null;
  } catch (e) {
    error.value = message(e);
  }
}

async function reload() {
  try {
    st.value = await getWhole();
    error.value = '';
    emit('changed', st.value);
  } catch (e) {
    error.value = message(e);
  }
}

async function start(dryRun: boolean, allowCreate = false) {
  starting.value = true;
  startError.value = '';
  try {
    forgeStatus.value = await startWhole(dryRun, allowCreate);
    void loadForges(); // polls while it runs
  } catch (e) {
    startError.value = message(e);
  } finally {
    starting.value = false;
  }
}

async function runReal() {
  confirming.value = false;
  await start(false, bigRows.value.length > 0 && bigRows.value.every((b) => understood.value.includes(b.name)));
}

async function remoteDo(action: 'fetch' | 'pull' | 'push') {
  rowBusy.value = action;
  error.value = '';
  try {
    await remoteAction(remote, action);
    await reload();
    void loadRemotes(); // the header's counts
  } catch (e) {
    error.value = `git board ${action}: ${message(e)}`;
  } finally {
    rowBusy.value = '';
  }
}
async function forgeDo(name: string, mode: 'get' | 'send' | 'sync') {
  starting.value = true;
  startError.value = '';
  try {
    forgeStatus.value = await startForgeSync(false, mode, [name]);
    void loadForges(); // polls while it runs
  } catch (e) {
    startError.value = message(e);
  } finally {
    starting.value = false;
  }
}
async function projectDo(side?: 'labels' | 'project') {
  projectBusy.value = true;
  projectError.value = '';
  try {
    await applySync(side);
    await reload();
  } catch (e) {
    projectError.value = message(e);
  } finally {
    projectBusy.value = false;
  }
}
// a board started over: every row's line, the header's counts, the job
async function afterStartOver() {
  await reload();
  void loadRemotes();
  void loadForges();
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
.gb-linkbtn.sy-small { margin-left: .5rem; font-size: .8125rem; }
.sy-project { display: flex; flex-wrap: wrap; align-items: center; gap: .5rem; }
.sy-project .epfl-input { flex: 0 1 16rem; min-width: 0; }
.sy-project-help { flex-basis: 100%; }
.sy-project-help .gb-linkbtn { color: inherit; }
.sy-doc :deep(th), .sy-doc :deep(td code) { white-space: nowrap; }
.sy-err { margin-top: .25rem; font-size: .8125rem; color: var(--danger-fg); }
.sy-acts { white-space: nowrap; }
.sy-acts .epfl-btn + .epfl-btn { margin-left: .25rem; }
</style>
