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

    <section aria-labelledby="set-bridges">
      <h2 id="set-bridges" class="h5">Bridges</h2>
      <p class="gb-hint">A bridge with sync on takes part in <router-link to="/sync">Sync with GitHub/GitLab</router-link>.</p>
      <div v-if="settings?.bridges.length" class="gb-table-wrap">
        <table class="gb-table">
          <thead><tr><th>Bridge</th><th>Forge</th><th>Project</th><th>Sync</th></tr></thead>
          <tbody>
            <tr v-for="b in settings.bridges" :key="b.name">
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
            </tr>
          </tbody>
        </table>
      </div>
      <p v-else-if="settings" class="gb-hint">No bridge yet.</p>
      <p class="gb-hint">Add one with <code>git bug bridge new --name gh --url https://github.com/owner/repo --sync</code>.</p>
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
import { onMounted, ref, watch } from 'vue';
import { getSettings, getViews, setBridgeSync, type Bridge, type Settings } from '@/api/board';
import { loadForges } from '@/forges';
import { message } from '@/issue';
import { loadMe, me } from '@/me';
import AppHeader from '@/components/AppHeader.vue';
import IdentityChooser from '@/components/IdentityChooser.vue';
import ConfigEditor from '@/components/ConfigEditor.vue';

const settings = ref<Settings | null>(null);
const viewNames = ref<string[]>([]);
const error = ref('');
const busy = ref('');
const choosing = ref(false);

// back to the sentence once an identity is picked
watch(() => me.value?.me?.id, () => (choosing.value = false));

function forgeName(target: string) {
  return ({ github: 'GitHub', gitlab: 'GitLab', gitea: 'Gitea' } as Record<string, string>)[target] ?? target;
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
.gb-switch { display: inline-flex; align-items: center; gap: .5rem; cursor: pointer; font-weight: 700; }
.gb-switch input { width: 1.125rem; height: 1.125rem; accent-color: var(--primary-hover); }
</style>
