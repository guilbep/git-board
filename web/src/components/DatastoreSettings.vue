<!-- Settings › Datastore: where the board is kept (ADR 0002,
     docs/guide/datastores.md). board.datastore in this clone's git config:
     git, a postgres:// URL, or forge. A URL can be tested, and the board in
     git copied into it, before it is saved; a save takes effect when git
     board serve starts again. -->
<template>
  <div class="set-panel ds">
    <h2 class="h4">Datastore</h2>
    <p class="ds-lead">Where the board's issues, people and sync records are kept. Your code stays in git whatever you choose.</p>
    <p v-if="error" class="epfl-alert epfl-alert-danger" role="alert">{{ error }}</p>
    <p v-if="!d && !error" class="ds-muted">Loading…</p>

    <template v-if="d">
      <p class="ds-status" role="status">
        Kept in <b>{{ d.datastore }}</b><template v-if="d.where && d.where !== d.datastore">, {{ d.where.replace(d.datastore + ' ', '') }}</template>.
        <template v-if="d.forge_only"> Each write goes to the forges at once.</template>
      </p>
      <p v-if="d.restart" class="epfl-alert epfl-alert-warning" role="alert">
        <b>Saved: {{ d.setting }}.</b> Restart <code>git board serve</code> to use it; until then the board is still kept in {{ d.datastore }}.
      </p>
      <p v-if="d.env" class="epfl-alert epfl-alert-info" role="note">
        <code>GIT_BOARD_DATASTORE</code> sets it where git-board starts, over git config: change it there.
      </p>

      <div class="set-cols">
        <form class="ds-form" @submit.prevent="save">
          <fieldset class="ds-kinds" :disabled="d.env || busy !== ''">
            <legend class="ds-label">Keep the board in</legend>
            <label v-for="k in kinds" :key="k.kind" class="ds-kind" :class="{ 'ds-kind-on': kind === k.kind }">
              <input v-model="kind" type="radio" name="ds-kind" :value="k.kind" />
              <span>
                <b>{{ k.label }}</b><span v-if="openKind === k.kind" class="ds-now"> · now</span><span v-else-if="d.restart && kindOf(d.setting) === k.kind" class="ds-now"> · saved, after a restart</span>
                <span class="ds-gives">{{ k.gives }}</span>
                <span class="ds-gives ds-gives-up">Gives up: {{ k.givesUp }}</span>
              </span>
            </label>
          </fieldset>

          <div v-if="kind === 'postgres'" class="ds-url">
            <label class="ds-label" for="ds-url">PostgreSQL URL</label>
            <input
              id="ds-url"
              v-model="url"
              class="epfl-input"
              placeholder="postgres://user:password@host:5432/database"
              autocomplete="off" data-1p-ignore data-lpignore="true" spellcheck="false"
              :disabled="d.env || busy !== ''"
              :aria-invalid="!!urlErr && url !== ''"
              aria-describedby="ds-url-hint"
            />
            <p id="ds-url-hint" class="gb-hint">
              {{ urlErr && url ? urlErr : 'Kept in this clone’s git config, never in .git-board.toml: it may hold a password, which this page never shows again.' }}
              The tables go in the schemas <code>git_board</code> and <code>git_board_index</code>, made on first use.
            </p>
            <div class="ds-actions">
              <button type="button" class="epfl-btn epfl-btn-secondary" :disabled="!!urlErr || busy !== ''" @click="test">{{ busy === 'test' ? 'Testing…' : 'Test connection' }}</button>
              <button
                type="button"
                class="epfl-btn epfl-btn-secondary"
                :disabled="!!urlErr || busy !== '' || !d.replica"
                :title="d.replica ? 'Copies every issue, person and record kept in git; what the database has already is left alone' : 'The board isn’t kept in git here: there is nothing in git to copy'"
                @click="copy"
              >{{ busy === 'copy' ? 'Copying…' : 'Copy the board from git' }}</button>
            </div>
            <p v-if="tested" class="epfl-alert" :class="tested.ok ? 'epfl-alert-success' : 'epfl-alert-danger'" role="status">
              {{ tested.line }}
            </p>
          </div>

          <p v-if="kind === 'forge'" class="gb-hint">
            Needs a forge with its sync on (<router-link to="/settings/sync">Sync</router-link>). Writes from the command line go with the next sync;
            <code>git board datastore drop</code> throws the copy away, and the next sync pulls it back.
          </p>

          <div class="ds-actions">
            <button type="submit" class="epfl-btn epfl-btn-primary" :disabled="d.env || !changed || (kind === 'postgres' && !!urlErr) || busy !== ''">{{ busy === 'save' ? 'Saving…' : 'Save' }}</button>
            <span v-if="!changed" class="ds-muted">Nothing to save.</span>
          </div>
        </form>

        <aside class="ds-help" aria-labelledby="ds-help-title">
          <h3 id="ds-help-title" class="ds-help-title">What {{ d.datastore }} can do</h3>
          <ul class="ds-caps">
            <li :class="d.replica ? 'ds-yes' : 'ds-no'"><b>{{ d.replica ? 'Yes' : 'No' }}</b> pull, push and start over with git remotes</li>
            <li :class="d.counter ? 'ds-yes' : 'ds-no'"><b>{{ d.counter ? 'Yes' : 'No' }}</b> project numbers (<router-link to="/settings/numbers">Numbers</router-link>)</li>
            <li class="ds-yes"><b>Yes</b> forge sync, the same ids everywhere</li>
          </ul>
          <p>A board moves out of git by copying it: every id stays, so the markers sync left on the forges stay true. The git refs aren’t touched.</p>
          <p>From a terminal: <code>git config board.datastore postgres://…</code>, <code>git board datastore copy</code>, <code>git board datastore</code>.</p>
        </aside>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { copyToDatastore, getDatastore, setDatastore, testDatastore, type Datastore, type DatastoreTest } from '@/api/board';
import { message } from '@/issue';
import { kindOf, kinds, urlProblem, valueOf, type StoreKind } from '@/datastore';

// changed: the datastore as saved, for the menu's line
const emit = defineEmits<{ changed: [d: Datastore] }>();
const d = ref<Datastore | null>(null);
const kind = ref<StoreKind>('git');
const url = ref('');
const busy = ref<'' | 'test' | 'copy' | 'save'>('');
const error = ref('');
const tested = ref<DatastoreTest | null>(null);

const urlErr = computed(() => urlProblem(url.value));
// the kind the server opened with, which a save changes only at a restart
const openKind = computed<StoreKind>(() => (!d.value ? 'git' : d.value.forge_only ? 'forge' : d.value.replica ? 'git' : 'postgres'));
const changed = computed(() => !!d.value && valueOf(kind.value, url.value) !== d.value.setting);

function take(fresh: Datastore) {
  d.value = fresh;
  kind.value = kindOf(fresh.setting);
  url.value = kind.value === 'postgres' ? fresh.setting : '';
  emit('changed', fresh);
}

async function act(what: typeof busy.value, f: () => Promise<void>) {
  busy.value = what;
  error.value = '';
  try {
    await f();
  } catch (e) {
    error.value = message(e);
  } finally {
    busy.value = '';
  }
}

const test = () => act('test', async () => void (tested.value = await testDatastore(url.value)));
const copy = () => act('copy', async () => void (tested.value = await copyToDatastore(url.value)));
const save = () => act('save', async () => take(await setDatastore(valueOf(kind.value, url.value))));

onMounted(() => act('', async () => take(await getDatastore())));
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
