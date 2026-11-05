<!-- Settings › Numbers: the project's own numbers, GB-39
     (docs/guide/configuration.md, Numbers). Start them: a prefix from the
     repository's name, a start above the forges' numbers (POST
     /api/numbers {prefix, start}); then number the issues that have none
     ({}). Where they stand (GET /api/numbers) comes later (api.md, 37):
     `git board numbers` says it meanwhile. -->
<template>
  <div class="set-panel nb">
    <h2 class="h4">Numbers</h2>
    <p class="nb-lead">One number per issue, the same on every forge: <b>{{ prefixErr ? 'ABC' : prefix }}-{{ start || 123 }}</b>. The counter is kept on origin in git, or beside the board in PostgreSQL, where every writer reads it.</p>
    <p v-if="datastore && !datastore.counter" class="epfl-alert epfl-alert-info" role="note">The board is kept in {{ datastore.name }}, which hands out no project numbers: the issues go by the forges' numbers. <router-link to="/settings/datastore">Datastore</router-link></p>
    <p v-if="error" class="epfl-alert epfl-alert-danger" role="alert">{{ error }}</p>
    <p v-if="result" class="epfl-alert epfl-alert-success" role="status">{{ result }}</p>

    <div class="nb-setup">
      <form class="nb-form" @submit.prevent="confirming = true">
        <div>
          <label class="nb-label" for="nb-prefix">Prefix</label>
          <input id="nb-prefix" v-model.trim="prefix" class="epfl-input nb-prefix" autocomplete="off" :aria-invalid="!!prefixErr" />
          <p class="gb-hint" :class="{ 'nb-bad': prefixErr }">{{ prefixErr || 'From the repository’s name: its initials.' }}</p>
        </div>
        <div>
          <label class="nb-label" for="nb-start">Start</label>
          <input id="nb-start" v-model.number="start" class="epfl-input nb-start" type="number" min="1" />
          <p class="gb-hint">Above every number an issue has on a forge: below it, an issue keeps its forge number (#41 is {{ prefixErr ? 'ABC' : prefix }}-41).</p>
        </div>
        <div v-if="confirming" class="nb-confirm" role="alert">
          <p><b>This starts the counter at {{ prefix }}-{{ start }}</b>, on origin (a push of refs/board/seq) or in PostgreSQL. Nothing else goes anywhere.</p>
          <div class="nb-actions">
            <button type="button" class="epfl-btn epfl-btn-primary" :disabled="busy" @click="begin">{{ busy ? 'Starting…' : 'Start numbering' }}</button>
            <button type="button" class="epfl-btn epfl-btn-secondary" :disabled="busy" @click="confirming = false">Cancel</button>
          </div>
        </div>
        <button v-else type="submit" class="epfl-btn epfl-btn-primary nb-start-btn" :disabled="!!prefixErr || !start || datastore?.counter === false">Start numbering…</button>
      </form>
      <aside class="nb-help">
        <h3 class="nb-help-title">Once started</h3>
        <ul>
          <li>Every issue made here, or imported later, takes the next number when it reaches origin.</li>
          <li>The issues origin has without one take theirs at once: <button type="button" class="gb-linkbtn" :disabled="busy || datastore?.counter === false" @click="numberAll">Number them now</button>.</li>
          <li>In a terminal: <code>git board numbers</code>.</li>
        </ul>
      </aside>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { message, numbers } from '@/api';
import { board } from '@/me';
import { datastore, loadDatastore } from '@/kept';
import { prefixProblem, suggestPrefix } from '@/numbers';

const prefix = ref('');
const start = ref(1);
const busy = ref(false);
const error = ref('');
const result = ref('');
const confirming = ref(false);
const prefixErr = computed(() => prefixProblem(prefix.value));

async function act(fn: () => Promise<string>) {
  busy.value = true;
  error.value = '';
  result.value = '';
  try {
    result.value = await fn();
    confirming.value = false;
  } catch (e) {
    error.value = message(e);
  } finally {
    busy.value = false;
  }
}
const begin = () => act(async () => {
  const r = await numbers({ prefix: prefix.value, start: start.value });
  return `Numbering started at ${prefix.value}-${start.value}${r.where ? `, the counter ${r.where}` : ''}.`;
});
const numberAll = () => act(async () => {
  const r = await numbers();
  return r.numbered ? `${r.numbered} issue${r.numbered === 1 ? '' : 's'} numbered.` : 'No issue needed a number.';
});

onMounted(() => {
  void loadDatastore();
  prefix.value = suggestPrefix(board.value?.dir.split('/').filter(Boolean).at(-1) ?? '');
});
</script>

<style scoped>
.set-panel { display: flex; flex-direction: column; gap: 1.25rem; max-width: 64rem; }
.nb h2, .nb h3, .nb p { margin: 0; }
.nb-lead { max-width: 40rem; color: var(--fg-muted); }
.nb-muted { color: var(--fg-muted); }
.nb-tiles { display: grid; grid-template-columns: repeat(auto-fit, minmax(9rem, 1fr)); gap: .75rem; max-width: 45rem; }
.nb-wait { display: grid; gap: .75rem; justify-items: start; }
.nb-setup { display: grid; grid-template-columns: minmax(0, 1fr) minmax(16rem, 22rem); gap: 2rem; align-items: start; }
.nb-form { display: flex; flex-direction: column; gap: 1.25rem; min-width: 0; }
.nb-label { display: block; margin-bottom: .375rem; font-weight: 700; }
.nb-prefix { max-width: 10rem; }
.nb-start { max-width: 10rem; }
.nb-bad { color: var(--danger-fg); }
.nb-preview { padding: .75rem 1rem; background: var(--bg-subtle); border-left: 3px solid var(--border-strong); }
.nb-confirm { display: grid; gap: .75rem; padding: .875rem 1rem; background: var(--warning-bg); }
.nb-actions { display: flex; flex-wrap: wrap; gap: .5rem; }
.nb-start-btn { align-self: flex-start; }
.nb-help { display: flex; flex-direction: column; gap: .75rem; padding: 1.25rem 1.5rem; background: var(--bg-subtle); font-size: .875rem; }
.nb-help-title { font-size: 1.0625rem; font-weight: 700; }
.nb-help ul { display: grid; gap: .625rem; margin: 0; padding-left: 1.125rem; }
.nb-help code, .nb code { font-size: .8125rem; overflow-wrap: anywhere; }
@media (max-width: 59.99rem) {
  .nb-setup { grid-template-columns: minmax(0, 1fr); }
}
</style>
