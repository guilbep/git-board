<!-- Releases (internal/server/releases.go): what runs where, a column per
     environment in order, and every release, candidates under their
     version. A job (deploy, cut, promote, rollback) runs on the server, one
     at a time, and says each step as a `releases` event: the page reloads
     on it. The canvas's Releases board; its alerts from the States board. -->
<template>
  <AppHeader :views="viewNames" active="" />
  <main class="gb-page">
    <span v-if="repo" class="epfl-eyebrow">{{ repo }}</span>
    <div class="gb-titlerow">
      <h1 class="gb-h1">Releases</h1>
      <span v-if="meta" class="gb-meta">{{ meta }}</span>
      <div v-if="canCut" class="gb-titlerow-actions">
        <button type="button" class="epfl-btn epfl-btn-primary" :disabled="busy" @click="openDialog({ kind: 'cut' })"><GbIcon name="tag" />Cut a release</button>
      </div>
    </div>

    <p v-if="!data && !error" class="gb-hint" role="status">Fetching the releases and the overlays…</p>
    <p v-if="error" class="epfl-alert epfl-alert-danger" role="alert">{{ error }}</p>
    <p v-if="actionError" class="epfl-alert epfl-alert-danger" role="alert"><b>Not started.</b> {{ actionError }}</p>

    <section v-if="data && !data.enabled" aria-labelledby="setup">
      <h2 id="setup" class="gb-h2 gb-rel-setup">No releases here yet</h2>
      <div v-if="doc" class="gb-md" v-html="doc" />
      <p><router-link to="/settings/project-file">Edit .git-board.toml in Settings</router-link></p>
    </section>

    <template v-else-if="data">
      <ReleaseJob v-if="data.job && (data.job.state !== 'done' || watched)" :job="data.job" :names="repoNames(envs)">
        <!-- a cut found its tag taken: who took it, and the next candidate (States w3) -->
        <template v-if="taken" #default>
          <div class="epfl-alert epfl-alert-info gb-alert-row" role="status">
            <GbIcon name="tag" />
            <span v-if="taken.row"><b>{{ (!taken.row.beforeGitBoard && taken.row.record?.author) || 'Someone' }} cut {{ taken.tag }} {{ on(taken.row.tag.date, true) }}</b>, on <span class="gb-mono">{{ sha7(taken.row.tag.commit) }}</span>{{ taken.row.tag.commit === lastCut?.commit ? ', the same commit' : '' }}.</span>
            <span v-else><b>Someone cut {{ taken.tag }} first.</b></span>
          </div>
          <div class="gb-actions">
            <router-link class="epfl-btn epfl-btn-primary epfl-btn-sm" :to="releasePath(taken.tag)">Open {{ taken.tag }}</router-link>
            <button v-if="taken.next" type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="busy" @click="recut">Cut as {{ taken.next }}</button>
          </div>
        </template>
      </ReleaseJob>

      <AdoptPanel v-if="envs.some((e) => !e.adopted && e.targets.length)" :envs="envs" :busy="busy" :adopt="adopt" />

      <section aria-labelledby="where">
        <h2 id="where" class="gb-sr">What runs where</h2>
        <div class="gb-envs">
          <div v-for="v in views" :key="v.env.name" class="gb-env">
            <div class="gb-env-head"><span :class="markClass(envMark(v.env.role, true))">{{ v.env.name }}</span></div>
            <div v-if="v.env.version || v.env.commit">
              <router-link v-if="tagPattern.test(v.env.version)" class="gb-env-ver gb-env-link" :to="releasePath(v.env.version)">{{ v.env.version }}</router-link>
              <span v-else class="gb-env-ver">{{ v.env.version || sha7(v.env.commit) }}</span>
              <div class="gb-env-sub">
                <span v-if="v.env.commit" class="gb-mono">{{ sha7(v.env.commit) }}</span>
                <span v-if="v.since">{{ v.env.role === 'follow' ? 'deployed' : 'since' }} {{ when(v.since.date, true) }} by <b>{{ v.since.author }}</b></span>
              </div>
            </div>
            <div v-if="v.split" class="gb-env-drift" role="status">
              <GbIcon name="warn" small />
              <span><b>{{ v.split[0] }}</b> {{ v.split[1] }}</span>
            </div>
            <ul v-if="v.targets.length" class="gb-targets" aria-label="Config repos">
              <li v-for="(t, j) in v.targets" :key="t.repo + t.path">
                <GbIcon v-if="t.deployed.commit && (!v.split || j === 0)" name="check" small class="gb-state-ok" />
                <GbIcon v-else name="error" small class="gb-state-warn" />
                <span>{{ t.name }}<template v-if="v.split"> · {{ runs(t) }}<template v-if="j === 0 && t.since"> since {{ when(t.since.date, true) }}, {{ t.since.author }}</template></template></span>
                <span v-if="!v.split" class="gb-mono" :title="t.path">{{ t.path.split('/').slice(-2).join('/') }}</span>
              </li>
            </ul>
            <p v-if="!v.env.adopted && v.targets.length" class="gb-env-sub">The overlays don't say which commit they run yet.</p>
            <div v-if="v.env.behind.length" class="gb-env-drift" role="status">
              <GbIcon name="warn" small />
              <span><b>{{ behindText(v.env, data.trunk || v.env.name) }}:</b> {{ merges(v.env.behind) }} {{ v.env.behind.length === 1 ? "isn't" : "aren't" }} deployed yet.</span>
            </div>
            <div class="gb-env-foot">
              <button v-if="v.env.role === 'follow'" type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="busy" @click="start(() => deployEnvironment(v.env.name))">Deploy {{ v.env.name }}</button>
              <button v-else-if="v.finish" type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="busy" @click="start(() => deployEnvironment(v.env.name, v.finish))">Finish deploying {{ v.env.name }}</button>
              <button v-else-if="v.promotion" type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="busy" @click="start(() => promoteRelease(v.promotion!.tag))">Promote to {{ v.promotion.to }} as {{ v.promotion.final }}</button>
              <button v-else-if="v.env.role === 'last' && v.env.version" type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="busy" @click="openDialog({ kind: 'rollback', env: v.env.name })"><GbIcon name="undo" small />Roll back…</button>
            </div>
          </div>
        </div>
      </section>

      <section aria-labelledby="all" class="gb-rel-all">
        <div class="gb-side-head">
          <h2 id="all" class="gb-h2">All releases</h2>
          <span class="gb-note">A tag names each one. What runs where comes from the overlays.</span>
        </div>
        <p v-if="!data.releases.length" class="gb-hint">No release tag yet.</p>
        <div v-else class="gb-table-wrap">
          <table class="gb-table">
            <thead><tr><th scope="col">Release</th><th scope="col">Commit</th><th scope="col">Cut</th><th scope="col">Runs in</th><th scope="col">Made by</th></tr></thead>
            <tbody>
              <template v-for="g in shownGroups" :key="g.version">
                <tr v-if="!g.final" class="gb-rel-group"><td colspan="5">v{{ g.version }} <span class="gb-note gb-rel-group-note">· candidates</span></td></tr>
                <tr v-for="r in g.final ? [g.final, ...g.candidates] : g.candidates" :key="r.tag.name" :class="{ 'gb-rel-cand': r.tag.version.rc, 'gb-rel-yanked': r.record?.yanked }">
                  <td><router-link class="gb-rel-name" :to="releasePath(r.tag.name)">{{ r.tag.name }}</router-link></td>
                  <td class="gb-mono">{{ sha7(r.tag.commit) }}</td>
                  <td>{{ when(r.tag.date, recent(r.tag.date)) }}</td>
                  <td>
                    <template v-if="r.record?.yanked">Yanked: {{ r.record.yanked }}</template>
                    <span v-else-if="r.runsIn.length" class="gb-envline">
                      <span v-for="n in r.runsIn" :key="n" :class="markClass(envMark(roleOf(n), true))">{{ n }}</span>
                      <span v-if="lastTargets(r)" class="gb-note">{{ lastTargets(r) }}</span>
                    </span>
                    <span v-else :class="markClass('off')">nowhere</span>
                  </td>
                  <!-- a release before git-board: its record's author only reconciled it -->
                  <td><span v-if="r.beforeGitBoard" class="gb-tag">before git-board</span><template v-else>{{ r.record?.author }}</template></td>
                </tr>
              </template>
            </tbody>
          </table>
        </div>
        <div v-if="groups.length > shownGroups.length || legacy" class="gb-actions gb-rel-more">
          <button v-if="groups.length > shownGroups.length" type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" @click="showAll = true">Show older releases</button>
          <span v-if="legacy" class="gb-note">Releases cut before git-board get their record from their tag: commit and images by digest.</span>
        </div>
      </section>
    </template>

    <!-- persistent while its POST is out, so a refusal lands in it; the
         content stays through the hide animation, cleared on hide -->
    <q-dialog v-model="dialogOpen" :persistent="pending" @hide="dialogOpen || (dialog = null)">
      <CutReleaseDialog v-if="dialog?.kind === 'cut'" v-model:sending="pending" :envs="envs" :releases="data?.releases ?? []" :remote="data?.remote || 'origin'" :trunk="data?.trunk ?? ''" :busy="busy" @close="dialogOpen = false" @started="onCut" />
      <RollbackDialog v-else-if="dialog?.kind === 'rollback'" v-model:sending="pending" :env="dialog.env" :envs="envs" :releases="data?.releases ?? []" :busy="busy" @close="dialogOpen = false" @started="onStarted" />
    </q-dialog>
  </main>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import AdoptPanel from '@/components/AdoptPanel.vue';
import AppHeader from '@/components/AppHeader.vue';
import CutReleaseDialog from '@/components/CutReleaseDialog.vue';
import GbIcon from '@/components/GbIcon.vue';
import ReleaseJob from '@/components/ReleaseJob.vue';
import RollbackDialog from '@/components/RollbackDialog.vue';
import { adoptOverlays, cutRelease, deployEnvironment, getConfigDocs, getDraft, getReleases, getViews, promoteRelease, type CutRequest, type ReleaseJob as Job, type Proposal, type ReleaseRow, type Releases } from '@/api/board';
import { message } from '@/issue';
import { debounced, onLive } from '@/live';
import { behindText, byVersion, envMark, groupReleases, markClass, newest, promotion, releasePath, repoName, repoNames, runs, sha7, splitParts, tagPattern } from '@/releases';
import { remotes } from '@/remotes';
import { on, when } from '@/when';

const viewNames = ref<string[]>([]);
const data = ref<Releases | null>(null);
const error = ref('');
const actionError = ref('');
const pending = ref(false); // an action sent, its answer not back yet
const doc = ref(''); // docs/configuration.md's ## Release, without [release]
const showAll = ref(false);
// a job this page saw running stays shown once done; a failed one always
const watched = ref(false);
// the dialog asked for: a cut, or rolling an environment back; kept until
// the dialog has hidden
const dialog = ref<{ kind: 'cut' } | { kind: 'rollback'; env: string } | null>(null);
const dialogOpen = ref(false);
// the cut this page last asked for, to ask again when its tag was taken
const lastCut = ref<CutRequest | null>(null);

const running = computed(() => data.value?.job?.state === 'running');
const busy = computed(() => running.value || pending.value);
const envs = computed(() => data.value?.environments ?? []);
const canCut = computed(() => envs.value.some((e) => e.role === 'candidate' || e.role === 'last'));
const list = new Intl.ListFormat('en', { type: 'conjunction' });
// "dev, stage, prod · deployed to enack8s and openshift"
const meta = computed(() => {
  if (!envs.value.length) return '';
  const targets = [...new Set(envs.value.flatMap((e) => e.targets.map((t) => t.name)))];
  return envs.value.map((e) => e.name).join(', ') + (targets.length ? ` · deployed to ${list.format(targets)}` : '');
});
// the repository, as origin names it
const repo = computed(() => {
  const r = remotes.value?.find((x) => x.name === 'origin') ?? remotes.value?.[0];
  return r ? repoName(r.url) : '';
});

// each environment's column, worked out once: in a split, the targets lead
// first (the highest version), its sentence, and the release to finish with
const views = computed(() =>
  envs.value.map((env, i) => {
    const targets = env.split ? byVersion(env.targets) : env.targets;
    const lead = targets[0]?.deployed.version ?? '';
    return {
      env,
      targets,
      since: newest(env.targets),
      split: env.split ? splitParts(env) : null,
      finish: env.split && env.role !== 'follow' && tagPattern.test(lead) ? lead : '',
      promotion: promotion(envs.value, i),
    };
  }),
);

// a cut that found its tag taken: the tag, its row once fetched, and the
// candidate after it when this page can cut again
const taken = computed(() => {
  const j = data.value?.job;
  const tag = j?.state === 'failed' && j.code === 'taken' && j.kind === 'cut' ? j.tag : '';
  if (!tag) return null;
  const rc = tagPattern.exec(tag)?.[4];
  return { tag, row: data.value?.releases.find((r) => r.tag.name === tag), next: rc && lastCut.value ? `rc.${+rc + 1}` : '' };
});

const groups = computed(() => groupReleases(data.value?.releases ?? []));
const shownGroups = computed(() => (showAll.value ? groups.value : groups.value.slice(0, 10)));
const legacy = computed(() => data.value?.releases.some((r) => r.beforeGitBoard) ?? false);

// cut in the last two weeks: "17 Oct, 14:02"; before: "28 Sep"
const recent = (iso: string) => Date.now() - Date.parse(iso) < 14 * 864e5;
const roleOf = (env: string) => envs.value.find((e) => e.name === env)?.role ?? '';
// the merges dev doesn't run, three at most
const merges = (b: string[]) => list.format([...b.slice(0, 3).map((s) => `“${s}”`), ...(b.length > 3 ? [`${b.length - 3} more`] : [])]);
// the last environment's targets, where a release runs in it
const lastTargets = (r: ReleaseRow) => {
  const last = envs.value.find((e) => e.role === 'last');
  return last && r.runsIn.includes(last.name) && last.targets.length > 1 ? last.targets.map((t) => t.name).join(', ') : '';
};

async function reload() {
  try {
    const r = await getReleases();
    if (r.job?.state === 'running') watched.value = true;
    else actionError.value = ''; // a refusal while a job ran is stale once none runs
    data.value = r;
    error.value = '';
    if (!r.enabled && !doc.value) doc.value = (await getConfigDocs()).find((d) => d.id === 'release')?.html ?? '';
  } catch (e) {
    error.value = message(e);
  }
}

// a deploy or a promotion: the server answers with the job started (202),
// or 409 releasing while another runs
async function start(f: () => Promise<Job>) {
  actionError.value = '';
  pending.value = true;
  try {
    const job = await f();
    watched.value = true;
    if (data.value) data.value.job = job;
  } catch (e) {
    actionError.value = message(e);
  } finally {
    pending.value = false;
  }
}

// a dialog's job started: the page shows it
function onStarted(job: Job) {
  actionError.value = '';
  watched.value = true;
  if (data.value) data.value.job = job;
  dialogOpen.value = false;
}

function openDialog(d: NonNullable<typeof dialog.value>) {
  dialog.value = d;
  dialogOpen.value = true;
}

function onCut(job: Job, req: CutRequest) {
  lastCut.value = req;
  onStarted(job);
}

// the same cut again: the draft first, which fetches origin's tags as the
// dialog's did; the engine numbers the candidate past the one taken
function recut() {
  const req = lastCut.value;
  if (req) void start(() => getDraft(req.final).then(() => cutRelease(req)));
}

// adopt: a quick write, busy until it answers; the panel says a refusal
async function adopt(ps: Proposal[]) {
  pending.value = true;
  try {
    await adoptOverlays(ps);
  } finally {
    pending.value = false;
  }
  await reload();
}

// a job's steps; while one runs, "issues" too: live.ts says it after a
// reconnect, when a step may have been missed; config: [release] changed
const reloadSoon = debounced(() => void reload());
const offLive = onLive((e) => (e.kind === 'releases' || e.kind === 'config' || (e.kind === 'issues' && running.value)) && reloadSoon());
onBeforeUnmount(offLive);

onMounted(async () => {
  void getViews().then(
    (v) => (viewNames.value = Object.keys(v).sort()),
    () => {},
  );
  await reload();
});
</script>

<style scoped>
.gb-rel-setup { margin: .75rem 0 1.5rem; }
.gb-env-link { text-decoration: none; }
.gb-rel-all { margin-top: 2.5rem; }
.gb-rel-all .gb-side-head { margin-bottom: 1rem; }
.gb-rel-group-note { font-weight: 400; }
.gb-rel-more { margin-top: 1rem; }
</style>
