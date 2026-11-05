<!-- Settings › Datastore: where the board is kept (ADR 0002,
     docs/guide/datastores.md): GET /api/datastore. Another one is chosen
     with board.datastore in this clone's git config, from a terminal (PUT
     /api/datastore comes later: docs/rewrite/api.md, 28); the board in
     git can be copied into it from here first (POST /api/datastore/copy),
     every id kept. -->
<template>
  <div class="set-panel ds">
    <h2 class="h4">Datastore</h2>
    <p class="ds-lead">Where the board's issues, people and sync records are kept. Your code stays in git whatever you choose.</p>
    <p v-if="error" class="epfl-alert epfl-alert-danger" role="alert">{{ error }}</p>
    <p v-if="!d && !error" class="ds-muted">Loading…</p>

    <template v-if="d">
      <p class="ds-status" role="status">Kept in <b>{{ d.name }}</b><template v-if="d.where && d.where !== d.name">, {{ d.where }}</template>.</p>

      <div class="set-cols">
        <form class="ds-form" @submit.prevent>
          <fieldset class="ds-kinds" :disabled="busy">
            <legend class="ds-label">Keep the board in</legend>
            <label v-for="k in kinds" :key="k.kind" class="ds-kind" :class="{ 'ds-kind-on': kind === k.kind }">
              <input v-model="kind" type="radio" name="ds-kind" :value="k.kind" />
              <span>
                <b>{{ k.label }}</b><span v-if="openKind === k.kind" class="ds-now"> · now</span>
                <span class="ds-gives">{{ k.gives }}</span>
                <span class="ds-gives ds-gives-up">Gives up: {{ k.givesUp }}</span>
              </span>
            </label>
          </fieldset>

          <div v-if="kind === 'postgres'" class="ds-url">
            <label class="ds-label" for="ds-url">PostgreSQL URL</label>
            <input id="ds-url" v-model="url" class="epfl-input" placeholder="postgres://user:password@host:5432/database" autocomplete="off" data-1p-ignore data-lpignore="true" spellcheck="false" :disabled="busy" :aria-invalid="!!urlErr && url !== ''" aria-describedby="ds-url-hint" />
            <p id="ds-url-hint" class="gb-hint">{{ urlErr && url ? urlErr : 'Kept in this clone’s git config, never in .git-board.toml: it may hold a password.' }}</p>
          </div>
          <div v-if="kind === 'markdown'" class="ds-url">
            <label class="ds-label" for="ds-dir">Folder, from the checkout's top</label>
            <input id="ds-dir" v-model="dir" class="epfl-input" placeholder="issues" autocomplete="off" spellcheck="false" :disabled="busy" :aria-invalid="!!dirErr && dir !== ''" aria-describedby="ds-dir-hint" />
            <p id="ds-dir-hint" class="gb-hint">{{ dirErr && dir ? dirErr : 'One NNN-slug.md per issue, rendered after each write. Commit the folder with the code.' }}</p>
          </div>
          <p v-if="kind === 'forge'" class="gb-hint">Needs a forge with its sync on (<router-link to="/settings/sync">Sync</router-link>).</p>

          <template v-if="kind !== openKind && valid">
            <div class="ds-actions">
              <button type="button" class="epfl-btn epfl-btn-secondary" :disabled="busy || !d.replica || kind === 'forge'" :title="d.replica ? 'Copies every issue, person and record kept in git; what is there already is left alone' : 'The board isn’t kept in git here: there is nothing in git to copy'" @click="copy">{{ busy ? 'Copying…' : 'Copy the board from git' }}</button>
            </div>
            <p v-if="copied" class="epfl-alert epfl-alert-success" role="status">{{ copied }}</p>
            <p class="gb-hint">Then, in a terminal: <code>git config board.datastore {{ value }}</code>, and start <code>git board serve</code> again.</p>
          </template>
        </form>

        <aside class="ds-help" aria-labelledby="ds-help-title">
          <h3 id="ds-help-title" class="ds-help-title">What {{ d.name }} can do</h3>
          <ul class="ds-caps">
            <li :class="d.replica ? 'ds-yes' : 'ds-no'"><b>{{ d.replica ? 'Yes' : 'No' }}</b> pull and push with git remotes</li>
            <li :class="d.counter ? 'ds-yes' : 'ds-no'"><b>{{ d.counter ? 'Yes' : 'No' }}</b> project numbers (<router-link to="/settings/numbers">Numbers</router-link>)</li>
            <li class="ds-yes"><b>Yes</b> forge sync, the same ids everywhere</li>
          </ul>
          <p>A board moves out of git by copying it: every id stays, so the markers sync left on the forges stay true. The git refs aren’t touched.</p>
        </aside>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { copyDatastore, message } from '@/api';
import { folderProblem, kinds, urlProblem, valueOf, type StoreKind } from '@/datastore';
import { datastore, loadDatastore } from '@/kept';

const d = datastore;
const kind = ref<StoreKind>('git');
const url = ref('');
const dir = ref('');
const busy = ref(false);
const error = ref('');
const copied = ref('');

// the kind the board is kept in now, from its name
const openKind = computed<StoreKind>(() => ({ PostgreSQL: 'postgres', 'Markdown files': 'markdown', forge: 'forge' })[d.value?.name ?? ''] as StoreKind ?? 'git');
const urlErr = computed(() => urlProblem(url.value));
const dirErr = computed(() => folderProblem(dir.value));
const valid = computed(() => (kind.value === 'postgres' ? !urlErr.value : kind.value === 'markdown' ? !dirErr.value : true));
const value = computed(() => valueOf(kind.value, kind.value === 'markdown' ? dir.value : url.value));

async function copy() {
  busy.value = true;
  error.value = '';
  copied.value = '';
  try {
    const c = await copyDatastore('', value.value);
    copied.value = `Copied ${c.entities.toLocaleString()} entities to ${c.to}${c.counter ? ', and the numbers’ counter' : ''}.`;
  } catch (e) {
    error.value = message(e);
  } finally {
    busy.value = false;
  }
}

onMounted(async () => {
  await loadDatastore();
  kind.value = openKind.value;
});
</script>

<style scoped>
.set-panel { display: flex; flex-direction: column; gap: 1rem; max-width: 64rem; container-type: inline-size; }
.ds h2, .ds h3, .ds p { margin: 0; }
.ds-lead { max-width: 42rem; color: var(--fg-muted); }
.ds-muted { color: var(--fg-muted); }
.ds-status { padding: .5rem .75rem; border-left: 3px solid var(--success); background: var(--bg-subtle); }
.set-cols { display: grid; grid-template-columns: minmax(0, 3fr) minmax(0, 2fr); gap: 1.25rem; align-items: start; }
@container (max-width: 52rem) { .set-cols { grid-template-columns: minmax(0, 1fr); } }
.ds-form { display: grid; gap: 1rem; min-width: 0; }
.ds-kinds { display: grid; gap: .5rem; margin: 0; padding: 0; border: 0; }
.ds-label { font-weight: 700; font-size: .875rem; margin-bottom: .25rem; }
.ds-kind { display: grid; grid-template-columns: auto 1fr; gap: .5rem; padding: .625rem .75rem; border: 1px solid var(--border); cursor: pointer; }
.ds-kind-on { border-color: var(--fg); }
.ds-kind input { margin-top: .25rem; }
.ds-now { color: var(--fg-muted); font-weight: 400; }
.ds-gives { display: block; font-size: .8125rem; color: var(--fg-muted); }
.ds-gives-up { margin-top: .125rem; }
.ds-url { display: grid; gap: .5rem; }
.ds-url .epfl-input { width: 100%; font-family: var(--font-mono, monospace); }
.ds-actions { display: flex; gap: .5rem; flex-wrap: wrap; align-items: center; }
.ds-help { display: grid; gap: .5rem; padding: .75rem; background: var(--bg-subtle); font-size: .875rem; }
.ds-help-title { font-size: 1rem; }
.ds-caps { margin: 0; padding: 0; list-style: none; display: grid; gap: .25rem; }
.ds-yes b { color: var(--success); }
.ds-no b { color: var(--fg-muted); }
</style>
