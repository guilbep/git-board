<!-- Settings › Attachments: the [uploads] table — the S3 the files
     attached to comments go to, and the CDN that serves them. Every key
     empty is the defaults, and the table goes from the file. Off (no
     table), the comment box holds no attach button. -->
<template>
  <div class="set-panel as">
    <h2 class="h4">Attachments</h2>
    <p class="as-lead">Files attached to comments upload to an S3 bucket, served by a CDN.</p>
    <!-- the switch that decides comes first: the server's keys, not the file -->
    <p v-if="status" class="as-status" :class="status.enabled ? 'as-on' : 'as-off'" role="status">
      <template v-if="status.enabled"><b>On</b>{{ status.where ? `, served at ${status.where}` : '' }}.</template>
      <template v-else><b>Off on this server</b>: set <code>ACCESS_KEY_ID</code>, <code>ACCESS_KEY_SECRET</code> and <code>BUCKET_NAME</code> where git-board runs, then restart it.</template>
    </p>
    <h3 class="as-h">Where the files go, once on</h3>

    <div class="set-cols">
      <div class="set-forms">
        <p v-if="modelError" class="epfl-alert epfl-alert-danger" role="alert">{{ modelError }}</p>
        <p v-else-if="!draft.value.value" class="as-muted">Loading…</p>

        <template v-else>
          <div class="as-grid">
            <label v-for="f in fields" :key="f.key" class="as-field">
              <span>{{ f.label }}</span>
              <input :value="draft.value.value[f.key]" class="as-input" :placeholder="f.placeholder" spellcheck="false" :aria-label="f.label" @input="set(f.key, ($event.target as HTMLInputElement).value)" />
            </label>
          </div>
          <p class="gb-hint">An empty key takes EPFL's default, shown in it.</p>

          <ConfigSaveBar
            section="uploads"
            :dirty="draft.dirty.value"
            :saving="draft.saving.value"
            :saved="draft.saved.value"
            :valid="true"
            :error="draft.saveError.value"
            :conflict="draft.conflict.value"
            :state="state"
            @save="draft.save()"
            @discard="reset"
            @reload="draft.load()"
            @overwrite="draft.save(true)"
          />
        </template>


      </div>

      <ConfigRefPanel :sections="['attachments']" title="the uploads table" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { getUploads, type UploadStatus } from '@/api';
import { SectionDraft, configModel, configModelError } from '@/configmodel';
import ConfigRefPanel from '@/components/configforms/ConfigRefPanel.vue';
import ConfigSaveBar from '@/components/configforms/ConfigSaveBar.vue';

type Uploads = { endpoint: string; public_url: string; prefix: string; region: string };

const modelError = configModelError;

const fields = [
  { key: 'endpoint', label: 'S3 endpoint', placeholder: 'empty: https://s3.epfl.ch' },
  { key: 'public_url', label: 'Public URL (the CDN)', placeholder: 'empty: https://enacit4r-cdn.epfl.ch' },
  { key: 'prefix', label: 'Bucket prefix', placeholder: 'empty: git-board/<repository>' },
  { key: 'region', label: 'Region', placeholder: 'empty: us-east-1' },
] as const;

const draft = new SectionDraft<Uploads>(
  'uploads',
  (m) => ({
    endpoint: m.uploads.endpoint ?? '',
    public_url: m.uploads.public_url ?? '',
    prefix: m.uploads.prefix ?? '',
    region: m.uploads.region ?? '',
  }),
  (a, b) => JSON.stringify(a) !== JSON.stringify(b),
  (value) => ({
    endpoint: value.endpoint || undefined,
    public_url: value.public_url || undefined,
    prefix: value.prefix || undefined,
    region: value.region || undefined,
  }),
);

function set(key: keyof Uploads, v: string) {
  if (draft.value.value) draft.value.value = { ...draft.value.value, [key]: v };
}

const state = computed(() => {
  const u = draft.value.value;
  const n = u ? [u.endpoint, u.public_url, u.prefix, u.region].filter(Boolean).length : 0;
  return n ? `${n} key${n === 1 ? '' : 's'} set` : 'no key set: the defaults';
});

function reset() {
  if (configModel.value) draft.reset(configModel.value);
}

const status = ref<UploadStatus | null>(null);

onMounted(() => {
  void draft.load();
  getUploads()
    .then((s) => (status.value = s))
    .catch(() => {}); // no status line without it
});
</script>

<style scoped>
.set-panel { display: grid; gap: .75rem; container-type: inline-size; }
/* the forms left, the reference right, like the old editor; one column when narrow */
.set-cols { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); gap: .75rem; align-items: start; }
.set-forms { display: grid; gap: .75rem; min-width: 0; }
@container (max-width: 52rem) { .set-cols { grid-template-columns: minmax(0, 1fr); } }
h2, p { margin: 0; }
.as-lead { max-width: 42rem; color: var(--fg-muted); }
.as-muted { color: var(--fg-muted); }
.as-grid { display: flex; gap: .75rem; flex-wrap: wrap; }
.as-field { display: grid; gap: .25rem; font-size: .8125rem; }
.as-field > span { color: var(--fg-muted); }
.as-input { min-width: 16rem; padding: .375rem .625rem; border: 1px solid var(--border); background: var(--bg); color: var(--fg); font: inherit; }
.as-input:focus { outline: 2px solid var(--fg); outline-offset: 2px; border-color: var(--fg); }
.as-status { margin: 0; padding: .5rem .75rem; border-left: 3px solid var(--border-strong); background: var(--bg-subtle); }
.as-on { border-left-color: var(--success); }
.as-off { border-left-color: var(--warning); }
.as-h { margin: .25rem 0 0; font-size: 1rem; }
</style>
