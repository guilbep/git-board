<!-- The reference, per tab: the sections of docs/guide/configuration.md the
     tab owns, in a side column like the old editor's — the forms left, the
     reference right, scrolling on its own. The server serves the doc
     sectioned (GET /api/config/docs); the panel fetches once per page. -->
<template>
  <aside class="crp">
    <div class="crp-head">
      <h3 class="crp-title">Reference</h3>
      <span class="crp-hint">{{ title }}</span>
    </div>
    <nav v-if="mine.length > 1" class="crp-nav" aria-label="Reference sections">
      <button
        v-for="d in mine"
        :key="d.id"
        type="button"
        class="crp-tab"
        :aria-current="d.id === id ? 'true' : undefined"
        @click="id = d.id"
      >
        {{ d.title }}
      </button>
    </nav>
    <div v-if="doc" ref="bodyEl" class="gb-md crp-doc" v-html="doc.html" />
  </aside>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue';
import { getConfigDocs, type DocSection } from '@/api/board';
import { tomlTokens } from '@/toml';

const props = defineProps<{
  sections: string[]; // the doc headings' ids the tab owns, in display order
  title?: string; // what the head's hint says, the first section's title when empty
}>();

const docs = ref<DocSection[]>([]);
const id = ref(props.sections[0] ?? '');

// the tab's sections, in the order the tab named them
const mine = computed(() => props.sections.flatMap((s) => docs.value.filter((d) => d.id === s)));
const doc = computed(() => mine.value.find((d) => d.id === id.value) ?? mine.value[0]);

onMounted(() => {
  getConfigDocs().then(
    (d) => (docs.value = d),
    () => (docs.value = []),
  );
});

// the TOML examples colored, the tables labelled per column when narrow —
// the old editor's enhanceDoc, by the rendered HTML the server serves
function enhance(el: HTMLElement) {
  for (const code of el.querySelectorAll<HTMLElement>('pre > code.language-toml')) {
    if (code.dataset.done) continue;
    code.dataset.done = '1';
    const lines = (code.textContent ?? '').replace(/\n$/, '').split('\n');
    code.replaceChildren(
      ...lines.flatMap((line, i) => [
        ...tomlTokens(line).map((t) => {
          const span = document.createElement('span');
          if (t.cls) span.className = `tk-${t.cls}`;
          span.textContent = t.text;
          return span;
        }),
        ...(i < lines.length - 1 ? [document.createTextNode('\n')] : []),
      ]),
    );
  }
  for (const table of el.querySelectorAll<HTMLTableElement>('table')) {
    if (table.dataset.done) continue;
    table.dataset.done = '1';
    const heads = [...table.querySelectorAll('thead th')].map((th) => th.textContent ?? '');
    for (const row of table.querySelectorAll('tbody tr')) {
      [...row.children].forEach((td, i) => ((td as HTMLElement).dataset.label = heads[i] ?? ''));
    }
  }
}

const bodyEl = ref<HTMLElement | null>(null);
watch(
  doc,
  () => void nextTick(() => {
    if (bodyEl.value) enhance(bodyEl.value);
  }),
  { flush: 'post' },
);
</script>

<style scoped>
.crp { container-type: inline-size; display: grid; align-content: start; gap: .75rem; height: max(24rem, calc(100vh - 13rem)); min-height: 0; padding: 1rem 1.25rem; border: 1px solid var(--border); background: var(--bg-subtle); overflow: auto; }
.crp-head { display: flex; align-items: baseline; gap: .5rem; }
.crp-title { margin: 0; font-size: .8125rem; font-weight: 700; text-transform: uppercase; letter-spacing: .06em; color: var(--primary); }
.crp-hint { color: var(--fg-muted); }
.crp-nav { display: flex; flex-wrap: wrap; gap: .25rem; }
.crp-tab { padding: .125rem .5rem; border: 1px solid var(--border); background: var(--bg); color: var(--fg); font: inherit; font-size: .75rem; cursor: pointer; }
.crp-tab:hover { border-color: var(--fg); }
.crp-tab[aria-current] { border-color: var(--fg); background: var(--fg); color: var(--bg); font-weight: 700; }
.crp-doc { font-size: .9375rem; }
.crp-doc :deep(table) { width: 100%; border-collapse: collapse; font-size: .875rem; }
.crp-doc :deep(th), .crp-doc :deep(td) { padding: .375rem .5rem; border: 0; border-bottom: 1px solid var(--border); text-align: left; vertical-align: top; }
.crp-doc :deep(th) { font-size: .8125rem; color: var(--fg-muted); }
.crp-doc :deep(td:first-child code) { white-space: nowrap; }
.crp-doc :deep(pre) { padding: .625rem .875rem; background: var(--bg); border: 1px solid var(--border); white-space: pre-wrap; overflow-wrap: anywhere; font-size: .8125rem; color: var(--fg); }
.crp-doc :deep(pre code) { color: inherit; }
/* a narrow reference: each table row a card, its cells labelled */
@container (max-width: 36rem) {
  .crp-doc :deep(thead) { display: none; }
  .crp-doc :deep(table), .crp-doc :deep(tbody), .crp-doc :deep(tr), .crp-doc :deep(td) { display: block; }
  .crp-doc :deep(tr) { padding: .625rem .75rem; margin-top: .5rem; border: 1px solid var(--border); background: var(--bg); }
  .crp-doc :deep(td) { padding: 0; border: 0; }
  .crp-doc :deep(td:first-child) { font-weight: 700; }
  .crp-doc :deep(td + td:not(:last-child)) { display: inline-block; margin: .25rem .5rem 0 0; font-size: .8125rem; color: var(--fg-muted); }
  .crp-doc :deep(td + td:not(:last-child))::before { content: attr(data-label) ': '; }
}
</style>
