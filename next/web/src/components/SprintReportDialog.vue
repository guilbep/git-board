<!-- The sprint report (report.go): Markdown ready for Slack, an email or an
     issue, shown rendered or as text, copied or downloaded in one click. -->
<template>
  <q-dialog v-model="open" @show="load">
    <section class="srep" aria-labelledby="srep-title">
      <header class="srep-head">
        <h2 id="srep-title" class="h5">Sprint report<span v-if="report" class="gb-hint"> · {{ report.title }}</span></h2>
        <button type="button" class="gb-icon-btn" aria-label="Close" @click="open = false"><GbIcon name="close" /></button>
      </header>
      <div class="gb-tabs" role="tablist">
        <button type="button" class="gb-tab" role="tab" :aria-selected="tab === 'preview'" @click="tab = 'preview'">Preview</button>
        <button type="button" class="gb-tab" role="tab" :aria-selected="tab === 'markdown'" @click="tab = 'markdown'">Markdown</button>
      </div>
      <p v-if="error" class="epfl-alert epfl-alert-danger" role="alert">{{ error }}</p>
      <p v-else-if="!report" class="gb-hint" role="status">Writing the report…</p>
      <div v-else-if="tab === 'preview'" class="srep-body gb-md" v-html="html" />
      <textarea v-else ref="raw" class="srep-body srep-raw" readonly :value="report.markdown" aria-label="The report in Markdown" />
      <footer v-if="report" class="srep-actions">
        <button type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" @click="copy">{{ copied ? 'Copied' : 'Copy Markdown' }}</button>
        <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" @click="download">Download .md</button>
        <span class="gb-hint">Paste it in Slack, an email or an issue.</span>
      </footer>
    </section>
  </q-dialog>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import { getReport, renderMarkdown, type SprintReport } from '@/api/board';
import { message } from '@/issue';
import GbIcon from '@/components/GbIcon.vue';

const props = defineProps<{ sprint?: string }>();
const open = defineModel<boolean>({ required: true });

const report = ref<SprintReport | null>(null);
const html = ref('');
const error = ref('');
const tab = ref<'preview' | 'markdown'>('preview');
const copied = ref(false);
const raw = ref<HTMLTextAreaElement | null>(null);

async function load() {
  report.value = null;
  error.value = '';
  try {
    report.value = await getReport(props.sprint);
    html.value = (await renderMarkdown(report.value.markdown)).html;
  } catch (e) {
    error.value = message(e);
  }
}

async function copy() {
  if (!report.value) return;
  try {
    await navigator.clipboard.writeText(report.value.markdown);
    copied.value = true;
    setTimeout(() => (copied.value = false), 2000);
  } catch {
    tab.value = 'markdown'; // no clipboard here: select the text instead
    setTimeout(() => raw.value?.select(), 0);
  }
}

function download() {
  if (!report.value) return;
  const a = document.createElement('a');
  a.href = URL.createObjectURL(new Blob([report.value.markdown], { type: 'text/markdown' }));
  a.download = `sprint-report-${report.value.sprint}.md`;
  a.click();
  setTimeout(() => URL.revokeObjectURL(a.href), 1000);
}
</script>

<style scoped>
.srep { display: grid; gap: .75rem; width: min(52rem, calc(100vw - 2rem)); max-height: calc(100vh - 4rem); padding: 1.25rem 1.5rem 1.5rem; background: var(--bg); overflow: hidden; grid-template-rows: auto auto minmax(0, 1fr) auto; }
.srep-head { display: flex; align-items: center; justify-content: space-between; }
.srep-head h2 { margin: 0; }
.srep-body { min-height: 16rem; max-height: 60vh; overflow: auto; padding: .75rem 1rem; border: 1px solid var(--border); }
.srep-raw { width: 100%; font-family: var(--font-mono); font-size: .8125rem; line-height: 1.55; resize: none; background: var(--bg-subtle); color: var(--fg); }
.srep-actions { display: flex; align-items: center; gap: .75rem; flex-wrap: wrap; }
</style>
