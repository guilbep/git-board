<!-- Settings, in tabs with their address (#/settings/<tab>, settingstabs.ts):
     General (who you write as, the appearance), the project file
     (.git-board.toml), attachments, then where your issues go: remotes,
     forges (git-bug bridges: add one, sync on or off, remove one) and the
     GitHub Project. #/settings/forges?add=1 opens "Add a forge". -->
<template>
  <AppHeader :views="viewNames" active="" />
  <div class="gb-page set">
    <nav class="set-nav" aria-label="Settings">
      <h1 class="h3 set-title">Settings</h1>
      <router-link v-for="t in settingsTabs.slice(0, 3)" :key="t.id" :to="`/settings/${t.id}`" class="set-tab" :aria-current="tab === t.id ? 'page' : undefined">{{ t.label }}</router-link>
      <div class="set-group">Where your issues go</div>
      <router-link v-for="(c, i) in channels" :key="c.id" :to="`/settings/${c.id}`" class="set-tab" :aria-current="tab === c.id ? 'page' : undefined">
        <span class="set-n">{{ i + 1 }}</span> {{ c.label }} <span class="set-via">{{ c.via }}</span>
      </router-link>
    </nav>

    <main class="set-main">
      <p v-if="error" class="epfl-alert epfl-alert-danger" role="alert">{{ error }}</p>

      <div v-if="tab === 'general'" class="set-panel">
        <h2 class="h4">General</h2>
        <section aria-labelledby="set-me">
          <h3 id="set-me" class="h5">You</h3>
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
          <h3 id="set-look" class="h5">Appearance</h3>
          <div class="gb-opts" role="radiogroup" aria-labelledby="set-look">
            <label v-for="o in themeOptions" :key="o.value" class="gb-opt" :class="{ on: themePref === o.value }">
              <input v-model="themePref" type="radio" name="theme" :value="o.value" />
              <span><b>{{ o.label }}</b><br /><span class="gb-hint">{{ o.hint }}</span></span>
            </label>
          </div>
        </section>
      </div>

      <!-- v-show: switching tabs keeps an unsaved draft -->
      <div v-show="tab === 'project-file'" class="set-panel set-panel-wide">
        <ConfigEditor @saved="reload" />
      </div>

      <div v-if="tab === 'attachments'" class="set-panel">
        <h2 class="h4">Attachments</h2>
        <p v-if="settings?.uploads.enabled">
          Files attached to comments go to <code>{{ settings.uploads.endpoint }}</code>, under <code>{{ settings.uploads.prefix }}/</code>,
          served at <a :href="settings.uploads.public_url" target="_blank" rel="noopener noreferrer">{{ settings.uploads.public_url }}</a>.
        </p>
        <p v-else-if="settings">
          Off. Set <code>ACCESS_KEY_ID</code>, <code>ACCESS_KEY_SECRET</code> and <code>BUCKET_NAME</code> where git-board-server runs,
          and optionally <code>[uploads]</code> in <code>.git-board.toml</code>.
        </p>
      </div>

      <div v-if="tab === 'remotes'" class="set-panel">
        <ChannelMap current="remotes" />
        <h2 class="h4">Remotes</h2>
        <p class="set-lead">
          Your issues travel with git, like your code: <code>git bug push</code> sends them to a remote with their comments and people, <code>git bug pull</code> gets them.
          Push and pull them from <router-link to="/sync">Sync › Remotes</router-link>.
        </p>
      </div>

      <div v-if="tab === 'forges'" class="set-panel">
        <ChannelMap current="forges" />
        <div class="set-head">
          <div>
            <h2 class="h4">Forges</h2>
            <p class="set-lead">A forge mirrors this repository's issues on GitHub, GitLab or Gitea, for the people who work there. Your issues already travel with git: see <router-link to="/settings/remotes">Remotes</router-link>.</p>
          </div>
          <div class="set-actions">
            <button type="button" class="epfl-btn epfl-btn-secondary" @click="guideOpen = true">How forges work</button>
            <button type="button" class="epfl-btn epfl-btn-primary" @click="adding = true">+ Add a forge</button>
          </div>
        </div>
        <div v-if="settings?.bridges.length" class="gb-table-wrap">
          <table class="gb-table">
            <thead><tr><th>Name</th><th>Forge</th><th>Project</th><th>Sync</th><th><span class="visually-hidden">Remove</span></th></tr></thead>
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
                    <b>Remove the forge {{ b.name }}?</b> Its settings go. The issues it brought stay, still linked to {{ forgeName(b.target) }}, and the stored token stays too.
                  </p>
                  <button type="button" class="epfl-btn epfl-btn-primary epfl-btn-sm" :disabled="busy === b.name" @click="remove(b)">Remove {{ b.name }}</button>
                  <button type="button" class="epfl-btn epfl-btn-secondary epfl-btn-sm" @click="confirming = ''">Cancel</button>
                </td>
              </tr>
              </template>
            </tbody>
          </table>
        </div>
        <p v-else-if="settings" class="gb-hint">No forge yet: <button type="button" class="gb-linkbtn" @click="adding = true">add one</button>.</p>
        <p class="set-lead">
          Running a check or a sync: <router-link to="/sync">Sync › Forges</router-link>.
          The Project's fields have their own tab: <router-link to="/settings/github-project">3 · GitHub Project</router-link>.
        </p>
      </div>

      <div v-if="tab === 'github-project'" class="set-panel">
        <ChannelMap current="github-project" />
        <h2 class="h4">GitHub Project</h2>
        <p class="set-lead">The Project's fields (status, priority, sprint) become labels here, and go back when writing is on.</p>
        <template v-if="settings?.github_project.number">
          <div class="set-tiles">
            <div class="gb-value gb-value-agreed"><span class="gb-value-label">Owner</span><span class="gb-value-val">{{ settings.github_project.owner }}</span></div>
            <div class="gb-value gb-value-agreed"><span class="gb-value-label">Project</span><span class="gb-value-val">#{{ settings.github_project.number }}</span></div>
            <div class="gb-value gb-value-agreed"><span class="gb-value-label">Writing</span><span class="gb-value-val">{{ settings.github_project.write ? 'On · read and written' : 'Off · read only' }}</span></div>
          </div>
          <p><router-link to="/conflicts">Sync with the Project</router-link></p>
        </template>
        <p v-else-if="settings">None. Set <code>[github-project]</code> in the <router-link to="/settings/project-file">project file</router-link>, or run <code>git board bridge pull --owner … --project …</code>.</p>
      </div>
    </main>
  </div>

  <AddForgeDialog v-model="adding" @added="onAdded" @guide="guideOpen = true" />
  <ForgesGuide v-model="guideOpen" />
</template>

<script setup lang="ts">
import { themePref } from '@/theme';
import { onLive } from '@/live';
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { getSettings, getViews, removeBridge, setBridgeSync, type Bridge, type Settings } from '@/api/board';
import { loadForges } from '@/forges';
import { message } from '@/issue';
import { loadMe, me } from '@/me';
import { channels, settingsTab, settingsTabs } from '@/settingstabs';
import AppHeader from '@/components/AppHeader.vue';
import IdentityChooser from '@/components/IdentityChooser.vue';
import ConfigEditor from '@/components/ConfigEditor.vue';
import ChannelMap from '@/components/ChannelMap.vue';
import AddForgeDialog from '@/components/AddForgeDialog.vue';
import ForgesGuide from '@/components/ForgesGuide.vue';

const route = useRoute();
const router = useRouter();
const tab = computed(() => settingsTab(route.params.tab));
// "Add a forge" is open while the address says ?add=1 (the Sync page
// links there); closing it, however, drops it
const adding = computed({
  get: () => tab.value === 'forges' && route.query.add === '1',
  set: (on) => void router.replace({ path: '/settings/forges', query: on ? { add: '1' } : {} }),
});
const guideOpen = ref(false);

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

async function onAdded() {
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
const offLive = onLive((e) => (e.kind === 'settings' || e.kind === 'config') && void reload());
onBeforeUnmount(offLive);
</script>

<style scoped>
.set { display: flex; align-items: flex-start; gap: 3rem; }
.set-nav { flex: none; width: 15rem; display: flex; flex-direction: column; gap: .125rem; }
.set-title { margin: 0 0 1rem; }
.set-tab { display: flex; align-items: baseline; gap: .625rem; padding: .625rem .875rem; color: var(--fg); text-decoration: none; }
.set-tab:hover { background: var(--bg-subtle); color: var(--fg); }
.set-tab[aria-current='page'] { font-weight: 700; background: var(--bg-subtle); box-shadow: inset 3px 0 0 var(--primary); }
.set-group { margin: 1.25rem 0 .375rem; padding: 0 .875rem; font-size: .75rem; font-weight: 700; letter-spacing: .08em; text-transform: uppercase; color: var(--fg-muted); }
.set-n { color: var(--fg-muted); }
.set-tab[aria-current='page'] .set-n { color: var(--primary-hover); }
.set-via { margin-left: auto; font-size: .8125rem; font-weight: 400; color: var(--fg-muted); }
.set-main { flex: 1; min-width: 0; }
.set-panel { display: flex; flex-direction: column; gap: 1.25rem; max-width: 64rem; }
.set-panel-wide { max-width: none; }
.set-panel h2, .set-panel h3, .set-panel p { margin: 0; }
.set-panel section { display: grid; gap: .5rem; }
.set-head { display: flex; flex-wrap: wrap; align-items: flex-end; justify-content: space-between; gap: 1rem; }
.set-lead { max-width: 40rem; color: var(--fg-muted); }
.set-head h2 { margin-bottom: .375rem; }
.set-actions { display: flex; flex-wrap: wrap; gap: .75rem; }
.set-tiles { display: grid; grid-template-columns: repeat(auto-fit, minmax(10rem, 1fr)); gap: .75rem; max-width: 45rem; }
.gb-linkbtn { justify-self: start; padding: 0; border: 0; background: none; color: var(--fg); text-decoration: underline; cursor: pointer; font: inherit; }
.set-login { color: var(--fg-muted); }
.gb-confirm-row td { background: var(--warning-bg); }
.gb-confirm-row p { margin: 0 0 .5rem; }
.gb-confirm-row .epfl-btn + .epfl-btn { margin-left: .5rem; }
/* the hidden header stays in the table's scroll, not past the page */
.gb-table-wrap { position: relative; }
.visually-hidden { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
.gb-switch { display: inline-flex; align-items: center; gap: .5rem; cursor: pointer; font-weight: 700; }
.gb-switch input { width: 1.125rem; height: 1.125rem; accent-color: var(--primary-hover); }
/* phones: the menu, a few rows above the tab */
@media (max-width: 47.99rem) {
  .set { flex-direction: column; align-items: stretch; gap: 1.5rem; padding: 1.25rem 1rem 2rem; }
  .set-nav { width: auto; flex-direction: row; flex-wrap: wrap; }
  .set-title, .set-group { flex-basis: 100%; }
  .set-title { margin-bottom: .25rem; }
  .set-group { margin-top: .5rem; }
  .set-tab { padding: .5rem .75rem; }
  .set-tab[aria-current='page'] { box-shadow: inset 0 -3px 0 var(--primary); }
  .set-via { margin-left: 0; }
}
</style>
