<!-- Add a forge no git remote names, in two steps: the forge, GitHub or
     GitLab; the project (its address, a short name, a token, its way, sync
     or not). POST /api/forges writes its [forges.<name>] table and gives
     the token to git's credential helper, never the file (ADR 0010). -->
<template>
  <q-dialog v-model="open">
    <section class="af" aria-labelledby="af-title">
      <form class="af-main" @submit.prevent="next">
        <header class="af-head">
          <div>
            <div class="epfl-eyebrow">{{ added ? 'Added' : `Step ${step + 1} of ${forgeSteps.length}` }}</div>
            <h2 id="af-title" class="h5">Add a forge</h2>
          </div>
          <button type="button" class="gb-icon-btn" aria-label="Close" @click="open = false"><GbIcon name="close" /></button>
        </header>

        <ol class="gb-steps af-steps" aria-label="Steps">
          <li v-for="(s, i) in forgeSteps" :key="s" class="gb-step" :class="{ 'gb-step-done': i < step || added }" :aria-current="i === step && !added ? 'step' : undefined">
            <span class="gb-step-n">{{ i < step || added ? '✓' : i + 1 }}</span>{{ s }}<template v-if="i === 0 && step > 0">: {{ forge.name }}</template>
          </li>
        </ol>

        <fieldset v-if="step === 0" class="af-fields">
          <legend class="af-legend">Which forge?</legend>
          <p v-for="f in found" :key="f.name" class="gb-hint af-found">
            Already found in your git remotes: <b>{{ f.kind === 'gitlab' ? 'GitLab' : 'GitHub' }} · {{ f.project }}</b>, synced without a table.
            Add it here only to give it a name or a field mapping of its own.
          </p>
          <div class="gb-opts">
            <label v-for="f in forges" :key="f.target" class="gb-opt" :class="{ on: draft.target === f.target }">
              <input v-model="draft.target" type="radio" name="af-forge" :value="f.target" />
              <span><b>{{ f.name }}</b><br /><span class="gb-hint">{{ f.target === 'github' ? 'github.com' : 'gitlab.com, or your own' }}</span></span>
            </label>
          </div>
        </fieldset>

        <div v-else class="af-fields">
          <div>
            <label class="af-legend" for="af-url">The project's address on {{ forge.name }}</label>
            <input id="af-url" v-model="draft.url" class="epfl-input" type="url" autocomplete="off" data-1p-ignore data-lpignore="true" data-form-type="other" :placeholder="forge.placeholder" :disabled="!!added" @input="guess" />
            <p class="gb-hint">The page of the repository, as in your browser's address bar.</p>
          </div>
          <div>
            <label class="af-legend" for="af-name">A short name for this forge <span class="gb-hint af-inline">optional</span></label>
            <input id="af-name" v-model="draft.name" class="epfl-input af-name" autocomplete="off" data-1p-ignore :placeholder="forge.short" :disabled="!!added" />
            <p class="gb-hint">Letters, digits, - and _. Empty: {{ forge.short }}, or the next one free.</p>
          </div>
          <label class="af-check">
            <input v-model="sync" type="checkbox" :disabled="!!added" />
            <span><b>Sync</b><br /><span class="gb-hint">Off, the forge is kept but stays out of the Sync, until you switch it on.</span></span>
          </label>
          <div>
            <span class="af-legend">Its way</span>
            <div class="gb-opts">
              <label v-for="w in forgeWays" :key="w.way" class="gb-opt" :class="{ on: draft.way === w.way }">
                <input v-model="draft.way" type="radio" name="af-way" :value="w.way" :disabled="!!added" />
                <span><b>{{ w.name }}</b><br /><span class="gb-hint">{{ w.hint }}</span></span>
              </label>
            </div>
          </div>
          <div>
            <label class="af-legend" for="af-token">A token <span class="gb-hint af-inline">optional</span></label>
            <input id="af-token" v-model="draft.token" class="epfl-input" type="password" autocomplete="off" data-1p-ignore data-lpignore="true" data-form-type="other" :disabled="!!added" />
            <p class="gb-hint">{{ forge.name }} needs {{ forge.token }}. It goes to git's credential helper (the same store <code>git credential approve</code> fills), never to the file. Empty: the token already stored for this host is used, and the Tokens table can add one later.</p>
          </div>
          <p v-if="added" class="epfl-alert epfl-alert-success" role="status"><b>{{ added }}</b> is added to <code>.git-board.toml</code>.</p>
          <p v-else-if="error" class="epfl-alert epfl-alert-danger" role="alert">{{ error }}</p>
        </div>

        <footer class="af-foot">
          <button v-if="step > 0 && !added" type="button" class="epfl-btn epfl-btn-secondary" :disabled="busy" @click="step--">Back</button>
          <span class="gb-grow" />
          <button v-if="added" type="button" class="epfl-btn epfl-btn-primary" @click="open = false">Done</button>
          <button v-else type="submit" class="epfl-btn epfl-btn-primary" :disabled="busy || !canGoOn(step, draft)">{{ step === 0 ? 'Next' : 'Add the forge' }}</button>
        </footer>
      </form>
    </section>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue';
import { addForge, message, type ForgeWay, type SyncForge } from '@/api';
import { canGoOn, forgeWays, forges, forgeSteps, guessForge, type ForgeDraft } from '@/addforge';
import GbIcon from '@/components/GbIcon.vue';

const open = defineModel<boolean>({ required: true });
// way: the way it opens with, both by default (the Start page's backup: push)
const props = defineProps<{ found: SyncForge[]; way?: ForgeWay }>();
const emit = defineEmits<{ added: [name: string]; guide: [] }>();

const step = ref(0);
const draft = reactive<ForgeDraft>({ target: 'github', url: '', name: '', token: '', way: 'both' });
const sync = ref(true);
const busy = ref(false);
const error = ref('');
const added = ref('');
const forge = computed(() => forges.find((f) => f.target === draft.target) ?? forges[0]!);

// a fresh dialog each time it opens
watch(open, (o) => {
  if (!o) return;
  Object.assign(draft, { target: 'github', url: '', name: '', token: '', way: props.way ?? 'both' });
  Object.assign(step, { value: 0 });
  sync.value = true;
  error.value = '';
  added.value = '';
});

function guess() {
  const g = guessForge(draft.url);
  if (g) draft.target = g;
}

async function next() {
  if (step.value === 0) {
    step.value = 1;
    return;
  }
  busy.value = true;
  error.value = '';
  try {
    const { name } = await addForge({ kind: draft.target, url: draft.url.trim(), name: draft.name.trim(), token: draft.token, sync: sync.value, ...(draft.way === 'both' ? {} : { way: draft.way as 'push' | 'pull' }) });
    draft.token = ''; // not kept in the page
    added.value = name;
    emit('added', name);
  } catch (e) {
    error.value = message(e);
  } finally {
    busy.value = false;
  }
}
</script>

<style scoped>
/* the steps, scrolling when the window is short */
.af { display: grid; grid-template-columns: minmax(0, 1fr); width: min(44rem, calc(100vw - 2rem)); max-height: calc(100vh - 2rem); overflow: auto; background: var(--bg); }
.af-main { min-width: 0; overflow: auto; display: flex; flex-direction: column; gap: 1.25rem; padding: 1.5rem 2rem; }
.af-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 1rem; }
.af-head h2 { margin: 0; }
.af-steps .gb-step { flex-basis: 8rem; padding: .625rem .75rem; font-size: .875rem; }
.af-fields { display: flex; flex-direction: column; gap: 1rem; margin: 0; padding: 0; border: 0; min-width: 0; }
.af-legend { display: block; margin-bottom: .375rem; padding: 0; font-weight: 700; }
.af-inline { display: inline; margin: 0; font-weight: 400; }
.af-name { max-width: 15rem; }
.af-check { display: flex; align-items: flex-start; gap: .625rem; cursor: pointer; }
.af-check input { margin-top: .3rem; accent-color: var(--primary-hover); }
.af-foot { display: flex; flex-wrap: wrap; align-items: center; gap: .75rem; margin-top: auto; padding-top: .5rem; }
.gb-grow { flex: 1; }
.gb-linkbtn { padding: 0; border: 0; background: none; color: var(--fg); text-decoration: underline; cursor: pointer; font: inherit; }
@media (max-width: 59.99rem) {
  .af { display: block; height: auto; max-height: calc(100vh - 2rem); overflow: auto; }
}
@media (max-width: 47.99rem) {
  .af-main { padding: 1.25rem 1rem; }
}
</style>
