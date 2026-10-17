<!-- Add a forge, in four steps, as `git bug bridge new` does: the forge,
     the project (its address, a short name, sync both ways or not), the
     token (stored or pasted), then the check: the server asks the forge
     whether the token sees the project before keeping anything. Beside
     each step, the section of docs/forges.md it needs. -->
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
          <div class="gb-opts">
            <label v-for="f in forges" :key="f.target" class="gb-opt" :class="{ on: draft.target === f.target }">
              <input v-model="draft.target" type="radio" name="af-forge" :value="f.target" />
              <span><b>{{ f.name }}</b><br /><span class="gb-hint">{{ hostHint(f.target) }}</span></span>
            </label>
          </div>
        </fieldset>

        <div v-else-if="step === 1" class="af-fields">
          <div>
            <label class="af-legend" for="af-url">The project's address on {{ forge.name }}</label>
            <input
              id="af-url"
              v-model="draft.url"
              class="epfl-input"
              type="url"
              autocomplete="off" data-1p-ignore data-lpignore="true" data-form-type="other"
              :placeholder="forge.placeholder"
              @input="guess"
            />
            <p class="gb-hint">The page of the repository, as in your browser's address bar.</p>
          </div>
          <div>
            <label class="af-legend" for="af-name">A short name for this forge <span class="gb-hint af-inline">optional</span></label>
            <input id="af-name" v-model="draft.name" class="epfl-input af-name" autocomplete="off" data-1p-ignore :placeholder="forge.short" />
            <p class="gb-hint">Letters, digits, - and _. Empty: {{ forge.short }}, or the next one free.</p>
          </div>
          <label class="af-check">
            <input v-model="sync" type="checkbox" />
            <span><b>Sync both ways</b><br /><span class="gb-hint">Off, the forge is kept but stays out of Sync with forges, until you switch it on.</span></span>
          </label>
        </div>

        <fieldset v-else-if="step === 2" class="af-fields">
          <legend class="af-legend">The token</legend>
          <div class="af-creds">
            <label v-for="c in matching" :key="c.id" class="af-check">
              <input v-model="draft.credential" type="radio" name="af-cred" :value="c.id" />
              <span><b>{{ c.login || 'a token' }}</b> on {{ c.host }} <span class="gb-hint af-inline">stored, {{ c.id }}</span></span>
            </label>
            <label class="af-check">
              <input v-model="draft.credential" type="radio" name="af-cred" value="" />
              <span>Paste a new token</span>
            </label>
          </div>
          <div v-if="!draft.credential">
            <input
              v-model="draft.token"
              class="epfl-input"
              type="password"
              aria-label="Token"
              autocomplete="new-password" data-1p-ignore data-lpignore="true" data-form-type="other"
              :placeholder="forge.token"
            />
            <p class="gb-hint">{{ forge.name }} needs {{ forge.token }}. Kept in git-bug's credential store in this repository, never shown again.</p>
          </div>
        </fieldset>

        <div v-else class="af-fields">
          <dl class="af-sum">
            <dt>Forge</dt><dd>{{ forge.name }}</dd>
            <dt>Project</dt><dd class="af-url">{{ draft.url.trim() }}</dd>
            <dt>Name</dt><dd>{{ added || draft.name.trim() || `${forge.short}, or the next one free` }}</dd>
            <dt>Sync both ways</dt><dd>{{ sync ? 'On' : 'Off' }}</dd>
            <dt>Token</dt><dd>{{ credLabel }}</dd>
          </dl>
          <p v-if="added" class="epfl-alert epfl-alert-success" role="status">
            <b>{{ added }}</b> is added: {{ forge.name }} knows the token and the project.
            {{ sync ? 'It takes part in the next sync, from the Sync page.' : 'Switch its sync on in the table when you want it in the sync.' }}
          </p>
          <p v-else-if="error" class="epfl-alert epfl-alert-danger" role="alert">{{ error }}</p>
          <p v-else class="gb-hint">git-board asks {{ forge.name }} whether the token sees the project. Nothing is kept when it doesn't, and nothing is synced either way.</p>
        </div>

        <footer class="af-foot">
          <button v-if="step > 0 && !added" type="button" class="epfl-btn epfl-btn-secondary" :disabled="busy" @click="back">Back</button>
          <span class="gb-grow" />
          <button v-if="added" type="button" class="epfl-btn epfl-btn-primary" @click="open = false">Done</button>
          <button v-else type="submit" class="epfl-btn epfl-btn-primary" :disabled="busy || !canGoOn(step, draft)">{{ nextLabel }}</button>
        </footer>
      </form>

      <aside class="af-help" aria-labelledby="af-help-title">
        <h3 id="af-help-title" class="af-help-title">How forges work</h3>
        <div v-if="help" :class="`af-help-${help.id}`">
          <h4 class="af-help-h">{{ help.title }}</h4>
          <div class="gb-md" v-html="help.html" />
        </div>
        <p><button type="button" class="gb-linkbtn" @click="emit('guide')">The whole guide</button></p>
      </aside>
    </section>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue';
import { addBridge, getCredentials, getForgeDocs, type Credential, type DocSection } from '@/api/board';
import { canGoOn, forges, forgeSteps, guessForge, matchingCredentials, type ForgeDraft } from '@/addforge';
import { message } from '@/issue';
import GbIcon from '@/components/GbIcon.vue';

const open = defineModel<boolean>({ required: true });
const emit = defineEmits<{ added: [name: string]; guide: [] }>();

const step = ref(0);
const draft = reactive<ForgeDraft>({ target: 'github', url: '', name: '', credential: '', token: '' });
const sync = ref(true);
const busy = ref(false);
const error = ref('');
const added = ref('');
const creds = ref<Credential[]>([]);
const docs = ref<DocSection[]>([]);

const forge = computed(() => forges.find((f) => f.target === draft.target) ?? forges[0]!);
const matching = computed(() => matchingCredentials(creds.value, draft.target, draft.url));
// beside each step, its section of docs/forges.md: the three ways your
// issues go, sync on or off, the tokens, what adding does (ids pinned by
// TestForgeDocsComeInSections)
const helpIds = ['three', 'sync', 'tokens', 'adding'];
const help = computed(() => docs.value.find((d) => d.id === helpIds[step.value]));

// the first stored token fitting, when there is one
watch(matching, (list) => {
  if (!list.some((c) => c.id === draft.credential)) draft.credential = list[0]?.id ?? '';
});

const hostHint = (target: string) => (target === 'github' ? 'github.com' : target === 'gitlab' ? 'gitlab.com or your instance' : 'your instance, or Forgejo');
const credLabel = computed(() => {
  const c = creds.value.find((x) => x.id === draft.credential);
  return c ? `${c.login || 'a token'} on ${c.host} (stored, ${c.id})` : 'a new one, pasted';
});
const nextLabel = computed(() => (step.value === 3 ? (busy.value ? 'Checking with the forge…' : error.value ? 'Check again' : 'Add and check') : `Next: ${['the project', 'the token', 'check'][step.value]}`));

function guess() {
  const g = guessForge(draft.url);
  if (g) draft.target = g;
}

function back() {
  error.value = '';
  step.value--;
}

function next() {
  if (busy.value || added.value || !canGoOn(step.value, draft)) return;
  if (step.value < 3) step.value++;
  else void add();
}

async function add() {
  busy.value = true;
  error.value = '';
  try {
    const res = await addBridge({
      target: draft.target,
      url: draft.url.trim(),
      name: draft.name.trim() || undefined,
      credential: draft.credential || undefined,
      token: draft.credential ? undefined : draft.token.trim(),
      sync: sync.value,
    });
    draft.token = '';
    added.value = res.name;
    emit('added', res.name);
  } catch (e) {
    error.value = message(e);
  } finally {
    busy.value = false;
  }
}

// each opening starts over; a pasted token goes with the closing
watch(
  open,
  async (on) => {
    Object.assign(draft, { target: 'github', url: '', name: '', credential: '', token: '' });
    step.value = 0;
    sync.value = true;
    error.value = added.value = '';
    if (!on) return;
    if (!docs.value.length) getForgeDocs().then((d) => (docs.value = d), () => {});
    creds.value = await getCredentials().catch(() => []);
    draft.credential = matching.value[0]?.id ?? '';
  },
  { immediate: true },
);
</script>

<style scoped>
/* the steps and their help side by side, each scrolling, the same size
   at every step; narrow, the help under the steps */
.af { display: grid; grid-template-columns: minmax(0, 1fr) minmax(16rem, 22rem); width: min(68rem, calc(100vw - 2rem)); height: min(40rem, calc(100vh - 2rem)); background: var(--bg); }
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
.af-creds { display: flex; flex-direction: column; gap: .5rem; }
.af-sum { display: grid; grid-template-columns: auto minmax(0, 1fr); gap: .5rem 1.25rem; margin: 0; }
.af-sum dt { color: var(--fg-muted); }
.af-sum dd { margin: 0; font-weight: 700; }
.af-url { overflow-wrap: anywhere; }
.af-foot { display: flex; flex-wrap: wrap; align-items: center; gap: .75rem; margin-top: auto; padding-top: .5rem; }
.af-help { overflow: auto; display: flex; flex-direction: column; gap: .75rem; padding: 1.5rem 1.75rem; background: var(--bg-subtle); font-size: .875rem; }
.af-help p { margin: 0; }
.af-help-title { margin: 0; font-size: 1.0625rem; font-weight: 700; }
.af-help-h { margin: 0; font-size: .9375rem; font-weight: 700; }
.af-help-h + .gb-md { margin-top: .375rem; }
.af-help .gb-md { font-size: .875rem; }
/* the three channels, a card each */
.af-help-three :deep(ul) { list-style: none; padding: 0; display: flex; flex-direction: column; gap: .5rem; }
.af-help-three :deep(li) { padding: .625rem .75rem; background: var(--bg); }
.gb-grow { flex: 1; }
.gb-linkbtn { padding: 0; border: 0; background: none; color: var(--fg); text-decoration: underline; cursor: pointer; font: inherit; }
@media (max-width: 59.99rem) {
  .af { display: block; height: auto; max-height: calc(100vh - 2rem); overflow: auto; }
}
@media (max-width: 47.99rem) {
  .af-main { padding: 1.25rem 1rem; }
  .af-help { padding: 1.25rem 1rem; }
}
</style>
