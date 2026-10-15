<!-- Settings: who you write as, which bridges sync, how attachments and
     the GitHub Project are set up, and .git-board.toml itself. Bridges are
     added with git-bug (`git bug bridge new … --sync`). -->
<template>
  <AppHeader :views="viewNames" active="" />
  <main class="gb-page gb-settings">
    <div class="epfl-eyebrow">Settings</div>
    <h1 class="h3">Settings</h1>
    <p v-if="error" class="epfl-alert epfl-alert-danger" role="alert">{{ error }}</p>

    <section aria-labelledby="set-me">
      <h2 id="set-me" class="h5">You</h2>
      <p v-if="me?.me && !choosing">
        You write as <b>{{ me.me.name }}</b><span v-if="me.me.login && !me.me.name.includes(me.me.login)" class="set-login"> ({{ me.me.login }})</span>.
        <button type="button" class="gb-linkbtn" @click="choosing = true">Change</button>
      </p>
      <template v-else>
        <IdentityChooser />
        <button v-if="me?.me" type="button" class="gb-linkbtn" @click="choosing = false">Keep writing as {{ me.me.name }}</button>
      </template>
    </section>

    <section aria-labelledby="set-look">
      <h2 id="set-look" class="h5">Appearance</h2>
      <div class="set-theme" role="radiogroup" aria-labelledby="set-look">
        <label v-for="o in themeOptions" :key="o.value" class="set-theme-opt" :class="{ on: themePref === o.value }">
          <input v-model="themePref" type="radio" name="theme" :value="o.value" />
          <span><b>{{ o.label }}</b><br /><span class="gb-hint">{{ o.hint }}</span></span>
        </label>
      </div>
    </section>

    <section aria-labelledby="set-bridges">
      <h2 id="set-bridges" class="h5">Bridges</h2>
      <p class="gb-hint">A bridge links this repository's issues to a forge's. With sync on, it takes part in <router-link to="/sync">Sync with forges</router-link>.</p>
      <div v-if="settings?.bridges.length" class="gb-table-wrap">
        <table class="gb-table">
          <thead><tr><th>Bridge</th><th>Forge</th><th>Project</th><th>Sync</th><th><span class="visually-hidden">Remove</span></th></tr></thead>
          <tbody>
            <template v-for="b in settings.bridges" :key="b.name">
            <tr>
              <td><b>{{ b.name }}</b></td>
              <td>{{ forgeName(b.target) }}</td>
              <td><a v-if="b.where.startsWith('https://') && !b.where.includes(' ')" :href="b.where" target="_blank" rel="noopener noreferrer">{{ b.where }}</a><span v-else>{{ b.where || '—' }}</span></td>
              <td>
                <label v-if="b.can_sync" class="gb-switch">
                  <input type="checkbox" :checked="b.sync" :disabled="busy === b.name" @change="toggle(b, ($event.target as HTMLInputElement).checked)" />
                  <span>{{ b.sync ? 'On' : 'Off' }}</span>
                </label>
                <span v-else class="gb-hint">import/export only</span>
              </td>
              <td><button type="button" class="gb-linkbtn" :disabled="busy === b.name" @click="confirming = b.name">Remove</button></td>
            </tr>
            <tr v-if="confirming === b.name" class="gb-confirm-row">
              <td colspan="5">
                <p role="alert">
                  <b>Remove the bridge {{ b.name }}?</b> Its settings go. The issues it brought stay, still linked to {{ forgeName(b.target) }}, and the stored credential stays too.
                </p>
                <button type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="busy === b.name" @click="remove(b)">Remove {{ b.name }}</button>
                <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" @click="confirming = ''">Cancel</button>
              </td>
            </tr>
            </template>
          </tbody>
        </table>
      </div>
      <p v-else-if="settings" class="gb-hint">No bridge yet.</p>
      <p v-if="added" class="gb-hint" role="status">Bridge {{ added }} added, with sync on.</p>
      <BridgeAddForm @added="onAdded" />
    </section>

    <section aria-labelledby="set-uploads">
      <h2 id="set-uploads" class="h5">Attachments</h2>
      <p v-if="settings?.uploads.enabled">
        Files attached to comments go to <code>{{ settings.uploads.endpoint }}</code>, under <code>{{ settings.uploads.prefix }}/</code>,
        served at <a :href="settings.uploads.public_url" target="_blank" rel="noopener noreferrer">{{ settings.uploads.public_url }}</a>.
      </p>
      <p v-else-if="settings">
        Off. Set <code>ACCESS_KEY_ID</code>, <code>ACCESS_KEY_SECRET</code> and <code>BUCKET_NAME</code> where git-board-server runs,
        and optionally <code>[uploads]</code> in <code>.git-board.toml</code>.
      </p>
    </section>

    <section aria-labelledby="set-project">
      <h2 id="set-project" class="h5">GitHub Project</h2>
      <p v-if="settings?.github_project.number">
        <b>{{ settings.github_project.owner }} #{{ settings.github_project.number }}</b>,
        {{ settings.github_project.write ? 'read and written' : 'read only' }}. <router-link to="/conflicts">Sync with the Project</router-link>
      </p>
      <p v-else-if="settings">None. Set <code>[github-project]</code> in <code>.git-board.toml</code>, or run <code>git board bridge pull --owner … --project …</code>.</p>
    </section>

    <ConfigEditor @saved="reload" />
  </main>
</template>

<script setup lang="ts">
import { themePref } from '@/theme';
import { onMounted, ref, watch } from 'vue';
import { getSettings, getViews, removeBridge, setBridgeSync, type Bridge, type Settings } from '@/api/board';
import { loadForges } from '@/forges';
import { message } from '@/issue';
import { loadMe, me } from '@/me';
import AppHeader from '@/components/AppHeader.vue';
import IdentityChooser from '@/components/IdentityChooser.vue';
import ConfigEditor from '@/components/ConfigEditor.vue';
import BridgeAddForm from '@/components/BridgeAddForm.vue';

const settings = ref<Settings | null>(null);
const themeOptions = [
  { value: 'system', label: 'System', hint: 'as the computer is set' },
  { value: 'light', label: 'Light', hint: 'always' },
  { value: 'dark', label: 'Dark', hint: 'always' },
] as const;
const viewNames = ref<string[]>([]);
const error = ref('');
const busy = ref('');
const choosing = ref(false);

// back to the sentence once an identity is picked
watch(() => me.value?.me?.id, () => (choosing.value = false));

function forgeName(target: string) {
  return ({ github: 'GitHub', gitlab: 'GitLab', gitea: 'Gitea' } as Record<string, string>)[target] ?? target;
}

const confirming = ref('');
const added = ref('');

async function remove(b: Bridge) {
  busy.value = b.name;
  error.value = '';
  try {
    await removeBridge(b.name);
    confirming.value = '';
    await reload();
    void loadForges(); // the header's sync link
  } catch (e) {
    error.value = `${b.name} not removed: ${message(e)}`;
  } finally {
    busy.value = '';
  }
}

async function onAdded(name: string) {
  added.value = name;
  await reload();
  void loadForges();
}

async function toggle(b: Bridge, on: boolean) {
  busy.value = b.name;
  error.value = '';
  try {
    await setBridgeSync(b.name, on);
    b.sync = on;
    void loadForges(); // the header's sync link
  } catch (e) {
    error.value = `Sync of ${b.name} not changed: ${message(e)}`;
    settings.value = await getSettings().catch(() => settings.value);
  } finally {
    busy.value = '';
  }
}

// after a save of .git-board.toml: the header's views, the Project's line
async function reload() {
  try {
    settings.value = await getSettings();
  } catch (e) {
    error.value = message(e);
  }
  try {
    viewNames.value = Object.keys(await getViews()).sort();
  } catch {
    // the header shows no views
  }
}

onMounted(() => {
  if (!me.value) void loadMe();
  void reload();
});
</script>

<style scoped>
.gb-settings section { display: grid; gap: .5rem; margin-top: 2rem; }
.gb-settings section h2 { margin: 0; }
.gb-settings section p { margin: 0; }
.gb-linkbtn { justify-self: start; padding: 0; border: 0; background: none; color: var(--fg); text-decoration: underline; cursor: pointer; font: inherit; }
.set-login { color: var(--fg-muted); }
.set-theme { display: flex; flex-wrap: wrap; gap: .5rem; }
.set-theme-opt { display: flex; align-items: flex-start; gap: .5rem; min-width: 10rem; padding: .625rem .875rem; border: 1px solid var(--border); cursor: pointer; }
.set-theme-opt.on { border-color: var(--fg); }
.set-theme-opt input { margin-top: .3rem; accent-color: var(--primary-hover); }
.gb-confirm-row td { background: var(--warning-bg); }
.gb-confirm-row p { margin: 0 0 .5rem; }
.gb-confirm-row .epfl-btn + .epfl-btn { margin-left: .5rem; }
.visually-hidden { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
.gb-switch { display: inline-flex; align-items: center; gap: .5rem; cursor: pointer; font-weight: 700; }
.gb-switch input { width: 1.125rem; height: 1.125rem; accent-color: var(--primary-hover); }
</style>
