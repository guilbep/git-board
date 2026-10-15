<!-- Sync with GitHub/GitLab: git-bug's forge sync, run by the server.
     Always a dry run first; the real run is confirmed with its counts, and
     with an explicit "I understand" when it creates many issues on a forge.
     Design: the canvas's "Forge sync" and "Sync run" artboards. -->
<template>
  <AppHeader :views="viewNames" active="" />
  <main class="gb-page gb-syncpage">
    <div class="epfl-eyebrow">Sync with GitHub/GitLab</div>
    <h1 class="h3">Sync with GitHub/GitLab</h1>
    <p class="gb-lead">
      Mirrors issues, comments, titles, open/closed and labels between git-board and the forges, both ways.
      Every run starts with a dry run: nothing is written until you confirm.
    </p>
    <p class="gb-hint">Project fields (status, prio, sprint…) have their own sync, <router-link to="/conflicts">Sync with the Project</router-link><!-- -->.</p>

    <div v-if="status && !status.bridges.length" class="epfl-alert epfl-alert-warning">
      No bridge has sync enabled. Set one up with <code>git bug bridge new --name gh --url https://github.com/owner/repo --sync</code>.
    </div>

    <template v-else-if="status">
      <ol class="gb-steps" aria-label="Steps">
        <li v-for="(s, i) in steps" :key="s" class="gb-step" :class="{ 'gb-step-done': i < step }" :aria-current="i === step ? 'step' : undefined">
          <span class="gb-step-n">{{ i < step ? '✓' : i + 1 }}</span>{{ s }}
        </li>
      </ol>

      <div v-if="error" class="epfl-alert epfl-alert-danger" role="alert"><b>Not started.</b> {{ error }}</div>

      <!-- 1: nothing to review yet -->
      <p v-if="job?.dry_run && job.state === 'done' && job.errors" class="epfl-alert epfl-alert-danger" role="alert">
        <b>The dry run had {{ job.errors }} error{{ job.errors === 1 ? '' : 's' }}</b> (see the details below). Fix {{ job.errors === 1 ? 'it' : 'them' }} and dry run again: a real run needs a clean dry run.
      </p>
      <section v-if="step === 0" class="gb-run">
        <p v-if="job && !job.dry_run && job.state !== 'running'" class="gb-hint">Last run {{ when(job.started, true) }}.</p>
        <button type="button" class="epfl-btn epfl-btn-primary" :disabled="starting" @click="start(true)">
          <GbIcon name="sync" small /> {{ starting ? 'Starting…' : 'Dry run' }}
        </button>
        <p class="gb-hint">Reads both forges and says what a sync would do. Writes nothing.</p>
      </section>

      <!-- running, dry or real -->
      <section v-if="job?.state === 'running'" class="gb-run" role="status">
        <h2 class="h5"><GbIcon name="sync" small /> {{ job.dry_run ? 'Dry run' : 'Sync' }} running…</h2>
        <p class="gb-hint">Started {{ when(job.started, true) }}. It goes on if you leave this page.</p>
        <p v-if="!job.dry_run" class="epfl-alert epfl-alert-info"><b>Edits wait.</b> Changes you make in git-board now are saved right after the run.</p>
      </section>

      <!-- counts, per forge -->
      <section v-if="job && job.state !== 'failed'" aria-labelledby="counts">
        <div class="gb-run-head">
          <h2 id="counts" class="h5">{{ job.dry_run ? 'What the run would do' : job.state === 'running' ? 'So far' : `Sync finished ${when(job.ended ?? job.started, true)}` }}</h2>
          <span class="gb-hint">{{ job.dry_run ? `Dry run of ${when(job.started, true)}. Nothing was written.` : '' }}</span>
          <button v-if="step === 1" type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="starting" @click="start(true)"><GbIcon name="sync" small /> Dry run again</button>
        </div>
        <div class="gb-forges">
          <article v-for="b in bridgeRows" :key="b.name" class="gb-forge">
            <header class="gb-forge-head">
              <h3>{{ b.label }}</h3>
              <span class="gb-hint">bridge {{ b.name }}</span>
              <span class="gb-grow" />
              <span v-if="b.tally?.errors" class="gb-state gb-state-err"><GbIcon name="error" small /> {{ b.tally.errors }} error{{ b.tally.errors === 1 ? '' : 's' }}</span>
              <span v-else-if="b.tally?.warnings" class="gb-hint">{{ b.tally.warnings }} warning{{ b.tally.warnings === 1 ? '' : 's' }}</span>
            </header>
            <div class="gb-forge-body">
              <p v-if="job.dry_run && (b.tally?.to.issues ?? 0) >= bigRun" class="epfl-alert epfl-alert-danger">
                <b>Creates {{ b.tally!.to.issues.toLocaleString() }} issues on {{ b.label }}.</b>
                {{ b.label }} may email everyone watching the project, once per issue. Ask watchers to mute it first, or leave {{ b.label }} out of this run.
              </p>
              <div class="gb-counts">
                <dl class="gb-countlist">
                  <dt class="gb-section-title"><GbIcon name="up" small /> To {{ b.label }}</dt>
                  <div><dt>Issues {{ job.dry_run ? 'to create' : 'created' }}</dt><dd :class="{ 'gb-num-alert': (b.tally?.to.issues ?? 0) >= bigRun }">{{ n(b.tally?.to.issues) }}</dd></div>
                  <div><dt>Linked issues with changes</dt><dd>{{ n(b.tally?.to.changed) }}</dd></div>
                  <div><dt>Comments {{ job.dry_run ? 'to post' : 'posted' }}</dt><dd>{{ n(b.tally?.to.comments) }}</dd></div>
                  <div><dt>Comment edits</dt><dd>{{ n(b.tally?.to.edits) }}</dd></div>
                  <div><dt>Title, state and label changes</dt><dd>{{ n(b.tally?.to.changes) }}</dd></div>
                </dl>
                <dl class="gb-countlist">
                  <dt class="gb-section-title"><GbIcon name="down" small /> From {{ b.label }}</dt>
                  <div><dt>Issues {{ job.dry_run ? 'to import' : 'imported' }}</dt><dd>{{ n(b.tally?.from.issues) }}</dd></div>
                  <div><dt>Linked issues with changes</dt><dd>{{ n(b.tally?.from.changed) }}</dd></div>
                  <div><dt>{{ job.dry_run ? 'Changes, up to' : 'Changes pulled' }}</dt><dd>{{ n((b.tally?.from.changes ?? 0) + (b.tally?.from.comments ?? 0) + (b.tally?.from.edits ?? 0)) }}</dd></div>
                </dl>
              </div>
              <p v-if="b.tally?.linked" class="gb-hint"><GbIcon name="link" small /> {{ b.tally.linked }} issue{{ b.tally.linked === 1 ? '' : 's' }} found on {{ b.label }} by their git-bug marker: synced, not created again.</p>
              <p v-if="b.tally?.stays_local" class="gb-note"><GbIcon name="local" small /> {{ b.tally.stays_local }} text{{ b.tally.stays_local === 1 ? '' : 's' }} stay local: no {{ b.label }} token for their authors.</p>
              <details v-if="b.tally?.lines.length" class="gb-lines">
                <summary>Details ({{ b.tally.lines.length }})</summary>
                <pre>{{ b.tally.lines.join('\n') }}</pre>
              </details>
            </div>
          </article>
        </div>
        <p v-if="job.pushed" class="gb-hint">git-bug data: {{ job.pushed }}</p>
      </section>

      <section v-if="job?.state === 'failed'" class="epfl-alert epfl-alert-danger" role="alert">
        <b>{{ job.dry_run ? 'The dry run' : 'The sync' }} stopped.</b> {{ job.error }}
        Nothing already written was undone.
      </section>

      <!-- 2: confirm -->
      <section v-if="step === 1" class="gb-runit">
        <h2 class="h5">Run it</h2>
        <fieldset class="gb-pick">
          <legend class="gb-hint">Forges in this run</legend>
          <label v-for="b in bridgeRows" :key="b.name"><input v-model="chosen" type="checkbox" :value="b.name" /> {{ b.label }}</label>
        </fieldset>
        <label v-if="status?.origin.length" class="gb-check">
          <input v-model="push" type="checkbox" />
          <span>Then push the git-bug data to <code>origin</code> ({{ status.origin.join(', ') }}), for other clones to know the links</span>
        </label>
        <p class="gb-hint">While it runs, edits in git-board wait and are saved right after it. It goes on if you leave this page.</p>
        <button type="button" class="epfl-btn epfl-btn-primary" :disabled="!chosen.length" @click="confirming = true">Run the sync…</button>
      </section>
    </template>
    <p v-else class="gb-hint">Loading…</p>

    <q-dialog v-model="confirming">
      <section class="gb-dialog" role="dialog" aria-modal="true" aria-labelledby="confirm-title">
        <header class="gb-dialog-head">
          <h2 id="confirm-title" class="h5">Run the sync with GitHub/GitLab?</h2>
          <button type="button" class="gb-icon-btn" aria-label="Cancel" @click="confirming = false"><GbIcon name="close" /></button>
        </header>
        <div class="gb-dialog-body">
          <p>From the dry run of {{ job ? when(job.started, true) : '' }}. If the forges changed since, the run does what is true then.</p>
          <ul class="gb-plain-list">
            <li v-for="b in chosenRows" :key="b.name">
              <b>{{ b.label }}</b>: create {{ count(b.tally?.to.issues, 'issue') }}, post {{ count(b.tally?.to.comments, 'comment') }},
              push {{ count((b.tally?.to.edits ?? 0) + (b.tally?.to.changes ?? 0), 'edit or change', 'edits and changes') }}, pull {{ count((b.tally?.from.issues ?? 0) + (b.tally?.from.changed ?? 0), 'issue') }}
            </li>
          </ul>
          <div v-for="b in bigRows" :key="b.name" class="epfl-alert epfl-alert-danger">
            <b>This creates {{ b.tally!.to.issues.toLocaleString() }} issues on {{ b.label }}, which may email everyone watching it.</b>
            Issues can’t be deleted in bulk afterwards.
            <label class="gb-check"><input v-model="understood" type="checkbox" :value="b.name" /> I understand: create {{ b.tally!.to.issues.toLocaleString() }} issues on {{ b.label }}</label>
          </div>
        </div>
        <footer class="gb-dialog-foot">
          <span class="gb-grow" />
          <button type="button" class="epfl-btn epfl-btn-secondary" @click="confirming = false">Cancel</button>
          <button type="button" class="epfl-btn epfl-btn-primary" :disabled="bigRows.some((b) => !understood.includes(b.name)) || starting" @click="runReal">
            Run: {{ count(chosenRows.reduce((s, b) => s + (b.tally?.to.issues ?? 0), 0), 'issue') }}, {{ count(chosenRows.reduce((s, b) => s + (b.tally?.to.comments ?? 0), 0), 'comment') }}
          </button>
        </footer>
      </section>
    </q-dialog>
  </main>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { getViews, startForgeSync } from '@/api/board';
import { forgeStatus as status, loadForges } from '@/forges';
import { message } from '@/issue';
import { when } from '@/me';
import AppHeader from '@/components/AppHeader.vue';
import GbIcon from '@/components/GbIcon.vue';

// a run creating this many issues on a forge asks for an explicit "I understand"
const bigRun = 50;
const steps = ['Dry run', 'Review and confirm', 'Run', 'Results'];

const viewNames = ref<string[]>([]);
const error = ref('');
const starting = ref(false);
const confirming = ref(false);
const chosen = ref<string[]>([]);
const understood = ref<string[]>([]);
const push = ref(false); // an outward write: the user's choice, each run

const job = computed(() => status.value?.job ?? null);
// where we are: 0 dry run to do, 1 review, 2 running, 3 results
const step = computed(() => {
  const j = job.value;
  if (!j || j.state === 'failed') return 0;
  // a dry run with errors: fix them, run it again (the server refuses a real run)
  if (j.dry_run) return j.state === 'running' || j.errors > 0 ? 0 : 1;
  return j.state === 'running' ? 2 : 3;
});

const forgeLabel = (target: string) => ({ github: 'GitHub', gitlab: 'GitLab', gitea: 'Gitea' })[target] ?? target;
const bridgeRows = computed(() =>
  (status.value?.bridges ?? []).map((b) => ({ name: b.name, label: forgeLabel(b.target), tally: job.value?.bridges[b.name] })),
);
const chosenRows = computed(() => bridgeRows.value.filter((b) => chosen.value.includes(b.name)));
const bigRows = computed(() => chosenRows.value.filter((b) => (b.tally?.to.issues ?? 0) >= bigRun));
const n = (v: number | undefined) => (v ?? 0).toLocaleString();
const count = (v: number | undefined, one: string, many = `${one}s`) => `${n(v)} ${(v ?? 0) === 1 ? one : many}`;

// a fresh dry run: every forge in, nothing understood yet
watch(() => job.value?.started, () => {
  chosen.value = bridgeRows.value.map((b) => b.name);
  understood.value = [];
});

async function start(dryRun: boolean, bridges: string[] = []) {
  starting.value = true;
  error.value = '';
  try {
    status.value = await startForgeSync(dryRun, push.value, bridges);
    void loadForges(); // polls while it runs
  } catch (e) {
    error.value = message(e);
  } finally {
    starting.value = false;
  }
}

async function runReal() {
  confirming.value = false;
  await start(false, chosen.value);
}

onMounted(async () => {
  void loadForges();
  try {
    viewNames.value = Object.keys(await getViews()).sort();
  } catch {
    // the header shows no views
  }
});
</script>

<style scoped>
.gb-syncpage { display: grid; gap: 1.25rem; max-width: 90rem; }
.gb-syncpage h1 { margin: 0; }
.gb-lead { max-width: 46rem; margin: 0; font-size: 1.125rem; }
.gb-run, .gb-runit { display: grid; gap: .75rem; justify-items: start; padding: 1.5rem; background: var(--bg-subtle); border: 1px solid var(--border); }
.gb-run h2, .gb-runit h2 { margin: 0; display: flex; gap: .5rem; align-items: center; }
.gb-run-head { display: flex; flex-wrap: wrap; align-items: baseline; gap: .5rem 1.25rem; margin-bottom: 1rem; }
.gb-run-head h2 { margin: 0; }
.gb-run-head .epfl-btn { margin-left: auto; }
.gb-countlist div { display: flex; justify-content: space-between; gap: 1rem; padding: .5rem 0; border-bottom: 1px solid var(--border-subtle); }
.gb-countlist dd { margin: 0; font-weight: 700; font-variant-numeric: tabular-nums; }
.gb-countlist > dt { margin-bottom: .25rem; }
.gb-lines pre { max-height: 20rem; overflow: auto; padding: .75rem; background: var(--bg-subtle); font-size: .75rem; }
.gb-check { display: flex; gap: .5rem; align-items: center; margin-top: .5rem; }
.gb-check input, .gb-pick input { accent-color: var(--epfl-red-dark); }
.gb-pick { display: flex; gap: 1.25rem; margin: 0; padding: 0; border: 0; }
</style>
