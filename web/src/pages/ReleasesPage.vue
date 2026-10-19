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
        <button type="button" class="epfl-btn epfl-btn-primary" :disabled="running" @click="dialog = { kind: 'cut' }"><GbIcon name="tag" />Cut a release</button>
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
      <ReleaseJob v-if="data.job && (data.job.state !== 'done' || watched)" :job="data.job" :names="repoNames" />

      <section aria-labelledby="where">
        <h2 id="where" class="gb-sr">What runs where</h2>
        <div class="gb-envs">
          <div v-for="(e, i) in data.environments" :key="e.name" class="gb-env">
            <div class="gb-env-head"><span :class="markClass(envMark(e.role, true))">{{ e.name }}</span></div>
            <div v-if="e.version || e.commit">
              <span class="gb-env-ver">{{ e.version || sha7(e.commit) }}</span>
              <div class="gb-env-sub">
                <span v-if="e.commit" class="gb-mono">{{ sha7(e.commit) }}</span>
                <span v-if="since(e)">{{ e.role === 'follow' ? 'deployed' : 'since' }} {{ when(since(e)!.date, true) }} by <b>{{ since(e)!.author }}</b></span>
              </div>
            </div>
            <div v-if="e.split" class="gb-env-drift" role="status">
              <GbIcon name="warn" small />
              <span><b>{{ splitParts(e)[0] }}</b> {{ splitParts(e)[1] }}</span>
            </div>
            <ul v-if="e.targets.length" class="gb-targets" aria-label="Config repos">
              <li v-for="(t, j) in targetsOf(e)" :key="t.repo + t.path">
                <GbIcon v-if="t.deployed.commit && (!e.split || j === 0)" name="check" small class="gb-state-ok" />
                <GbIcon v-else name="error" small class="gb-state-warn" />
                <span>{{ t.name }}<template v-if="e.split"> · {{ runs(t) }}<template v-if="t.since"> since {{ when(t.since.date, true) }}, {{ t.since.author }}</template></template></span>
                <span v-if="!e.split" class="gb-mono" :title="t.path">{{ t.path.split('/').slice(-2).join('/') }}</span>
              </li>
            </ul>
            <p v-if="!e.adopted && e.targets.length" class="gb-env-sub">The overlays don't say which commit they run yet.</p>
            <div v-if="e.behind.length" class="gb-env-drift" role="status">
              <GbIcon name="warn" small />
              <span><b>{{ behindText(e) }}:</b> {{ merges(e.behind) }} {{ e.behind.length === 1 ? "isn't" : "aren't" }} deployed yet.</span>
            </div>
            <div class="gb-env-foot">
              <button v-if="e.role === 'follow'" type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="running" @click="start(() => deployEnvironment(e.name))">Deploy {{ e.name }}</button>
              <button v-else-if="e.split && finishTag(e)" type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="running" @click="start(() => deployEnvironment(e.name, finishTag(e)))">Finish deploying {{ e.name }}</button>
              <button v-else-if="promotion(data.environments, i)" type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="running" @click="start(() => promoteRelease(promotion(data!.environments, i)!.tag))">Promote to {{ promotion(data.environments, i)!.to }} as {{ promotion(data.environments, i)!.final }}</button>
              <button v-else-if="e.role === 'last' && e.version" type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="running" @click="dialog = { kind: 'rollback', env: e.name }"><GbIcon name="undo" small />Roll back…</button>
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
                  <td><router-link class="gb-rel-name" :to="`/releases/${encodeURIComponent(r.tag.name)}`">{{ r.tag.name }}</router-link></td>
                  <td class="gb-mono">{{ sha7(r.tag.commit) }}</td>
                  <td>{{ when(r.tag.date) }}</td>
                  <td>
                    <template v-if="r.record?.yanked">Yanked: {{ r.record.yanked }}</template>
                    <span v-else-if="r.runsIn.length" class="gb-envline">
                      <span v-for="n in r.runsIn" :key="n" :class="markClass(envMark(roleOf(n), true))">{{ n }}</span>
                      <span v-if="lastTargets(r)" class="gb-note">{{ lastTargets(r) }}</span>
                    </span>
                    <span v-else :class="markClass('off')">nowhere</span>
                  </td>
                  <td>{{ r.record?.author }} <span v-if="!r.record?.author" class="gb-tag">before git-board</span></td>
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
  </main>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import AppHeader from '@/components/AppHeader.vue';
import GbIcon from '@/components/GbIcon.vue';
import ReleaseJob from '@/components/ReleaseJob.vue';
import { deployEnvironment, getConfigDocs, getReleases, getViews, promoteRelease, type EnvState, type ReleaseJob as Job, type ReleaseRow, type Releases } from '@/api/board';
import { message } from '@/issue';
import { debounced, onLive } from '@/live';
import { behindText, byRecent, envMark, groupReleases, promotion, runs, splitText, type EnvMark } from '@/releases';
import { remotes } from '@/remotes';
import { when } from '@/when';

const viewNames = ref<string[]>([]);
const data = ref<Releases | null>(null);
const error = ref('');
const actionError = ref('');
const doc = ref(''); // docs/configuration.md's ## Release, without [release]
const showAll = ref(false);
// a job this page saw running stays shown once done; a failed one always
const watched = ref(false);
// the dialog asked for: a cut, or rolling an environment back
const dialog = ref<{ kind: 'cut' } | { kind: 'rollback'; env: string } | null>(null);

const running = computed(() => data.value?.job?.state === 'running');
const envs = computed(() => data.value?.environments ?? []);
const canCut = computed(() => envs.value.some((e) => e.role === 'candidate' || e.role === 'last'));
const list = new Intl.ListFormat('en', { type: 'conjunction' });
// "dev, stage, prod · deployed to enack8s and openshift"
const meta = computed(() => {
  if (!envs.value.length) return '';
  const targets = envs.value[0]!.targets.map((t) => t.name);
  return envs.value.map((e) => e.name).join(', ') + (targets.length ? ` · deployed to ${list.format(targets)}` : '');
});
// the repository, as origin names it
const repo = computed(() => {
  const r = remotes.value?.find((x) => x.name === 'origin') ?? remotes.value?.[0];
  return r ? (r.url.replace(/(\.git)?\/*$/, '').split(/[/:]/).at(-1) ?? '') : '';
});

const repoNames = computed(() => Object.fromEntries(envs.value.flatMap((e) => e.targets.map((t) => [t.repo, t.name]))));
const groups = computed(() => groupReleases(data.value?.releases ?? []));
const shownGroups = computed(() => (showAll.value ? groups.value : groups.value.slice(0, 10)));
const legacy = computed(() => data.value?.releases.some((r) => !r.record?.author) ?? false);

const sha7 = (c: string) => c.slice(0, 7);
const markClass = (m: EnvMark) => ['gb-envmark', m === 'dev' ? '' : `gb-envmark-${m}`];
const roleOf = (env: string) => envs.value.find((e) => e.name === env)?.role ?? '';
// when the environment changed last, and who did it: its newest overlay change
const since = (e: EnvState) => byRecent(e.targets)[0]?.since ?? null;
// in a split, the target a deploy already moved first
const targetsOf = (e: EnvState) => (e.split ? byRecent(e.targets) : e.targets);
const splitParts = (e: EnvState) => {
  const s = splitText(e);
  const k = s.indexOf('. ') + 1;
  return [s.slice(0, k), s.slice(k + 1)];
};
// a split release environment: the release its first target moved to
const finishTag = (e: EnvState) => {
  const v = byRecent(e.targets)[0]?.deployed.version ?? '';
  return e.role !== 'follow' && /^v\d+\.\d+\.\d+(-rc\.\d+)?$/.test(v) ? v : '';
};
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
  try {
    const job = await f();
    watched.value = true;
    if (data.value) data.value.job = job;
  } catch (e) {
    actionError.value = message(e);
  }
}

const reloadSoon = debounced(() => void reload());
const offLive = onLive((e) => e.kind === 'releases' && reloadSoon());
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
.gb-rel-all { margin-top: 2.5rem; }
.gb-rel-all .gb-side-head { margin-bottom: 1rem; }
.gb-rel-group-note { font-weight: 400; }
.gb-rel-more { margin-top: 1rem; }
</style>
