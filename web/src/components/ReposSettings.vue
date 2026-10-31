<!-- Settings › Repos: the [repos] table — the GitHub repositories the
     project's labels live on, in the order the label page reads them;
     without one, origin. The assignees setting (how the forges hold
     assignees) is preserved but written by the Sync, not here. -->
<template>
  <div class="set-panel rs">
    <h2 class="h4">Repos</h2>
    <p class="rs-lead">
      The GitHub repositories the project's labels, numbers and issues live on — <code>owner/name</code>, in this order.
      None, and origin stands for them all. The issue pages read them in this order; the label page asks each.
    </p>

    <p v-if="modelError" class="epfl-alert epfl-alert-danger" role="alert">{{ modelError }}</p>
    <p v-else-if="!draft.value.value" class="rs-muted">Loading…</p>

    <template v-else>
      <ConfigRows :rows="draft.value.value.list" add-label="Add a repository" @update="draft.value.value.list = $event" @add="draft.value.value.list = [...draft.value.value.list, '']">
        <template #row="{ row, set }">
          <input :model-value="row" class="rs-input" placeholder="epfl-enac/git-board" spellcheck="false" aria-label="A GitHub repository" @update:model-value="set($event)" />
        </template>
      </ConfigRows>
      <p class="gb-hint">The order matters: the first is the issue pages' own repository, the number's authority.</p>

      <ConfigSaveBar
        section="repos"
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

    <ConfigRefPanel :sections="['repos']" title="the repos table" />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue';
import { SectionDraft, configModel, configModelError } from '@/configmodel';
import ConfigRefPanel from '@/components/configforms/ConfigRefPanel.vue';
import ConfigRows from '@/components/configforms/ConfigRows.vue';
import ConfigSaveBar from '@/components/configforms/ConfigSaveBar.vue';

type Repos = { list: string[] };

const modelError = configModelError;

const draft = new SectionDraft<Repos>(
  'repos',
  (m) => ({ list: [...m.repos.list] }),
  (a, b) => JSON.stringify(a.list) !== JSON.stringify(b.list),
  (value) => ({ list: value.list.filter((r) => r.trim()) }),
);

const valid = computed(() => (draft.value.value?.list ?? []).every((r) => /^[\w.-]+\/[\w.-]*$/.test(r.trim()) || !r.trim()));
const state = computed(() => {
  const n = (draft.value.value?.list ?? []).filter((r) => r.trim()).length;
  return n ? `${n} repositor${n === 1 ? 'y' : 'ies'}` : 'origin';
});

function reset() {
  if (configModel.value) draft.reset(configModel.value);
}

onMounted(() => void draft.load());
</script>

<style scoped>
.set-panel { display: grid; gap: 1rem; max-width: 64rem; }
h2, p { margin: 0; }
.rs-lead { max-width: 42rem; color: var(--fg-muted); }
.rs-muted { color: var(--fg-muted); }
.rs-input { width: 20rem; padding: .375rem .625rem; border: 1px solid var(--border); background: var(--bg); color: var(--fg); font: inherit; }
.rs-input:focus { outline: 2px solid var(--fg); outline-offset: 2px; border-color: var(--fg); }
</style>
