<!-- Settings › Release: the [release] family — the environments the code
     gets to, in order (the last takes the finals), the command a merge
     must pass, what to build (images, the chart, binaries), and the
     config repos holding the overlays as [[release.deploy]]. Nothing of
     release in it, the project has no releases and the tables go. -->
<template>
  <div class="set-panel rls">
    <h2 class="h4">Release</h2>
    <p class="rls-lead">
      How the project's code gets to its environments: <code>git board release</code> tags, builds and deploys — dev
      follows the trunk's head, candidates go to the middle environments, finals to the last one.
    </p>

    <p v-if="modelError" class="epfl-alert epfl-alert-danger" role="alert">{{ modelError }}</p>
    <p v-else-if="!draft.value.value" class="rls-muted">Loading…</p>

    <template v-else>
      <ConfigRows :rows="draft.value.value.environments" add-label="Add an environment" @update="draft.value.value.environments = $event" @add="draft.value.value.environments = [...draft.value.value.environments, '']">
        <template #row="{ row, index, set }">
          <div class="rls-env">
            <input :model-value="row" class="rls-input" placeholder="dev" spellcheck="false" :aria-label="`Environment ${index + 1}`" @update:model-value="set($event)" />
            <span class="rls-tag">{{ tag(index) }}</span>
          </div>
        </template>
      </ConfigRows>
      <p class="gb-hint">
        In order: the first follows the trunk's head (unless Follow says otherwise), the last takes the finals, the middle
        ones take candidates. One environment: finals only.
      </p>

      <div class="rls-grid">
        <label class="rls-field">
          <span>A merge must pass</span>
          <input :value="draft.value.value.checks" class="rls-input" placeholder="make ci" spellcheck="false" aria-label="The checks command" @input="set('checks', ($event.target as HTMLInputElement).value)" />
        </label>
        <label class="rls-field">
          <span>Tags and release records on</span>
          <input :value="draft.value.value.remote" class="rls-input" placeholder="origin" spellcheck="false" aria-label="The authority remote" @input="set('remote', ($event.target as HTMLInputElement).value)" />
        </label>
        <label class="rls-field">
          <span>Follows the trunk</span>
          <input :value="draft.value.value.follow ?? ''" class="rls-input" placeholder="the first environment" spellcheck="false" aria-label="The environment that follows the trunk" @input="set('follow', ($event.target as HTMLInputElement).value)" />
        </label>
      </div>

      <h3 class="h5">Images</h3>
      <ConfigRows :rows="imageRows" add-label="Add an image" @update="imageRows = $event" @add="imageRows = [...imageRows, { name: '', context: '.' }]">
        <template #row="{ row, set: setRow }">
          <div class="rls-pair">
            <input :model-value="row.name" class="rls-input" placeholder="board" spellcheck="false" aria-label="The image's name" @update:model-value="setRow({ ...row, name: $event })" />
            <input :model-value="row.context" class="rls-input" placeholder="." spellcheck="false" aria-label="The docker build context" @update:model-value="setRow({ ...row, context: $event })" />
          </div>
        </template>
      </ConfigRows>

      <h3 class="h5">Helm chart</h3>
      <div class="rls-grid">
        <label class="rls-field">
          <span>Chart directory</span>
          <input :value="draft.value.value.chartPath ?? ''" class="rls-input" placeholder="chart" spellcheck="false" aria-label="The chart's directory" @input="setChart('path', ($event.target as HTMLInputElement).value)" />
        </label>
        <label class="rls-field">
          <span>OCI registry</span>
          <input :value="draft.value.value.chartRegistry ?? ''" class="rls-input" placeholder="registry.epfl.ch/enac" spellcheck="false" aria-label="The OCI registry" @input="setChart('registry', ($event.target as HTMLInputElement).value)" />
        </label>
      </div>

      <h3 class="h5">Deploy — the config repos holding the overlays</h3>
      <ConfigRows :rows="deployRows" add-label="Add a deploy target" @update="deployRows = $event" @add="deployRows = [...deployRows, { repo: '', path: 'apps/app/{env}' }]">
        <template #row="{ row, set: setRow }">
          <div class="rls-pair">
            <input :model-value="row.repo" class="rls-input" placeholder="epfl-enac/app-deploy" spellcheck="false" aria-label="The config repository" @update:model-value="setRow({ ...row, repo: $event })" />
            <input :model-value="row.path" class="rls-input" placeholder="apps/app/{env}" spellcheck="false" aria-label="The overlays' path, {env} in it" @update:model-value="setRow({ ...row, path: $event })" />
          </div>
        </template>
      </ConfigRows>
      <p class="gb-hint">
        The path holds <code>{env}</code>; the overlay there gets the image tags and the version. Images built, every
        target needs a registry for them.
      </p>

      <details class="rls-more" :open="binaries.length > 0">
        <summary>Binaries <span class="rls-count">{{ binaries.length }}</span></summary>
        <ConfigRows :rows="binaries" add-label="Add a main package" @update="set('binaries', $event)" @add="set('binaries', [...binaries, './cmd/…'])">
          <template #row="{ row, set }">
            <input :model-value="row" class="rls-input" placeholder="./cmd/git-board" spellcheck="false" aria-label="A main package" @update:model-value="set($event)" />
          </template>
        </ConfigRows>
        <p class="gb-hint">
          Main packages built per platform and published as archives on the forge's release. A table with binaries and no
          environments is enabled: finals only, deployed nowhere.
        </p>
      </details>

      <ConfigSaveBar
        section="release"
        :dirty="draft.dirty.value"
        :saving="draft.saving.value"
        :saved="draft.saved.value"
        :valid="valid"
        :error="draft.saveError.value"
        :conflict="draft.conflict.value"
        :state="state"
        @save="draft.save()"
        @discard="reset"
        @reload="draft.load()"
      />
    </template>

    <ConfigRefPanel :sections="['release']" title="the release tables" />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue';
import type { ConfigRelease } from '@/api/board';
import { SectionDraft, configModel, configModelError } from '@/configmodel';
import ConfigRefPanel from '@/components/configforms/ConfigRefPanel.vue';
import ConfigRows from '@/components/configforms/ConfigRows.vue';
import ConfigSaveBar from '@/components/configforms/ConfigSaveBar.vue';

type ImageRow = { name: string; context: string };
type DeployRow = { repo: string; path: string };
type Release = {
  remote: string;
  environments: string[];
  follow: string;
  checks: string;
  images: ImageRow[];
  chartPath: string;
  chartRegistry: string;
  deploy: DeployRow[];
  binaries: string[];
};

const modelError = configModelError;

const draft = new SectionDraft<Release>(
  'release',
  (m) => ({
    remote: m.release.remote ?? '',
    environments: [...(m.release.environments ?? [])],
    follow: m.release.follow ?? '',
    checks: m.release.checks ?? '',
    images: (m.release.images ?? []).map((i) => ({ ...i })),
    chartPath: m.release.chart?.path ?? '',
    chartRegistry: m.release.chart?.registry ?? '',
    deploy: (m.release.deploy ?? []).map((t) => ({ repo: t.repo, path: t.path })),
    binaries: [...(m.release.binaries ?? [])],
  }),
  (a, b) => JSON.stringify(a) !== JSON.stringify(b),
  (value) => {
    const r: ConfigRelease = {
      remote: value.remote || undefined,
      environments: value.environments.filter((e) => e.trim()),
      follow: value.follow || null,
      checks: value.checks || undefined,
      images: value.images.filter((i) => i.name.trim()),
      deploy: value.deploy.filter((t) => t.repo.trim()),
      binaries: value.binaries.filter((b) => b.trim()),
    };
    if (value.chartPath.trim() || value.chartRegistry.trim()) {
      r.chart = { path: value.chartPath.trim(), registry: value.chartRegistry.trim() };
    }
    return r;
  },
);

function set<K extends keyof Release>(key: K, v: Release[K]) {
  if (draft.value.value) draft.value.value = { ...draft.value.value, [key]: v };
}

function setChart(key: 'path' | 'registry', v: string) {
  if (!draft.value.value) return;
  set(key === 'path' ? 'chartPath' : 'chartRegistry', v);
}

const imageRows = computed({
  get: () => draft.value.value?.images ?? [],
  set: (rows) => set('images', rows),
});
const deployRows = computed({
  get: () => draft.value.value?.deploy ?? [],
  set: (rows) => set('deploy', rows),
});
const binaries = computed({
  get: () => draft.value.value?.binaries ?? [],
  set: (rows) => set('binaries', rows),
});

// the environment's role in the flow, as the hint under the list says it
function tag(i: number): string {
  const n = draft.value.value?.environments.filter((e) => e.trim()).length ?? 0;
  if (n === 1) return 'finals only';
  if (i === 0) return 'follows the trunk';
  if (i === n - 1) return 'finals';
  return 'candidates';
}

const valid = computed(() => {
  const r = draft.value.value;
  if (!r) return false;
  const envs = r.environments.filter((e) => e.trim());
  return (
    envs.every((e) => /^[A-Za-z0-9_-]+$/.test(e.trim())) &&
    new Set(envs.map((e) => e.trim())).size === envs.length &&
    r.images.every((i) => i.name.trim() !== '') &&
    r.deploy.every((t) => t.repo.trim() !== '' && t.path.includes('{env}')) &&
    (!r.follow?.trim() || envs.includes(r.follow.trim()))
  );
});
const state = computed(() => {
  const r = draft.value.value;
  if (!r) return '';
  const n = r.environments.filter((e) => e.trim()).length;
  return n ? `${n} environment${n === 1 ? '' : 's'} · ${r.images.length} image${r.images.length === 1 ? '' : 's'}` : 'no releases';
});

function reset() {
  if (configModel.value) draft.reset(configModel.value);
}

onMounted(() => void draft.load());
</script>

<style scoped>
.set-panel { display: grid; gap: 1rem; max-width: 64rem; }
h2, h3, p { margin: 0; }
.rls-lead { max-width: 42rem; color: var(--fg-muted); }
.rls-muted { color: var(--fg-muted); }
.rls-grid { display: flex; gap: 1rem; flex-wrap: wrap; }
.rls-field { display: grid; gap: .25rem; font-size: .8125rem; }
.rls-field > span { color: var(--fg-muted); }
.rls-input { padding: .375rem .625rem; border: 1px solid var(--border); background: var(--bg); color: var(--fg); font: inherit; }
.rls-input:focus { outline: 2px solid var(--fg); outline-offset: 2px; border-color: var(--fg); }
.rls-env { display: flex; align-items: center; gap: .5rem; }
.rls-tag { font-size: .75rem; color: var(--fg-muted); }
.rls-pair { display: flex; gap: .5rem; flex-wrap: wrap; }
.rls-pair .rls-input { width: 14rem; }
.rls-more { display: grid; gap: .5rem; }
.rls-more summary { cursor: pointer; font-size: .875rem; font-weight: 700; color: var(--fg-muted); }
.rls-more summary:hover { color: var(--fg); }
.rls-count { display: inline-block; margin-left: .375rem; padding: 0 .375rem; border: 1px solid var(--border); font-weight: 400; font-size: .75rem; color: var(--fg-muted); }
</style>
