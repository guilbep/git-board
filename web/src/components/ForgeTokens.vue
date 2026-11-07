<!-- Settings › Sync › Tokens: the forges' tokens, as git board forge
     status reads them (GET /api/forges/auth): for each forge, for each of
     its accounts (the posting login, yours), whether a token is found and
     where — an environment variable, or git's credential helper. Check
     asks each forge whose each token is. Add a token (POST
     /api/forges/auth) stores it in git's credential helper (forge login,
     ADR 0010): never the file, never shown again, the body alone. -->
<template>
  <section class="ft">
    <div class="ft-head">
      <div>
        <h3 class="h5">Tokens</h3>
        <p class="ft-lead">
          Each forge syncs with a token: taken from <code>GITHUB_TOKEN</code> / <code>GITLAB_TOKEN</code>, else git's credential helper — the tokens <code>git board forge status</code> reads. A token is never shown.
          <button type="button" class="gb-linkbtn" @click="guideOpen = true">How forges work</button>
        </p>
      </div>
      <div class="ft-actions">
        <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" :disabled="checking || busy" @click="check">
          {{ checking ? 'Checking…' : 'Check' }}
        </button>
        <button type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="busy" @click="adding = true">+ Add a token</button>
      </div>
    </div>
    <p v-if="error" class="epfl-alert epfl-alert-danger" role="alert">{{ error }}</p>

    <div v-if="items.length" class="gb-table-wrap">
      <table class="gb-table">
        <thead><tr><th>Forge</th><th>Account</th><th>Token</th></tr></thead>
        <tbody>
          <template v-for="f in items" :key="f.name">
            <tr v-for="(t, i) in f.tokens" :key="`${f.name} ${t.role}`">
              <td v-if="i === 0" :rowspan="f.tokens.length">
                <b>{{ f.name }}</b> <span class="ft-kind">{{ kindName(f.kind) }}</span>
                <div class="ft-note">{{ f.host }} · {{ f.project }}</div>
              </td>
              <td>{{ who(t) }}</td>
              <td>{{ line(t) }}</td>
            </tr>
          </template>
        </tbody>
      </table>
    </div>
    <p v-else-if="!error" class="ft-note">No forge: none in <code>.git-board.toml</code>, and none found in the git remotes.</p>

    <q-dialog v-model="adding">
      <section class="ft-dialog" role="dialog" aria-modal="true" aria-labelledby="ft-title">
        <form @submit.prevent="store">
          <header class="ft-head">
            <h2 id="ft-title" class="h5">Add a token</h2>
            <button type="button" class="gb-icon-btn" aria-label="Cancel" @click="adding = false"><GbIcon name="close" /></button>
          </header>
          <div class="ft-fields">
            <div>
              <label class="ft-label" for="ft-forge">The forge</label>
              <select id="ft-forge" v-model="draft.forge" class="epfl-input">
                <option v-for="f in items" :key="f.name" :value="f.name">{{ f.name }} — {{ f.host }}/{{ f.project }}</option>
              </select>
            </div>
            <div>
              <label class="ft-label" for="ft-login">The account it is for <span class="ft-note">optional</span></label>
              <input id="ft-login" v-model="draft.login" class="epfl-input" type="text" autocomplete="off" data-1p-ignore data-lpignore="true" data-form-type="other" placeholder="yours on the host" />
              <p class="ft-note">Empty: your own account on the host, else the forge's posting account.</p>
            </div>
            <div>
              <label class="ft-label" for="ft-token">The token</label>
              <input id="ft-token" v-model="draft.token" class="epfl-input" type="password" autocomplete="off" data-1p-ignore data-lpignore="true" data-form-type="other" required />
              <p class="ft-note">It goes to git's credential helper (<code>git credential approve</code>), never to <code>.git-board.toml</code>, and is never shown again. On a terminal, <code>gh auth token | git board forge login &lt;forge&gt;</code> stores the same.</p>
            </div>
            <p v-if="error" class="epfl-alert epfl-alert-danger" role="alert">{{ error }}</p>
          </div>
          <footer class="ft-foot">
            <span class="gb-grow" />
            <button type="button" class="epfl-btn epfl-btn-secondary" :disabled="busy" @click="adding = false">Cancel</button>
            <button type="submit" class="epfl-btn epfl-btn-primary" :disabled="busy || !draft.token || !draft.forge">{{ busy ? 'Storing…' : 'Store the token' }}</button>
          </footer>
        </form>
      </section>
    </q-dialog>

    <ForgesGuide v-model="guideOpen" />
  </section>
</template>

<script setup lang="ts">
import { onMounted, ref, watch } from 'vue';
import { getForgeAuth, message, storeForgeToken, type ForgeAuth, type ForgeToken } from '@/api';
import ForgesGuide from '@/components/ForgesGuide.vue';
import GbIcon from '@/components/GbIcon.vue';

const emit = defineEmits<{ changed: [] }>();

const items = ref<ForgeAuth[]>([]);
const error = ref('');
const checking = ref(false);
const busy = ref(false);
const adding = ref(false);
const guideOpen = ref(false);
const draft = ref({ forge: '', login: '', token: '' });

const kindName = (k: string) => ({ github: 'GitHub', gitlab: 'GitLab' })[k] ?? k;
const who = (t: ForgeToken) =>
  t.role === 'you' ? `${t.login} (you)` : t.login ? `${t.login} (posting)` : 'any login (posting)';

// the token, said as forge status says it: found and where, and, checked,
// whose the forge says it is. Never the token.
function line(t: ForgeToken): string {
  if (!t.found) return 'none found — Add a token, or git board forge login';
  if (t.error) return `found (${t.source}), but the forge refused it: ${t.error}`;
  if (t.owner && t.login && t.owner.toLowerCase() !== t.login.toLowerCase()) return `found (${t.source}), but it is ${t.owner}'s`;
  if (t.owner) return `found (${t.source}), it is ${t.owner}'s`;
  return `found (${t.source})`;
}

async function load(check = false) {
  if (check) checking.value = true;
  try {
    items.value = (await getForgeAuth(check)).items;
    error.value = '';
  } catch (e) {
    error.value = message(e);
  } finally {
    checking.value = false;
  }
}
const check = () => load(true);

// a fresh dialog each time it opens, on the first forge
watch(adding, (o) => {
  if (!o) return;
  draft.value = { forge: items.value[0]?.name ?? '', login: '', token: '' };
  error.value = '';
});

async function store() {
  busy.value = true;
  error.value = '';
  try {
    await storeForgeToken({ forge: draft.value.forge, login: draft.value.login.trim(), token: draft.value.token });
    draft.value.token = ''; // not kept in the page
    adding.value = false;
    await load();
    emit('changed');
  } catch (e) {
    error.value = message(e);
  } finally {
    busy.value = false;
  }
}

onMounted(() => void load());
</script>

<style scoped>
.ft { display: flex; flex-direction: column; gap: 1rem; }
.ft h3, .ft p { margin: 0; }
.ft-head { display: flex; flex-wrap: wrap; align-items: flex-start; justify-content: space-between; gap: 1rem; }
.ft-lead { max-width: 44rem; color: var(--fg-muted); }
.ft-actions { display: flex; flex-wrap: wrap; align-items: center; gap: .5rem; }
.ft-kind, .ft-note { font-size: .8125rem; color: var(--fg-muted); }
.ft-dialog { display: grid; grid-template-columns: minmax(0, 1fr); width: min(36rem, calc(100vw - 2rem)); background: var(--bg); }
.ft-dialog form { display: flex; flex-direction: column; gap: 1.25rem; padding: 1.5rem 2rem; overflow: auto; }
.ft-fields { display: flex; flex-direction: column; gap: 1rem; min-width: 0; }
.ft-label { display: block; margin-bottom: .375rem; font-weight: 700; }
.ft-foot { display: flex; flex-wrap: wrap; align-items: center; gap: .75rem; margin-top: auto; padding-top: .5rem; }
.gb-grow { flex: 1; }
.gb-linkbtn { padding: 0; border: 0; background: none; color: var(--fg); text-decoration: underline; cursor: pointer; font: inherit; }
@media (max-width: 47.99rem) {
  .ft-dialog form { padding: 1.25rem 1rem; }
}
</style>
