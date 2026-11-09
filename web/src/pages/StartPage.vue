<!-- The Start page: a new board's first run (docs/design/first-run/
     proposal.md). It reads the board's state (GET /api/state) and walks
     five steps: who you are, what you want, log in, check the first run,
     run it. Each step writes one decision, through the routes the Settings
     pages use, and the page never shows a terminal command. The board
     opens here while it is new and empty, until you skip or finish. -->
<template>
  <AppHeader :views="viewNames" />
  <main class="gb-page gb-start">
    <div class="epfl-eyebrow">Start</div>
    <h1 class="h3">Set up this board</h1>
    <p class="gb-lead">Five steps, each one optional. You can change every choice later in Settings.</p>
    <p v-if="error" class="epfl-alert epfl-alert-danger" role="alert">{{ error }}</p>

    <ol v-if="st" class="gb-start-steps">
      <!-- 1. who you are -->
      <li class="gb-start-step" :class="{ done: st.me.state === 'chosen' }">
        <h2 class="h5">1. Who you are</h2>
        <p v-if="st.me.state === 'chosen'" class="gb-hint" data-test="me-done">You write as <b>{{ myName }}</b>.</p>
        <IdentityChooser v-else />
      </li>

      <!-- 2. what you want -->
      <li class="gb-start-step" :class="{ done: !!goal && goalDone }">
        <h2 class="h5">2. What you want</h2>
        <div class="gb-opts gb-start-goals" role="radiogroup" aria-label="What you want">
          <label v-for="g in goals" :key="g.id" class="gb-opt" :class="{ on: goal === g.id }">
            <input v-model="goal" type="radio" name="start-goal" :value="g.id" />
            <span><b>{{ g.name }}</b><br /><span class="gb-hint">{{ g.hint }}</span></span>
          </label>
        </div>

        <div v-if="goal" class="gb-start-goal" data-test="goal-panel">
          <template v-if="goal === 'see' || goal === 'both' || goal === 'backup'">
            <p v-for="f in tableForges" :key="f.name" class="gb-start-row">
              <span><b>{{ f.name }}</b> · {{ f.project }}: {{ wayWords(f.way) }}</span>
              <button v-if="goal !== 'backup' && f.way !== wantWay" type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="busy" @click="setWay(f.name, wantWay)">
                Make it {{ wayWords(wantWay) }}
              </button>
            </p>
            <p v-for="f in foundForges" :key="f.name" class="gb-hint">
              Found in your git remotes: <b>{{ f.project }}</b>, synced both ways.
              <template v-if="goal === 'see'">To make it read-only, add it as a forge with the way Read only.</template>
            </p>
            <p v-if="goalDone" class="gb-hint" data-test="goal-done">Done: {{ goalDoneWords }}</p>
            <button v-else type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" @click="adding = true">
              {{ goal === 'backup' ? 'Add the backup forge' : 'Add the forge' }}
            </button>
            <AddForgeDialog v-model="adding" :found="foundAsSync" :way="wantWay" @added="reload" />
          </template>
          <p v-else-if="goal === 'markdown'">
            The issues become Markdown files in a folder of the repository, which you edit with any text editor.
            <router-link to="/settings/datastore">Open Settings › Datastore</router-link>: copy the board into a folder, then keep it there.
            For a copy once, <a :href="markdownExportURL()" download data-test="md-export">download the issues as Markdown</a>.
          </p>
          <p v-else-if="goal === 'project'">
            The fields live in the columns of a GitHub Project.
            <router-link to="/settings/project">Open Settings › Project</router-link> and name it: its owner and number.
          </p>
        </div>

        <p v-if="st.file.state === 'none'" class="gb-hint gb-start-file">
          This board has no project file yet, and the defaults hold. The settings you change write <code>.git-board.toml</code>: commit it to share it with your team.
          <button type="button" class="gb-linkbtn" :disabled="busy" @click="writeFile">Write the default file now</button>
        </p>
      </li>

      <!-- 3. log in -->
      <li class="gb-start-step" :class="{ done: !needToken && st.forges.length > 0 }">
        <h2 class="h5">3. Log in</h2>
        <p v-if="!st.forges.length" class="gb-hint">No forge yet: step 2 adds one.</p>
        <p v-else-if="!needToken" class="gb-hint" data-test="tokens-done">A token is found for every forge.</p>
        <ForgeTokens v-else @changed="reload" />
      </li>

      <!-- 4. check the first run, 5. run it -->
      <li class="gb-start-step" :class="{ done: ranAll }">
        <h2 class="h5">4. Check the first run</h2>
        <p v-if="!syncable.length" class="gb-hint">Nothing to run yet: a forge with a token comes first.</p>
        <template v-else>
          <p class="gb-hint">A check reads the forges and writes nothing: it says what the run will do.</p>
          <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="running" data-test="check" @click="go(true)">{{ running && run?.dry_run ? 'Checking…' : 'Check' }}</button>
          <p v-if="checked" class="gb-start-report" role="status" data-test="check-report">The check: {{ reportWords(checked.report) || 'nothing to change' }}.</p>
        </template>
      </li>
      <li class="gb-start-step" :class="{ done: ranAll }">
        <h2 class="h5">5. Run it</h2>
        <template v-if="syncable.length">
          <label v-if="creates" class="af-check">
            <input v-model="allowCreate" type="checkbox" />
            <span>Create the board's issues on the forges that do not have them yet</span>
          </label>
          <button type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="running" data-test="run" @click="go(false)">{{ running && !run?.dry_run ? 'Running…' : 'Run it' }}</button>
          <p v-if="run && !run.dry_run && run.state !== 'running'" class="gb-start-report" role="status">
            {{ run.state === 'failed' ? `The run stopped: ${run.error ?? ''}` : `Done: ${reportWords(run.report)}.` }}
          </p>
        </template>
        <p v-else class="gb-hint">Nothing to run yet.</p>
      </li>
    </ol>

    <div class="gb-start-end">
      <button type="button" class="epfl-btn epfl-btn-primary" data-test="open-board" @click="finish">Open the board</button>
    </div>
  </main>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useRouter } from 'vue-router';
import { getState, getSync, initConfig, markdownExportURL, message, setForgeWay, startSync, type BoardState, type ForgeWay, type SyncForge, type SyncRun } from '@/api';
import { configModel } from '@/configmodel';
import { me } from '@/me';
import { onLive } from '@/live';
import { reportWords } from '@/ways';
import AddForgeDialog from '@/components/AddForgeDialog.vue';
import AppHeader from '@/components/AppHeader.vue';
import ForgeTokens from '@/components/ForgeTokens.vue';
import IdentityChooser from '@/components/IdentityChooser.vue';

const router = useRouter();
const viewNames = computed(() => configModel.value?.view_order ?? []);
const st = ref<BoardState | null>(null);
const error = ref('');
const busy = ref(false);
const adding = ref(false);
const run = ref<SyncRun | null>(null);
const checked = ref<SyncRun | null>(null);
const allowCreate = ref(false);

type Goal = 'see' | 'both' | 'backup' | 'markdown' | 'project';
const goal = ref<Goal | ''>('');
const goals: { id: Goal; name: string; hint: string }[] = [
  { id: 'see', name: 'See my forge’s issues here', hint: 'Read them here. Nothing is written on the forge.' },
  { id: 'both', name: 'Work here, and keep the forge in step', hint: 'What changes here goes there, and what changes there comes here.' },
  { id: 'backup', name: 'Keep a backup on another forge', hint: 'The board’s issues go there. An edit made there is put back.' },
  { id: 'markdown', name: 'Keep the issues as Markdown files', hint: 'A folder of files in the repository, for any text editor.' },
  { id: 'project', name: 'Plan in a GitHub Project', hint: 'The fields live in the Project’s columns.' },
];
const wantWay = computed<ForgeWay>(() => ({ see: 'pull', both: 'both', backup: 'push' } as const)[goal.value as 'see'] ?? 'both');
const wayWords = (way: string) => ({ push: 'a backup (push only)', pull: 'read only', both: 'both ways' })[way] ?? way;

const myName = computed(() => me.value?.me?.name ?? st.value?.me.id?.slice(0, 7) ?? '');
const tableForges = computed(() => st.value?.forges.filter((f) => !f.discovered) ?? []);
const foundForges = computed(() => st.value?.forges.filter((f) => f.discovered) ?? []);
const foundAsSync = computed<SyncForge[]>(() => foundForges.value.map((f) => ({ name: f.name, kind: f.kind, project: f.project, host: '', sync: true, way: f.way, discovered: true })));
function doneFor(g: Goal | ''): boolean {
  const fs = st.value?.forges ?? [];
  const way = ({ see: 'pull', both: 'both', backup: 'push' } as Record<string, string>)[g];
  if (way) return fs.some((f) => f.way === way && f.state !== 'off');
  if (g === 'markdown') return st.value?.datastore.state === 'markdown';
  if (g === 'project') return st.value?.project.state === 'configured';
  return false;
}
const goalDone = computed(() => doneFor(goal.value));
const goalDoneWords = computed(() => {
  const f = st.value?.forges.find((x) => x.way === wantWay.value && x.state !== 'off');
  return f ? `${f.name} (${f.project}) is ${wayWords(f.way)}.` : '';
});
const needToken = computed(() => st.value?.forges.some((f) => f.state === 'no-token') ?? false);
const syncable = computed(() => st.value?.forges.filter((f) => f.state === 'first' || f.state === 'on' || f.state === 'synced') ?? []);
const creates = computed(() => syncable.value.some((f) => f.way !== 'pull'));
const ranAll = computed(() => syncable.value.length > 0 && syncable.value.every((f) => f.state === 'synced'));
const running = computed(() => run.value?.state === 'running');

async function reload() {
  try {
    st.value = await getState();
    // a goal already reached is the one shown
    if (!goal.value) goal.value = goals.find((g) => doneFor(g.id))?.id ?? '';
  } catch (e) {
    error.value = message(e);
  }
}

async function act(fn: () => Promise<unknown>) {
  busy.value = true;
  error.value = '';
  try {
    await fn();
    await reload();
  } catch (e) {
    error.value = message(e);
  } finally {
    busy.value = false;
  }
}
const setWay = (name: string, way: ForgeWay) => act(() => setForgeWay(name, way));
const writeFile = () => act(() => initConfig());

let poll: ReturnType<typeof setInterval> | undefined;
async function go(dry: boolean) {
  error.value = '';
  try {
    run.value = await startSync({ scope: 'forges', dry_run: dry, allow_create: !dry && allowCreate.value });
  } catch (e) {
    error.value = message(e);
    return;
  }
  clearInterval(poll);
  poll = setInterval(async () => {
    const s = await getSync().catch(() => null);
    if (!s?.run || s.run.id !== run.value?.id) return;
    run.value = s.run;
    if (s.run.state === 'running') return;
    clearInterval(poll);
    if (s.run.dry_run) checked.value = s.run;
    await reload();
  }, 400);
}

function finish() {
  void router.push('/');
}

watch(me, () => void reload());
const off = onLive((e) => {
  if (e.kind === 'config' || e.kind === 'sync') void reload();
}, { own: true });
onMounted(reload);
onBeforeUnmount(() => {
  clearInterval(poll);
  off();
});
</script>

<style scoped>
.gb-start { max-width: 52rem; }
.gb-start-steps { display: grid; gap: var(--space-4); margin: var(--space-4) 0; padding: 0; list-style: none; }
.gb-start-step { display: grid; gap: var(--space-2); padding: var(--space-3) var(--space-4); border: var(--border-w) solid var(--border); border-left: 4px solid var(--border-strong); }
.gb-start-step.done { border-left-color: var(--success, #2e7d32); }
.gb-start-step h2 { margin: 0; }
.gb-start-goals { display: grid; gap: var(--space-2); }
.gb-start-goal { display: grid; gap: var(--space-2); padding-top: var(--space-2); }
.gb-start-goal .epfl-btn, .gb-start-step > .epfl-btn, .gb-start-step .epfl-btn-sm { justify-self: start; }
.gb-start-row { display: flex; flex-wrap: wrap; gap: var(--space-2); align-items: center; margin: 0; }
.gb-start-file { margin-top: var(--space-2); }
.gb-start-report { margin: 0; }
.gb-start-end { display: flex; flex-wrap: wrap; gap: var(--space-3); align-items: center; }
.gb-linkbtn { padding: 0; border: 0; background: none; color: var(--fg); text-decoration: underline; cursor: pointer; font: inherit; }
.af-check { display: flex; gap: var(--space-2); align-items: flex-start; }
</style>
