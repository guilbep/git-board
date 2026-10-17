<!-- How forges work: docs/forges.md whole (GET /api/forges/docs), the
     same text readers find on GitHub. -->
<template>
  <q-dialog v-model="open" @show="load">
    <section class="fg" aria-labelledby="fg-title">
      <header class="fg-head">
        <h2 id="fg-title" class="h5">How forges work</h2>
        <button type="button" class="gb-icon-btn" aria-label="Close" @click="open = false"><GbIcon name="close" /></button>
      </header>
      <p v-if="error" class="epfl-alert epfl-alert-danger" role="alert">{{ error }}</p>
      <div v-for="d in docs" :key="d.id" class="fg-sec">
        <h3 class="fg-sec-title">{{ d.title }}</h3>
        <div class="gb-md" v-html="d.html" />
      </div>
    </section>
  </q-dialog>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import { getForgeDocs, type DocSection } from '@/api/board';
import { message } from '@/issue';
import GbIcon from '@/components/GbIcon.vue';

const open = defineModel<boolean>({ required: true });
const docs = ref<DocSection[]>([]);
const error = ref('');

async function load() {
  if (docs.value.length) return;
  try {
    docs.value = await getForgeDocs();
  } catch (e) {
    error.value = message(e);
  }
}
</script>

<style scoped>
.fg { width: min(46rem, calc(100vw - 2rem)); max-height: calc(100vh - 2rem); overflow: auto; padding: 1.25rem 2rem 2rem; background: var(--bg); }
.fg-head { position: sticky; top: -1.25rem; display: flex; align-items: center; justify-content: space-between; margin: -1.25rem -2rem 0; padding: .75rem 1rem .75rem 2rem; background: var(--bg); border-bottom: 1px solid var(--border-subtle); }
.fg-head h2 { margin: 0; }
.fg-sec { margin-top: 1.5rem; }
.fg-sec-title { margin: 0 0 .5rem; font-size: 1.125rem; font-weight: 700; }
.fg .gb-md { font-size: 1rem; line-height: 1.6; }
@media (max-width: 47.99rem) {
  .fg { padding: 1rem 1rem 1.5rem; }
  .fg-head { top: -1rem; margin: -1rem -1rem 0; padding-left: 1rem; }
}
</style>
