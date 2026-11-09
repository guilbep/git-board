<!-- Settings › Datastore: where the board is kept (ADR 0002,
     docs/guide/datastores.md): GET /api/datastore. Another one is chosen
     from here (PUT /api/datastore, api.md, 28): the clone's
     board.datastore is written, the board opens again, no terminal; the
     board in git is copied into it from here first (POST
     /api/datastore/copy), every id kept. -->
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
          <fieldset class="ds-kinds" :disabled="busy !== ''">
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
            <input id="ds-url" v-model="url" class="epfl-input" placeholder="postgres://user:password@host:5432/database" autocomplete="off" data-1p-ignore data-lpignore="true" spellcheck="false" :disabled="busy !== ''" :aria-invalid="!!urlErr && url !== ''" aria-describedby="ds-url-hint" />
            <p id="ds-url-hint" class="gb-hint">{{ urlErr && url ? urlErr : 'Kept in this clone’s git config, never in .git-board.toml: it may hold a password.' }}</p>
          </div>
          <div v-if="kind === 'markdown'" class="ds-url">
            <label class="ds-label" for="ds-dir">Folder, from the checkout's top, or a full path</label>
            <input id="ds-dir" v-model="dir" class="epfl-input" placeholder="issues" autocomplete="off" spellcheck="false" :disabled="busy !== ''" :aria-invalid="!!dirErr && dir !== ''" aria-describedby="ds-dir-hint" />
            <p id="ds-dir-hint" class="gb-hint">{{ dirErr && dir ? dirErr : 'One NNN-slug.md per issue, rendered after each write. Commit the folder with the code, or give the full path of another repository to keep the issues there.' }}</p>
          </div>
          <p v-if="kind === 'forge'" class="gb-hint">Needs a forge with its sync on (<router-link to="/settings/sync">Sync</router-link>).</p>

          <template v-if="kind !== openKind && valid">
            <div class="ds-actions">
              <button
                v-if="d.replica && kind !== 'forge'"
                type="button"
                class="epfl-btn epfl-btn-secondary"
                :disabled="busy !== ''"
                title="Copies every issue, person and record kept in git; what is there already is left alone"
                @click="copy"
              >{{ busy === 'copy' ? 'Copying…' : 'Copy the board from git' }}</button>
              <button type="button" class="epfl-btn epfl-btn-primary" :disabled="busy !== ''" @click="keep">{{ busy === 'keep' ? 'Switching…' : 'Keep the board here' }}</button>
            </div>
            <p v-if="copied" class="epfl-alert epfl-alert-success" role="status">{{ copied }}</p>
            <p class="gb-hint">Keeping the board here copies it from git first when it can, then this clone reads it from {{ kinds.find((k) => k.kind === kind)?.label }} at once: no restart, nothing in a terminal.</p>
          </template>
        </form>

        <aside class="ds-help" aria-labelledby="ds-help-title">
          <h3 id="ds-help-title" class="ds-help-title">What {{ d.name }} can do</h3>
          <ul class="ds-caps">
            <li :class="d.replica ? 'ds-yes' : 'ds-no'"><b>{{ d.replica ? 'Yes' : 'No' }}</b> pull and push with git remotes</li>
            <li :class="d.counter ? 'ds-yes' : 'ds-no'"><b>{{ d.counter ? 'Yes' : 'No' }}</b> project numbers (<router-link to="/settings/numbers">Numbers</router-link>)</li>
            <li class="ds-yes"><b>Yes</b> forge sync, the same ids everywhere</li>
          </ul>
          <p><a class="epfl-btn epfl-btn-secondary epfl-btn-sm" :href="markdownExportURL()" download data-test="md-export">Download the issues as Markdown</a></p>
          <p class="gb-hint">One NNN-slug.md per issue, in a zip: a copy, which the board does not read back.</p>
          <p>A board moves out of git by copying it: every id stays, so the markers sync left on the forges stay true. The git refs aren’t touched.</p>
        </aside>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { copyDatastore, markdownExportURL, message } from '@/api';
import { setDatastore } from '@/api/sync';
import { folderProblem, kinds, urlProblem, valueOf, type StoreKind } from '@/datastore';
import { datastore, loadDatastore } from '@/kept';

const d = datastore;
const kind = ref<StoreKind>('git');
const url = ref('');
const dir = ref('');
const busy = ref('');
const error = ref('');
const copied = ref('');

// the kind the board is kept in now, from its name
const openKind = computed<StoreKind>(() => ({ PostgreSQL: 'postgres', 'Markdown files': 'markdown', 'the forges alone': 'forge' })[d.value?.name ?? ''] as StoreKind ?? 'git');
const urlErr = computed(() => urlProblem(url.value));
const dirErr = computed(() => folderProblem(dir.value));
const valid = computed(() => (kind.value === 'postgres' ? !urlErr.value : kind.value === 'markdown' ? !dirErr.value : true));
const value = computed(() => valueOf(kind.value, kind.value === 'markdown' ? dir.value : url.value));

async function copy() {
  busy.value = 'copy';
  error.value = '';
  copied.value = '';
  try {
    const c = await copyDatastore('', value.value);
    copied.value = `Copied ${c.entities.toLocaleString()} entities to ${c.to}${c.counter ? ', and the numbers’ counter' : ''}.`;
  } catch (e) {
    error.value = message(e);
  } finally {
    busy.value = '';
  }
}

// keep switches this clone's board.datastore, from here: the board in
// git is copied in first, so nothing is left behind (what is there
// already is left alone, so a second run copies nothing)
async function keep() {
  busy.value = 'keep';
  error.value = '';
  copied.value = '';
  try {
    if (d.value?.replica && kind.value !== 'forge') await copyDatastore('', value.value);
    const k = await setDatastore(value.value);
    copied.value = `The board is kept in ${k.name} from now on.`;
    await loadDatastore();
  } catch (e) {
    error.value = message(e);
  } finally {
    busy.value = '';
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
