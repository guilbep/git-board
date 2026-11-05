<!-- Settings › Experimental: the [experimental] table, the work-in-progress
     features (experimental.ts): the development cycle. Off by default; a toggle writes the table at once, for
     the whole team. A clone's git config board.experimental wins over it
     there, and the page says so. -->
<template>
  <div class="set-panel xs">
    <h2 class="h4">Experimental</h2>
    <p class="xs-lead">Features still being built, off unless the team turns them on. git-board’s core, your issues kept in git whatever the forge, works without any of them.</p>

    <div class="set-cols">
      <div class="set-forms">
        <p v-if="modelError" class="epfl-alert epfl-alert-danger" role="alert">{{ modelError }}</p>
        <p v-else-if="!model" class="xs-muted">Loading…</p>

        <template v-else>
          <p class="gb-hint">{{ whereFrom(model.local) }}</p>
          <ul class="xs-list">
            <li v-for="f in model.known" :key="f.name">
              <label class="xs-row">
                <input
                  type="checkbox"
                  role="switch"
                  :checked="(draft.value.value?.features ?? model.features).includes(f.name)"
                  :disabled="draft.saving.value"
                  @change="toggle(f.name, ($event.target as HTMLInputElement).checked)"
                />
                <span>
                  <b>{{ f.title }}</b> <code>{{ f.name }}</code><span v-if="model.on.includes(f.name)" class="xs-on">on here</span><br />
                  <span class="gb-hint">{{ f.description }}</span>
                </span>
              </label>
            </li>
          </ul>
          <p v-if="draft.saveError.value" class="gb-save-error" role="alert">
            <b>Not saved.</b> {{ draft.saveError.value }}
            <button v-if="draft.conflict.value" type="button" class="gb-linkbtn" @click="draft.load()">Reload</button>
          </p>
          <p v-else-if="draft.saving.value" class="gb-hint" role="status">Saving…</p>
          <p v-else-if="draft.saved.value" class="gb-hint" role="status">Saved to .git-board.toml.</p>
        </template>
      </div>

      <ConfigRefPanel :sections="['experimental']" title="the experimental table" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue';
import { SectionDraft, configModel, configModelError } from '@/configmodel';
import { toggled, whereFrom, type Feature } from '@/experimental';
import { loadMe } from '@/me';
import ConfigRefPanel from '@/components/configforms/ConfigRefPanel.vue';

type Experimental = { features: string[] };

const modelError = configModelError;
const model = computed(() => configModel.value?.experimental ?? null);

const draft = new SectionDraft<Experimental>(
  'experimental',
  (m) => ({ features: [...m.experimental.features] }),
  (a, b) => JSON.stringify(a.features) !== JSON.stringify(b.features),
  (value) => ({ features: value.features }),
);

// a toggle saves at once; the header and the tabs follow /api/me
async function toggle(name: string, on: boolean) {
  if (!draft.value.value) return;
  draft.value.value = { features: toggled(draft.value.value.features, name as Feature, on) };
  if (await draft.save()) void loadMe();
}

onMounted(() => void draft.load());
</script>

<style scoped>
.set-panel { display: grid; gap: .75rem; container-type: inline-size; }
.set-cols { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); gap: .75rem; align-items: start; }
.set-forms { display: grid; gap: .75rem; min-width: 0; }
@container (max-width: 52rem) { .set-cols { grid-template-columns: minmax(0, 1fr); } }
h2, p { margin: 0; }
.xs-lead { max-width: 42rem; color: var(--fg-muted); }
.xs-muted { color: var(--fg-muted); }
.xs-list { list-style: none; margin: 0; padding: 0; display: grid; gap: .75rem; }
.xs-row { display: flex; align-items: flex-start; gap: .625rem; line-height: 1.45; }
.xs-row input { margin-top: .3rem; }
.xs-on { margin-left: .5rem; font-size: .8125rem; color: var(--fg-muted); }
</style>
