<!-- Add a bridge, as `git bug bridge new --sync` does: the forge, the
     project's address, and a credential, stored or pasted. The server asks
     the forge whether the token sees the project before keeping anything. -->
<template>
  <details class="gb-addbridge" :open="open" @toggle="open = ($event.target as HTMLDetailsElement).open">
    <summary>Add a bridge</summary>
    <form class="gb-addbridge-form" @submit.prevent="add">
      <fieldset class="gb-radios">
        <legend class="epfl-label">Forge</legend>
        <label v-for="f in forges" :key="f.target"><input v-model="target" type="radio" name="forge" :value="f.target" /> {{ f.name }}</label>
      </fieldset>

      <label class="epfl-label" for="bridge-url">Project address</label>
      <input
        id="bridge-url"
        v-model="url"
        class="epfl-input"
        type="url"
        required
        autocomplete="off" data-1p-ignore data-lpignore="true" data-form-type="other"
        :placeholder="placeholder"
        @input="guessForge"
      />

      <fieldset class="gb-radios gb-radios-stack">
        <legend class="epfl-label">Credential</legend>
        <label v-for="c in matching" :key="c.id">
          <input v-model="credential" type="radio" name="cred" :value="c.id" />
          <span><b>{{ c.login || 'a token' }}</b> on {{ c.host }} <span class="gb-hint">({{ c.id }})</span></span>
        </label>
        <label>
          <input v-model="credential" type="radio" name="cred" value="" />
          <span>Paste a new token</span>
        </label>
        <input
          v-if="!credential"
          v-model="token"
          class="epfl-input"
          type="password"
          aria-label="Token"
          autocomplete="new-password" data-1p-ignore data-lpignore="true" data-form-type="other"
          :placeholder="target === 'github' ? 'ghp_… with the repo scope' : 'a token with the api scope'"
        />
        <p v-if="!credential" class="gb-hint">Kept in git-bug's credential store in this repository, never shown again.</p>
      </fieldset>

      <label class="epfl-label" for="bridge-name">Name <span class="gb-hint">optional</span></label>
      <input id="bridge-name" v-model="name" class="epfl-input gb-addbridge-name" autocomplete="off" data-1p-ignore :placeholder="short[target]" pattern="[A-Za-z0-9][A-Za-z0-9_\-]*" title="Letters, digits, - and _" />

      <p class="gb-hint">A new bridge takes part in the sync; switch it off in the table above.</p>
      <p v-if="error" class="epfl-alert epfl-alert-danger" role="alert">{{ error }}</p>
      <div>
        <button type="submit" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="busy || !url.trim() || (!credential && !token.trim())">
          {{ busy ? 'Checking with the forge…' : 'Add and check' }}
        </button>
      </div>
    </form>
  </details>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { addBridge, getCredentials, type Credential } from '@/api/board';
import { message } from '@/issue';

const emit = defineEmits<{ added: [name: string] }>();

const forges = [
  { target: 'github', name: 'GitHub' },
  { target: 'gitlab', name: 'GitLab' },
  { target: 'gitea', name: 'Gitea' },
];
const short: Record<string, string> = { github: 'gh', gitlab: 'gl', gitea: 'gitea' };

const open = ref(false);
const target = ref('github');
const url = ref('');
const credential = ref('');
const token = ref('');
const name = ref('');
const busy = ref(false);
const error = ref('');
const creds = ref<Credential[]>([]);

const placeholder = computed(() =>
  target.value === 'github' ? 'https://github.com/owner/repo' : target.value === 'gitlab' ? 'https://gitlab.epfl.ch/group/project' : 'https://gitea.example.org/owner/repo',
);

function hostOf(u: string): string {
  try {
    return new URL(u).host;
  } catch {
    return '';
  }
}

// the stored credentials of this forge, of this instance once the address says it
const matching = computed(() => {
  const host = target.value === 'github' ? 'github.com' : hostOf(url.value);
  return creds.value.filter((c) => c.target === target.value && (!host || c.host === host));
});
// the first one fitting, when there is one
watch(matching, (list) => {
  if (!list.some((c) => c.id === credential.value)) credential.value = list[0]?.id ?? '';
});

function guessForge() {
  const host = hostOf(url.value);
  if (host === 'github.com') target.value = 'github';
  else if (host.includes('gitlab')) target.value = 'gitlab';
  else if (host.includes('gitea')) target.value = 'gitea';
}

async function add() {
  busy.value = true;
  error.value = '';
  try {
    const res = await addBridge({
      target: target.value,
      url: url.value.trim(),
      name: name.value.trim() || undefined,
      credential: credential.value || undefined,
      token: credential.value ? undefined : token.value.trim(),
    });
    token.value = '';
    url.value = '';
    name.value = '';
    open.value = false;
    creds.value = await getCredentials().catch(() => creds.value);
    emit('added', res.name);
  } catch (e) {
    error.value = message(e);
  } finally {
    busy.value = false;
  }
}

onMounted(async () => {
  creds.value = await getCredentials().catch(() => []);
  credential.value = matching.value[0]?.id ?? '';
});
</script>

<style scoped>
.gb-addbridge { margin-top: .5rem; }
.gb-addbridge summary { font-weight: 700; cursor: pointer; }
.gb-addbridge-form { display: grid; gap: .5rem; max-width: 36rem; margin-top: .75rem; }
.gb-radios { display: flex; flex-wrap: wrap; gap: .25rem 1.25rem; margin: 0; padding: 0; border: 0; }
.gb-radios-stack { flex-direction: column; }
.gb-radios label { display: inline-flex; align-items: baseline; gap: .375rem; cursor: pointer; }
.gb-radios input[type='radio'] { accent-color: var(--epfl-red-dark); }
.gb-addbridge-name { max-width: 12rem; }
</style>
